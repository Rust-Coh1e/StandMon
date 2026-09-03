package planner

import (
	"PriceMon/internal/scheduler"
	"context"
	"log"
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

func NewProductPlanner(sch ScheduleRepository, tsk TaskRepository, poll time.Duration) ProductPlanner {
	return ProductPlanner{
		schedules:    sch,
		tasks:        tsk,
		pollInterval: poll,
	}
}

// func (pp *ProductPlanner) Run(ctx context.Context) {
// 	ticker := time.NewTicker(pp.pollInterval)
// 	defer ticker.Stop()

// 	for {
// 		select {
// 		case <-ctx.Done():
// 			return
// 		case <-ticker.C:
// 			now := time.Now()

// 			products, err := pp.schedules.ListActive(ctx)
// 			if err != nil {
// 				continue
// 			}

// 			for _, product := range products {

// 				exist, err := pp.tasks.HasFutureTask(ctx, product.ProductID, now)

// 				if exist || err != nil {
// 					continue
// 				}

// 				nextRun := NextRunAt(product, now)

// 				newCheckTask := scheduler.CheckTask{
// 					ProductID:   product.ProductID,
// 					URL:         product.URL,
// 					ScheduledAt: nextRun,
// 					Attempt:     0,
// 				}

// 				_, err = pp.tasks.Create(ctx, newCheckTask)
// 				if err != nil {
// 					continue
// 				}
// 			}
// 		}
// 	}
// }

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
				log.Printf("ListActive error: %v", err)
				continue
			}

			log.Printf("active products: %d", len(products))

			for _, product := range products {
				log.Printf(
					"product id=%d url=%s interval=%s",
					product.ProductID,
					product.URL,
					product.Interval,
				)

				exist, err := pp.tasks.HasFutureTask(ctx, product.ProductID, now)
				if err != nil {
					log.Printf("HasFutureTask product=%d error: %v", product.ProductID, err)
					continue
				}

				log.Printf("product=%d futureTask=%v", product.ProductID, exist)

				if exist {
					continue
				}

				nextRun := NextRunAt(product, now)

				log.Printf(
					"creating task product=%d scheduled_at=%s",
					product.ProductID,
					nextRun,
				)

				task, err := pp.tasks.Create(ctx, scheduler.CheckTask{
					ProductID:   product.ProductID,
					URL:         product.URL,
					ScheduledAt: nextRun,
					Attempt:     0,
				})
				if err != nil {
					log.Printf("Create task error: %v", err)
					continue
				}

				log.Printf("created task id=%d", task.ID)
			}
		}
	}
}
