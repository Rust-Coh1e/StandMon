package scheduler

import (
	"context"
	"time"
)

type CheckTask struct {
	ID          int64
	ProductID   int64
	URL         string
	ScheduledAt time.Time
	Attempt     int
}

type TaskRepository interface {
	ClaimDueTasks(ctx context.Context, now time.Time, limit int) ([]CheckTask, error)
}

type Scheduler struct {
	repo         TaskRepository
	pollInterval time.Duration
	batchSize    int
}

func NewScheduler(repo TaskRepository, pollInterval time.Duration, batchSize int) *Scheduler {
	return &Scheduler{
		repo:         repo,
		pollInterval: pollInterval,
		batchSize:    batchSize,
	}
}

func (s *Scheduler) Run(ctx context.Context) <-chan CheckTask {
	out := make(chan CheckTask)

	go func() {
		defer close(out)

		ticker := time.NewTicker(s.pollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return

			case <-ticker.C:
				// 1. ClaimDueTasks
				currTasks, err := s.repo.ClaimDueTasks(ctx, time.Now(), s.batchSize)
				if err != nil {
					continue
				}
				// 3. пройтись по tasks
				for _, task := range currTasks {
					select {
					case <-ctx.Done():
						return
					case out <- task:
					}

				}
			}
		}
	}()

	return out
}
