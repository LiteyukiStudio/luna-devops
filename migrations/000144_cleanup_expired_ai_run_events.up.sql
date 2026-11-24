-- One-time idempotent cleanup of pre-accumulated expired AI streaming event rows (> 14 days
-- since terminal run completion). Historical conversations, turns, runs, timeline items, and
-- conversation summaries in ai.conversations, ai.turns, ai.runs, ai.items, and
-- ai.conversation_summaries are intentionally preserved.
--
-- Index idx_ai_runs_retention_terminal on the compact parent table (ai.runs) pairs with the
-- existing unique index run_events_run_id_event_sequence_key on ai.run_events so both this
-- migration and daily batched retention cleanup stream via index lookups without adding write
-- amplification to live SSE delta inserts on ai.run_events.
--
-- IMPORTANT: Do NOT run VACUUM FULL here because golang-migrate executes migrations inside
-- a transaction block. After upgrading a large database, run manually during a low-traffic
-- window to return physical disk space to the operating system:
--   VACUUM (FULL, ANALYZE) ai.run_events;
--   VACUUM (FULL, ANALYZE) public.runtime_observations;

CREATE INDEX IF NOT EXISTS idx_ai_runs_retention_terminal
    ON ai.runs USING btree ((COALESCE(completed_at, created_at)), id)
    WHERE status IN ('completed', 'failed', 'canceled', 'expired', 'interrupted');

DELETE FROM ai.run_events AS events
USING ai.runs AS parent
WHERE parent.id = events.run_id
  AND parent.status IN ('completed', 'failed', 'canceled', 'expired', 'interrupted')
  AND COALESCE(parent.completed_at, parent.created_at) < (now() - interval '14 days');

ALTER TABLE ai.run_events SET (
    autovacuum_vacuum_scale_factor = 0.05,
    autovacuum_analyze_scale_factor = 0.02
);

ALTER TABLE public.runtime_observations SET (
    autovacuum_vacuum_scale_factor = 0.05,
    autovacuum_analyze_scale_factor = 0.02
);
