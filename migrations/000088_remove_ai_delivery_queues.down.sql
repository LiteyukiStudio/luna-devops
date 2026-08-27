ALTER TABLE ai.tool_calls
    DROP CONSTRAINT IF EXISTS tool_calls_approval_decision_check;

ALTER TABLE ai.tool_calls
    ADD CONSTRAINT tool_calls_approval_decision_check
    CHECK (approval_decision IN ('approve', 'approve_always'));

ALTER TABLE ai.runs
    ADD COLUMN IF NOT EXISTS client_instance_id text;

ALTER TABLE ai.runs
    DROP COLUMN IF EXISTS execution_snapshot_ciphertext;

CREATE TABLE ai.tool_approval_exemptions (
    user_id text NOT NULL,
    operation_id text NOT NULL,
    source_tool_call_id text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, operation_id)
);

CREATE TABLE ai.ui_actions (
    id text PRIMARY KEY,
    run_id text NOT NULL REFERENCES ai.runs(id) ON DELETE CASCADE,
    tool_call_id text NOT NULL UNIQUE,
    client_instance_id text NOT NULL,
    action jsonb NOT NULL,
    status text NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'succeeded', 'failed', 'expired')),
    attempts integer NOT NULL DEFAULT 1 CHECK (attempts > 0),
    expires_at timestamptz NOT NULL,
    acknowledged_at timestamptz,
    actual_path text,
    error_code text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX ai_ui_actions_pending_client_idx
    ON ai.ui_actions (client_instance_id, created_at)
    WHERE status = 'pending';

-- Dropped preference/delivery rows and retired theme selections cannot be
-- reconstructed. This rollback restores only the schema required by the
-- previous application version.
