package checkprocessor

import (
	"PriceMon/internal/worker"
	"context"
	"fmt"
	"time"
)

type Repository interface {
	// ClaimDueTasks(ctx context.Context, now time.Time, limit int) ([]CheckTask, error)
	CreatePriceSnapshot(ctx context.Context, inputSnapshot worker.CheckResult, finishedAt time.Time) error
	RetryTask(ctx context.Context, taskID int64, nextRetryAt time.Time, inputErr string) error
	FailTask(ctx context.Context, taskID int64, inputErr string, now time.Time) error
}

type CheckProcessor struct {
	db Repository
	//retry policy
}

func NewCheckProcessor(db Repository) CheckProcessor {
	return CheckProcessor{
		db: db,
		// attempts: attempts,
	}
}

func (ch *CheckProcessor) Run(ctx context.Context, results <-chan worker.CheckResult) error {

	for {
		select {
		case <-ctx.Done():
			return nil
		case input, open := <-results:
			// Тут нужна логика обработки результата
			if !open {
				return nil
			}
			if input.Err == nil {
				err := ch.db.CreatePriceSnapshot(ctx, input, time.Now())
				if err != nil {
					return fmt.Errorf("SQL err: %w", err)
				}

				continue
			}

			upd, incTime := worker.RetryDelay(input.Task.Attempt)

			if upd {
				err := ch.db.RetryTask(ctx, input.Task.ID, time.Now().Add(incTime), input.Err.Error())
				if err != nil {
					return fmt.Errorf("SQL err: %w", err)
				}
				continue
			} else {
				err := ch.db.FailTask(ctx, input.Task.ID, input.Err.Error(), time.Now())
				if err != nil {
					return fmt.Errorf("SQL err: %w", err)
				}
				continue
			}

		}
	}

}
