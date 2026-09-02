package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"PriceMon/internal/checkprocessor"
	"PriceMon/internal/parser"
	"PriceMon/internal/planner"
	"PriceMon/internal/registry"
	postgres "PriceMon/internal/repository"
	"PriceMon/internal/scheduler"
	"PriceMon/internal/worker"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	// Один context управляет жизненным циклом всего приложения.
	ctx, cancel := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer cancel()

	// -------------------------
	// PostgreSQL
	// -------------------------

	db, err := pgxpool.New(
		ctx,
		"postgres://user:password@localhost:5432/pricemon",
	)
	if err != nil {
		log.Fatalf("postgres connect: %v", err)
	}
	defer db.Close()

	if err := db.Ping(ctx); err != nil {
		log.Fatalf("postgres ping: %v", err)
	}

	log.Println("postgres connected")

	// -------------------------
	// Repository
	// -------------------------

	repo := postgres.NewTaskRepository(
		db,
		"scheduler-1",
		30*time.Second,
	)

	// -------------------------
	// Parsers + Registry
	// -------------------------

	parserRegistry := registry.NewParserRegistry()


	// -------------------------
	// Planner
	// -------------------------

	productPlanner := planner.NewProductPlanner(
		repo          // ListActive
		repo,          // HasFutureTask + Create
		5*time.Second, // как часто проверяем необходимость создать task
	)

	// -------------------------
	// Scheduler
	// -------------------------

	taskScheduler := scheduler.NewScheduler(
		repo,
		time.Second, // polling due tasks
		10,          // batch size
	)

	// Scheduler сам запускает goroutine
	// и отдаёт канал задач.
	tasks := taskScheduler.Run(ctx)

	// -------------------------
	// Worker Pool
	// -------------------------

	workerPool := worker.NewWorkerPool(
		parserRegistry,
		5,
	)

	// WorkerPool также запускает workers
	// и возвращает канал результатов.
	results := workerPool.Run(ctx, tasks)

	// -------------------------
	// Check Processor
	// -------------------------

	checkProcessor := checkprocessor.NewCheckProcessor(repo)

	// Planner блокирующий, поэтому запускаем отдельно.
	go func() {
		log.Println("planner started")

		productPlanner.Run(ctx)

		log.Println("planner stopped")
	}()

	// CheckProcessor тоже блокирующий.
	go func() {
		log.Println("check processor started")

		if err := checkProcessor.Run(ctx, results); err != nil {
			log.Printf("check processor stopped with error: %v", err)
			cancel()
			return
		}

		log.Println("check processor stopped")
	}()

	log.Println("price monitor started")

	// main живёт, пока приложение не получит Ctrl+C / SIGTERM.
	<-ctx.Done()

	log.Println("shutting down")
}
