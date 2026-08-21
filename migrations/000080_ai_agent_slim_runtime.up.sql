-- Add the minimal identity and approval state needed by the single Tool
-- Executor. Existing runs, events, items, cards and tool calls are preserved.
ALTER TABLE ai.runs
    ADD COLUMN IF NOT EXISTS actor_session_id text;

ALTER TABLE ai.tool_calls
    ADD COLUMN IF NOT EXISTS approval_decision text
    CHECK (approval_decision IN ('approve', 'approve_always'));

ALTER TABLE ai.tool_calls
    ALTER COLUMN arguments_hash SET DEFAULT '';

CREATE TABLE IF NOT EXISTS ai.tool_approval_exemptions (
    user_id text NOT NULL,
    operation_id text NOT NULL,
    source_tool_call_id text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, operation_id)
);

CREATE INDEX IF NOT EXISTS ai_runs_unowned_queue_idx
    ON ai.runs (status, created_at)
    WHERE status = 'queued';

COMMENT ON COLUMN ai.runs.actor_session_id IS
    'Browser session bound by the authenticated API when this Run is created; never supplied by a model tool call.';
COMMENT ON TABLE ai.tool_approval_exemptions IS
    'Revocable per-user, per-operation approve-always preferences. No project or conversation wildcard is permitted.';
