ALTER TABLE ai.runs
    ADD COLUMN client_instance_id text;

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
