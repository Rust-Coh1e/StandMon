package checkprocessor

import (
	"PriceMon/internal/parser"
	"PriceMon/internal/scheduler"
	product_service "PriceMon/internal/service"
	"PriceMon/internal/worker"
	"context"
	"fmt"
	"time"
)

type Repository interface {
	// ClaimDueTasks(ctx context.Context, now time.Time, limit int) ([]CheckTask, error)
	CreatePriceSnapshot(ctx context.Context, inputSnapshot worker.CheckResult) (product_service.PriceSnapshot, error)
	CompleteTask(ctx context.Context, taskID int64, finishedAt time.Time) error
	RetryTask(ctx context.Context, taskID int64, nextRetryAt time.Time, inputErr string) error
	FailTask(ctx context.Context, taskID int64, inputErr string, now time.Time) error
}

type CheckProcessor struct {
	db Repository
	//retry policy
	attempts int
}

func NewCheckProcessor(db Repository, attempts int) CheckProcessor {
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
				_, err := ch.db.CreatePriceSnapshot(ctx, input)
				if err != nil {
					return fmt.Errorf("SQL err: %w", err)
				}
				err = ch.db.CompleteTask(ctx, input.Task.ID, time.Now())
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

type CheckResult struct {
	Task scheduler.CheckTask
	Info parser.ProductInfo
	Err  error
}

type CheckTask struct {
	ID          int64
	ProductID   int64
	URL         string
	ScheduledAt time.Time
	Attempt     int
}

type ProductInfo struct {
	// Name      string // Как будто не обязательно, поскольку можно взять у юзера
	Store     string
	Price     int
	CheckedAt time.Time
}
