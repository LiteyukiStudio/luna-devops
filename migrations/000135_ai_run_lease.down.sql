DROP INDEX IF EXISTS ai.ai_runs_running_lease_idx;

ALTER TABLE ai.runs
    DROP COLUMN IF EXISTS lease_expires_at;
