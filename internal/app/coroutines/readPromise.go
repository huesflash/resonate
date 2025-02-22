package coroutines

import (
	"log/slog"

	"github.com/resonatehq/resonate/internal/kernel/scheduler"
	"github.com/resonatehq/resonate/internal/kernel/t_aio"
	"github.com/resonatehq/resonate/internal/kernel/t_api"
	"github.com/resonatehq/resonate/internal/util"
	"github.com/resonatehq/resonate/pkg/promise"
)

func ReadPromise(req *t_api.Request, res func(*t_api.Response, error)) *scheduler.Coroutine {
	return scheduler.NewCoroutine("ReadPromise", func(s *scheduler.Scheduler, c *scheduler.Coroutine) {
		submission := &t_aio.Submission{
			Kind: t_aio.Store,
			Store: &t_aio.StoreSubmission{
				Transaction: &t_aio.Transaction{
					Commands: []*t_aio.Command{
						{
							Kind: t_aio.ReadPromise,
							ReadPromise: &t_aio.ReadPromiseCommand{
								Id: req.ReadPromise.Id,
							},
						},
					},
				},
			},
		}

		c.Yield(submission, func(completion *t_aio.Completion, err error) {
			if err != nil {
				slog.Error("failed to read promise", "req", req, "err", err)
				res(nil, err)
				return
			}

			util.Assert(completion.Store != nil, "completion must not be nil")

			result := completion.Store.Results[0].ReadPromise
			util.Assert(result.RowsReturned == 0 || result.RowsReturned == 1, "result must return 0 or 1 rows")

			if result.RowsReturned == 0 {
				res(&t_api.Response{
					Kind: t_api.ReadPromise,
					ReadPromise: &t_api.ReadPromiseResponse{
						Status: t_api.ResponseNotFound,
					},
				}, nil)
			} else {
				p, err := result.Records[0].Promise()
				if err != nil {
					slog.Error("failed to parse promise record", "record", result.Records[0], "err", err)
					res(nil, err)
					return
				}

				if p.State == promise.Pending && s.Time() >= p.Timeout {
					s.Add(TimeoutPromise(p, ReadPromise(req, res), func(err error) {
						if err != nil {
							slog.Error("failed to timeout promise", "req", req, "err", err)
							res(nil, err)
							return
						}

						res(&t_api.Response{
							Kind: t_api.ReadPromise,
							ReadPromise: &t_api.ReadPromiseResponse{
								Status: t_api.ResponseOK,
								Promise: &promise.Promise{
									Id:    p.Id,
									State: promise.Timedout,
									Param: p.Param,
									Value: promise.Value{
										Headers: map[string]string{},
										Data:    nil,
									},
									Timeout:                   p.Timeout,
									IdempotencyKeyForCreate:   p.IdempotencyKeyForCreate,
									IdempotencyKeyForComplete: p.IdempotencyKeyForComplete,
									Tags:                      p.Tags,
									CreatedOn:                 p.CreatedOn,
									CompletedOn:               &p.Timeout,
								},
							},
						}, nil)
					}))
				} else {
					res(&t_api.Response{
						Kind: t_api.ReadPromise,
						ReadPromise: &t_api.ReadPromiseResponse{
							Status:  t_api.ResponseOK,
							Promise: p,
						},
					}, nil)
				}
			}
		})
	})
}
