package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"PriceMon/internal/checkprocessor"
	"PriceMon/internal/handler"
	"PriceMon/internal/planner"
	"PriceMon/internal/registry"
	postgres "PriceMon/internal/repository"
	"PriceMon/internal/scheduler"
	"PriceMon/internal/worker"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
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
		"postgres://user:password@localhost:5434/pricemon",
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
	// Parser Registry
	// -------------------------

	parserRegistry := registry.NewParserRegistry()

	// Если NewParserRegistry сам НЕ регистрирует парсеры,
	// зарегистрируй их здесь.

	// -------------------------
	// Planner
	// -------------------------

	productPlanner := planner.NewProductPlanner(
		repo,
		repo,
		5*time.Second,
	)

	// -------------------------
	// Scheduler
	// -------------------------

	taskScheduler := scheduler.NewScheduler(
		repo,
		time.Second,
		10,
	)

	tasks := taskScheduler.Run(ctx)

	// -------------------------
	// Worker Pool
	// -------------------------

	workerPool := worker.NewWorkerPool(
		parserRegistry,
		5,
	)

	results := workerPool.Run(ctx, tasks)

	// -------------------------
	// Check Processor
	// -------------------------

	checkProcessor := checkprocessor.NewCheckProcessor(repo)

	// -------------------------
	// Background processes
	// -------------------------

	go func() {
		log.Println("planner started")

		productPlanner.Run(ctx)

		log.Println("planner stopped")
	}()

	go func() {
		log.Println("check processor started")

		if err := checkProcessor.Run(ctx, results); err != nil {
			log.Printf("check processor error: %v", err)
			cancel()
			return
		}

		log.Println("check processor stopped")
	}()

	// -------------------------
	// HTTP API
	// -------------------------
	parseHandler := handler.NewParseHandler(
		parserRegistry,
		repo,
	)

	mux := http.NewServeMux()

	mux.HandleFunc("POST /products", parseHandler.Create)

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	go func() {
		log.Println("HTTP server started on :8080")

		err := server.ListenAndServe()

		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("HTTP server error: %v", err)
			cancel()
		}
	}()

	log.Println("price monitor started")

	// -------------------------
	// Shutdown
	// -------------------------

	<-ctx.Done()

	log.Println("shutting down")

	shutdownCtx, shutdownCancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP shutdown error: %v", err)
	}

	log.Println("price monitor stopped")
}
