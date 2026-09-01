package worker

import (
	"PriceMon/internal/parser"
	"PriceMon/internal/registry"
	"PriceMon/internal/scheduler"
	"context"
	"sync"
)

type CheckResult struct {
	Task scheduler.CheckTask
	Info parser.ProductInfo
	Err  error
}

func NewCheckResult(task scheduler.CheckTask, info parser.ProductInfo, err error) CheckResult {
	return CheckResult{
		Task: task,
		Info: info,
		Err:  err,
	}
}

type WorkerPool struct {
	registry registry.ParserRegistry
	workers  int
}

func (w *WorkerPool) Run(ctx context.Context, tasks <-chan scheduler.CheckTask) <-chan CheckResult {
	var wg sync.WaitGroup

	results := make(chan CheckResult)

	for range w.workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case get, ok := <-tasks:
					if !ok {
						return
					}

					p, err := w.registry.Resolve(get.URL)

					if err != nil {
						resultStruct := NewCheckResult(get, parser.ProductInfo{}, err)
						results <- resultStruct
						continue
					}

					result, err := p.Parse(ctx, get.URL)
					resultStruct := NewCheckResult(get, result, err)
					select {
					case results <- resultStruct:
					case <-ctx.Done():
						return
					}
				}
			}

		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	return results
}
