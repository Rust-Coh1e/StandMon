package postgres

import (
	"PriceMon/internal/planner"
	"PriceMon/internal/scheduler"
	product_service "PriceMon/internal/service"
	"PriceMon/internal/worker"
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type TaskRepository struct {
	db            *pgxpool.Pool
	lockedBy      string
	leaseDuration time.Duration
}

func NewTaskRepository(db *pgxpool.Pool, lockedBy string, leaseDuration time.Duration) *TaskRepository {
	return &TaskRepository{
		db:            db,
		lockedBy:      lockedBy,
		leaseDuration: leaseDuration,
	}
}

// type CheckTask struct {
// 	ID          int64
// 	ProductID   int64
// 	URL         string
// 	ScheduledAt time.Time
// 	Attempt     int
// }

// CREATE TABLE price_check_tasks (
//     id BIGSERIAL PRIMARY KEY,
//     product_id BIGINT NOT NULL REFERENCES products(id),

//     scheduled_at TIMESTAMPTZ NOT NULL,

//     status TEXT NOT NULL DEFAULT 'pending',

//     attempt_count INT NOT NULL DEFAULT 0,

//     started_at TIMESTAMPTZ,
//     finished_at TIMESTAMPTZ,

//     locked_by TEXT,
//     locked_until TIMESTAMPTZ,

//     next_retry_at TIMESTAMPTZ,
//     error_message TEXT,

//     created_at TIMESTAMPTZ NOT NULL DEFAULT now()
// );

func (tr *TaskRepository) Create(ctx context.Context, task scheduler.CheckTask) (scheduler.CheckTask, error) {

	query := `INSERT INTO price_check_tasks (product_id, scheduled_at, attemt_count) 
				VALUES ($1, $2, $3) 
				RETURNING id`

	err := tr.db.QueryRow(ctx, query, task.ProductID, task.ScheduledAt, task.Attempt).Scan(&task.ID)

	if err != nil {
		return task, fmt.Errorf("create check task: %w", err)
	}

	return task, nil
}

func (tr *TaskRepository) HasFutureTask(ctx context.Context, productID int64, now time.Time) (bool, error) {
	var result bool
	query := `SELECT EXISTS (
					SELECT 1
					FROM price_check_tasks
					WHERE product_id = $1
					AND scheduled_at > $2
					AND status = 'pending'
				)`

	err := tr.db.QueryRow(ctx, query, productID, now).Scan(&result)

	if err != nil {
		return false, fmt.Errorf("create check task: %w", err)
	}

	return result, nil
}

func (tr *TaskRepository) ClaimDueTasks(ctx context.Context, now time.Time, limit int) ([]scheduler.CheckTask, error) {
	tx, err := tr.db.Begin(ctx)
	if err != nil {
		return []scheduler.CheckTask{}, fmt.Errorf("не удалось начать транзакцию: %w", err)
	}

	defer tx.Rollback(ctx)
	tasks := make([]scheduler.CheckTask, 0)
	query := `
			SELECT pct.id, pct.product_id, products.url, pct.scheduled_at, pct.attempt_count
			FROM price_check_tasks as pct
			INNER JOIN products ON pct.product_id = products.id
			WHERE status = 'pending'
			AND scheduled_at <= $1
			AND (
				next_retry_at IS NULL
				OR next_retry_at <= $1
			)
			AND (
				locked_until IS NULL
				OR locked_until <= $1
			)
			ORDER BY scheduled_at
			LIMIT $2
			FOR UPDATE OF pct SKIP LOCKED`

	rows, err := tx.Query(ctx, query, now, limit)
	if err != nil {
		return []scheduler.CheckTask{}, fmt.Errorf("ошибка списания: %w", err)
	}
	defer rows.Close()

	// Запомним вариант, но лучше в ручную поскольку названия таблицы различаются
	// tasks, err := pgx.CollectRows(rows, pgx.RowToStructByName[scheduler.CheckTask])
	// if err != nil {
	// 	return []scheduler.CheckTask{}, fmt.Errorf("ошибка сборки строк: %w", err)
	// }
	ids := make([]int64, 0)
	for rows.Next() {
		var t scheduler.CheckTask
		err := rows.Scan(&t.ID, &t.ProductID, &t.URL, &t.ScheduledAt, &t.Attempt)
		if err != nil {
			return []scheduler.CheckTask{}, fmt.Errorf("ошибка сканирования строки: %w", err)
		}

		tasks = append(tasks, t)
		// Дополнительно сохраняем все ID для LOCK
		ids = append(ids, t.ID)
	}

	if err := rows.Err(); err != nil {
		return []scheduler.CheckTask{}, fmt.Errorf("ошибка при чтении строк: %w", err)
	}
	//TODO Сделать так что если ids пустая коммит
	if len(ids) == 0 {
		err = tx.Commit(ctx)
		return []scheduler.CheckTask{}, err
	}
	// теперь нужно сделать Lock

	query = `
			UPDATE price_check_tasks as pct
			SET locked_by = $1, locked_until = $2
			WHERE id = ANY($3)`

	upd, err := tx.Exec(ctx, query, tr.lockedBy, now.Add(tr.leaseDuration), ids)
	if err != nil {
		return []scheduler.CheckTask{}, fmt.Errorf("ошибка обновления строки: %w", err)
	}
	if upd.RowsAffected() != int64(len(ids)) {
		return []scheduler.CheckTask{}, fmt.Errorf("Некорректное изменение строк: Changed %d waited %d", upd.RowsAffected(), int64(len(ids)))
	}

	err = tx.Commit(ctx)
	if err != nil {
		return []scheduler.CheckTask{}, err
	}

	return tasks, nil
}

func (tr *TaskRepository) CreatePriceSnapshot(ctx context.Context, inputSnapshot worker.CheckResult) (product_service.PriceSnapshot, error) {

	query := `INSERT into price_snapshots (task_id, product_id, price, checked_at) 
				VALUES ($1, $2, $3, $4) 
				RETURNING id`
	var id int64
	err := tr.db.QueryRow(ctx, query, inputSnapshot.Task.ID, inputSnapshot.Task.ProductID, inputSnapshot.Info.Price, inputSnapshot.Info.CheckedAt).Scan(&id)
	if err != nil {
		return product_service.PriceSnapshot{}, fmt.Errorf("create check task: %w", err)
	}

	return product_service.PriceSnapshot{
		ID:        id,
		ProductID: inputSnapshot.Task.ProductID,
		Price:     inputSnapshot.Info.Price,
		CheckedAt: inputSnapshot.Info.CheckedAt,
	}, nil
}

func (tr *TaskRepository) CompleteTask(ctx context.Context, taskID int64, finishedAt time.Time) error {
	query := `UPDATE price_check_tasks as pct
			  SET status = 'completed', finished_at = $1 locked_by = NULL, locked_until = NULL
			  WHERE 
			  		id = $2
			  AND
			  		status = 'pending'`
	rows, err := tr.db.Exec(ctx, query, finishedAt, taskID)
	if err != nil {
		return fmt.Errorf("sql update: %w", err)
	}
	if rows.RowsAffected() != 1 {
		return fmt.Errorf(
			"complete task %d: expected 1 affected row, got %d",
			taskID,
			rows.RowsAffected(),
		)
	}
	return nil
}

func (tr *TaskRepository) RetryTask(ctx context.Context, taskID int64, nextRetryAt time.Time, inputErr string) error {
	query := `	
		UPDATE price_check_tasks 
		SET
			attempt_count = attempt_count + 1,
			next_retry_at = $1,
			error_message = $2,
			locked_by = NULL,
			locked_until = NULL

		WHERE id = $3
			AND status = 'pending'`

	rows, err := tr.db.Exec(ctx, query, nextRetryAt, inputErr, taskID)
	if err != nil {
		return fmt.Errorf("sql update: %w", err)
	}

	if rows.RowsAffected() != 1 {
		return fmt.Errorf(
			"complete task %d: expected 1 affected row, got %d",
			taskID,
			rows.RowsAffected(),
		)
	}
	return nil
}

func (tr *TaskRepository) FailTask(ctx context.Context, taskID int64, inputErr string, now time.Time) error {
	query := `	
		UPDATE price_check_tasks 
		SET
			status = 'failed',
			error_message = $1,
			locked_by = NULL,
			locked_until = NULL,
			finished_at = $2,
			next_retry_at = NULL,
			attempt_count = attempt_count + 1,

		WHERE id = $3
			AND status = 'pending'`

	rows, err := tr.db.Exec(ctx, query, inputErr, now, taskID)
	if err != nil {
		return fmt.Errorf("sql update: %w", err)
	}

	if rows.RowsAffected() != 1 {
		return fmt.Errorf(
			"retry task %d: expected 1 affected row, got %d",
			taskID,
			rows.RowsAffected(),
		)
	}
	return nil
}

func (tr *TaskRepository) ListActive(ctx context.Context) ([]planner.ProductPlan, error) {
	query := `
		SELECT
			id,
			url,
			check_interval_seconds
		FROM products
		WHERE active = true
	`

	rows, err := tr.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list active products: %w", err)
	}
	defer rows.Close()

	products := make([]planner.ProductPlan, 0)

	for rows.Next() {
		var (
			product         planner.ProductPlan
			intervalSeconds int64
		)

		err := rows.Scan(
			&product.ProductID,
			&product.URL,
			&intervalSeconds,
		)
		if err != nil {
			return nil, fmt.Errorf("scan active product: %w", err)
		}

		product.Interval = time.Duration(intervalSeconds) * time.Second

		products = append(products, product)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active products: %w", err)
	}

	return products, nil
}
