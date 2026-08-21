-- Retire executable lease machinery after the runtime moved to one atomic
-- queued -> running claim. Data-bearing legacy columns are intentionally kept
-- on upgraded databases so historical rows are never discarded.
DROP FUNCTION IF EXISTS ai.release_run_lease(text, text);
DROP FUNCTION IF EXISTS ai.renew_run_lease(text, text, integer);
DROP FUNCTION IF EXISTS ai.claim_next_run(text, integer);

DROP INDEX IF EXISTS ai.ai_runs_queue_idx;
DROP INDEX IF EXISTS ai.ai_runs_unowned_queue_idx;

CREATE INDEX ai_runs_queue_idx
    ON ai.runs (status, created_at)
    WHERE status = 'queued';
