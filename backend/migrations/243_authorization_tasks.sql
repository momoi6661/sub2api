CREATE TABLE IF NOT EXISTS authorization_tasks (
    id BIGSERIAL PRIMARY KEY,
    account_id BIGINT NOT NULL DEFAULT 0,
    status TEXT NOT NULL CHECK (status IN ('queued', 'running', 'succeeded', 'ready', 'failed')),
    stage TEXT NOT NULL DEFAULT 'preparing',
    summary JSONB NOT NULL,
    events JSONB NOT NULL DEFAULT '[]',
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS authorization_tasks_created_idx ON authorization_tasks (id DESC);
CREATE INDEX IF NOT EXISTS authorization_tasks_active_idx ON authorization_tasks (status, id) WHERE status IN ('queued', 'running');
CREATE UNIQUE INDEX IF NOT EXISTS authorization_tasks_account_active_idx ON authorization_tasks (account_id) WHERE account_id > 0 AND status IN ('queued', 'running');
