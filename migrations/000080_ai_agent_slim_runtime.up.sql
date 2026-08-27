-- Add the minimal identity and approval state needed by the single Tool
-- Executor. Existing runs, events, items, cards and tool calls are preserved.
ALTER TABLE ai.runs
    ADD COLUMN IF NOT EXISTS actor_session_id text;

ALTER TABLE ai.tool_calls
    ADD COLUMN IF NOT EXISTS approval_decision text
    CHECK (approval_decision = 'approve');

ALTER TABLE ai.tool_calls
    ALTER COLUMN arguments_hash SET DEFAULT '';

CREATE INDEX IF NOT EXISTS ai_runs_unowned_queue_idx
    ON ai.runs (status, created_at)
    WHERE status = 'queued';

COMMENT ON COLUMN ai.runs.actor_session_id IS
    'Browser session bound by the authenticated API when this Run is created; never supplied by a model tool call.';
