ALTER TABLE ai.runs
    ADD COLUMN wake_at timestamp with time zone;

CREATE INDEX ai_runs_waiting_external_wake_idx
    ON ai.runs (wake_at, id)
    WHERE status = 'waiting_external';
