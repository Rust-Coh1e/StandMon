CREATE TABLE products (
    id BIGSERIAL PRIMARY KEY,
    url TEXT NOT NULL UNIQUE,
    store TEXT NOT NULL,
    check_interval_seconds BIGINT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE price_check_tasks (
    id BIGSERIAL PRIMARY KEY,
    product_id BIGINT NOT NULL REFERENCES products(id),

    scheduled_at TIMESTAMPTZ NOT NULL,

    status TEXT NOT NULL DEFAULT 'pending',

    attempt_count INT NOT NULL DEFAULT 0,

    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,

    locked_by TEXT,
    locked_until TIMESTAMPTZ,

    next_retry_at TIMESTAMPTZ,
    error_message TEXT,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE price_snapshots (
    id BIGSERIAL PRIMARY KEY,
    task_id BIGINT NOT NULL REFERENCES price_check_tasks(id) UNIQUE,
    product_id BIGINT NOT NULL REFERENCES products(id),
    price INT NOT NULL,
    checked_at TIMESTAMPTZ NOT NULL
);
