package planner

import (
	"PriceMon/internal/scheduler"
	"context"
	"time"
)

type ProductPlan struct {
	ProductID int64
	URL       string
	Interval  time.Duration
}

func NextRunAt(product ProductPlan, now time.Time) time.Time {
	interval := product.Interval

	offset := product.ProductID % int64(interval.Seconds())

	// Начало текущего временного окна.
	windowStart := now.Truncate(interval)

	// Кандидат внутри текущего окна.
	candidate := windowStart.Add(
		time.Duration(offset) * time.Second,
	)

	// Если этот момент уже прошёл,
	// переносим запуск в следующее окно.
	if !candidate.After(now) {
		candidate = candidate.Add(interval)
	}

	return candidate
}

type ScheduleRepository interface {
	ListActive(ctx context.Context) ([]ProductPlan, error)
}

type TaskRepository interface {
	HasFutureTask(ctx context.Context, productID int64, now time.Time) (bool, error)
	Create(ctx context.Context, task scheduler.CheckTask) (scheduler.CheckTask, error)
}

type ProductPlanner struct {
	schedules    ScheduleRepository
	tasks        TaskRepository
	pollInterval time.Duration
}

func (pp *ProductPlanner) Run(ctx context.Context) {
	ticker := time.NewTicker(pp.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()

			products, err := pp.schedules.ListActive(ctx)
			if err != nil {
				continue
			}

			for _, product := range products {

				exist, err := pp.tasks.HasFutureTask(ctx, product.ProductID, now)

				if exist || err != nil {
					continue
				}

				nextRun := NextRunAt(product, now)

				newCheckTask := scheduler.CheckTask{
					ProductID:   product.ProductID,
					URL:         product.URL,
					ScheduledAt: nextRun,
					Attempt:     0,
				}

				_, err = pp.tasks.Create(ctx, newCheckTask)
				if err != nil {
					continue
				}
			}
		}
	}
}
