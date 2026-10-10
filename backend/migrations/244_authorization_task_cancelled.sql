ALTER TABLE authorization_tasks DROP CONSTRAINT IF EXISTS authorization_tasks_status_check;
ALTER TABLE authorization_tasks ADD CONSTRAINT authorization_tasks_status_check
    CHECK (status IN ('queued', 'running', 'succeeded', 'ready', 'failed', 'cancelled'));
