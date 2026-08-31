

1. Что здесь не нравится?

```go
func GetUserOrders(ctx context.Context, db *sql.DB, userIDs []int64) ([]Order, error) {
	var result []Order

	for _, id := range userIDs {
        // Выполняются отдельно запросы для кажого id
        // используется общий контекст для всех запросов (N+1 проблема)

		rows, err := db.QueryContext(ctx,
			`SELECT id, user_id, amount
			 FROM orders
			 WHERE user_id = $1`, id)
        // Тут либо делать батч чтение либо делать новые контексты на базе ctx
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		for rows.Next() {
			var o Order
			if err := rows.Scan(&o.ID, &o.UserID, &o.Amount); err != nil {
				return nil, err
			}
			result = append(result, o)
		}
	}

	return result, nil
}


```
Лучшее решение:

```sql
SELECT id, user_id, amount
FROM orders
WHERE user_id = ANY($1);
```


2. Есть товар:

```sql
products (
    id bigint primary key,
    stock int not null
)
```

Код покупки:

```go
func Buy(ctx context.Context, db *sql.DB, id int64) error {
	var stock int

	err := db.QueryRowContext(ctx,
		`SELECT stock FROM products WHERE id = $1`,
		id,
	).Scan(&stock)
	if err != nil {
		return err
	}

	if stock <= 0 {
		return errors.New("out of stock")
	}

	_, err = db.ExecContext(ctx,
		`UPDATE products
		 SET stock = $1
		 WHERE id = $2`,
		stock-1, id,
	)

	return err
}
```

1. Тут проблема в том, что пока произвелась проверка на то, что количество больше 0, возможно само число стало равно нули. То есть нужен конкрентный доступ. Мы должны делать атомарный Update без привязки к текущему значени тип 
		`UPDATE products
		 SET stock = stock - 1
		 WHERE id = $2
            and stock > 0
        Returning stock`,
2. Я бы добавил returning value и проверка что она больше 0


Что произойдёт при 100 конкурентных запросах?

3. Регистрация пользователя:

```go
func CreateUser(ctx context.Context, db *sql.DB, email string) error {
	var exists bool

	err := db.QueryRowContext(ctx,
		`SELECT EXISTS (
			SELECT 1 FROM users WHERE email = $1
		)`,
		email,
	).Scan(&exists)
	if err != nil {
		return err
	}

	if exists {
		return errors.New("already exists")
	}

	_, err = db.ExecContext(ctx,
		`INSERT INTO users(email) VALUES ($1)`,
		email,
	)

	return err
}
```

В таблице нет `UNIQUE(email)`. Что здесь не так?

4. Транзакция:

```go
func Process(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx,
		`UPDATE orders SET status = 'processing' WHERE id = $1`,
		42,
	)
	if err != nil {
		return err
	}

	resp, err := http.Get("https://payment-service/pay")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return tx.Commit()
}
```

Что здесь опасного?

5. Worker queue:

```go
func TakeJob(ctx context.Context, tx *sql.Tx) (Job, error) {
	var j Job

	err := tx.QueryRowContext(ctx,
		`SELECT id, payload
		 FROM jobs
		 WHERE status = 'pending'
		 ORDER BY id
		 LIMIT 1
		 FOR UPDATE`,
	).Scan(&j.ID, &j.Payload)

	return j, err
}
```

Есть 50 workers. Почему система может работать хуже, чем ожидается? Что можно изменить?

6. Пагинация:

```sql
SELECT id, created_at, title
FROM posts
ORDER BY created_at DESC
LIMIT 50 OFFSET 500000;
```

В таблице 100 млн строк. Есть индекс:

```sql
CREATE INDEX idx_posts_created_at
ON posts(created_at DESC);
```

Что здесь всё равно плохо?

7. Cursor pagination:

```sql
SELECT id, created_at, title
FROM posts
WHERE created_at < $1
ORDER BY created_at DESC
LIMIT 50;
```

Что может сломаться, если много постов имеют одинаковый `created_at`?

8. Индекс:

```sql
CREATE INDEX idx_orders
ON orders(status, created_at);
```

Основные запросы:

```sql
SELECT *
FROM orders
WHERE created_at >= now() - interval '1 day';
```

и:

```sql
SELECT *
FROM orders
WHERE status = 'new'
  AND created_at >= now() - interval '1 day';
```

Насколько хорошо выбран индекс? Для какого запроса он полезнее?

9. Массовая вставка:

```go
for _, row := range rows {
	_, err := db.ExecContext(ctx,
		`INSERT INTO metrics(ts, value) VALUES ($1, $2)`,
		row.TS,
		row.Value,
	)
	if err != nil {
		return err
	}
}
```

`rows` содержит 2 млн элементов. Что бы ты поменял?

10. Большой delete:

```sql
DELETE FROM events
WHERE created_at < now() - interval '1 year';
```

Удаляется 300 млн строк. Какие проблемы ожидаешь и какие варианты предложишь?

11. Connection pool:

```go
db.SetMaxOpenConns(500)
```

Приложение работает в Kubernetes, 30 pod'ов. PostgreSQL настроен на `max_connections = 1000`.

Что здесь подозрительно?

12. Индекс ради ускорения:

```sql
CREATE INDEX idx_users_gender
ON users(gender);
```

Таблица 50 млн строк, примерно 50% `male`, 50% `female`.

Запрос:

```sql
SELECT *
FROM users
WHERE gender = 'male';
```

Почему PostgreSQL может проигнорировать индекс?
Потому что планировщику выгоднее пойти в seq scan потому что затрагивается большая часть таблицы.
