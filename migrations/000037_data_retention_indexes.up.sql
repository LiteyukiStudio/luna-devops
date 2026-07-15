CREATE INDEX IF NOT EXISTS idx_platform_events_retention
    ON platform_events(occurred_at, id);

CREATE INDEX IF NOT EXISTS idx_notification_deliveries_retention_terminal
    ON notification_deliveries(finished_at, id)
    WHERE status IN ('succeeded', 'failed');

CREATE INDEX IF NOT EXISTS idx_worker_task_events_retention
    ON worker_task_events(created_at, id);

CREATE INDEX IF NOT EXISTS idx_build_runs_retention_terminal
    ON build_runs(finished_at, id)
    WHERE status IN ('succeeded', 'failed', 'canceled', 'lost', 'timeout');

CREATE INDEX IF NOT EXISTS idx_release_logs_retention_parent
    ON release_logs(release_id, id);

CREATE INDEX IF NOT EXISTS idx_releases_retention_terminal
    ON releases(finished_at, id)
    WHERE status IN ('succeeded', 'failed');

CREATE INDEX IF NOT EXISTS idx_hook_run_logs_retention_parent
    ON hook_run_logs(hook_run_id, id);

CREATE INDEX IF NOT EXISTS idx_hook_runs_retention_terminal
    ON hook_runs(finished_at, id)
    WHERE status IN ('succeeded', 'failed');

CREATE INDEX IF NOT EXISTS idx_step_up_assertions_retention_expiry
    ON step_up_assertions((LEAST(idle_expires_at, absolute_expires_at)), id);

CREATE INDEX IF NOT EXISTS idx_user_sessions_retention_expiry
    ON user_sessions(expires_at, id);

CREATE INDEX IF NOT EXISTS idx_user_remember_tokens_retention_expiry
    ON user_remember_tokens(expires_at, id);

