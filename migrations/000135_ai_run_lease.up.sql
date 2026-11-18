ALTER TABLE ai.runs
    ADD COLUMN IF NOT EXISTS lease_expires_at timestamp with time zone;

CREATE INDEX IF NOT EXISTS ai_runs_running_lease_idx
    ON ai.runs (lease_expires_at, id)
    WHERE status = 'running';
