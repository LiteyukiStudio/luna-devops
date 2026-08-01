CREATE TABLE IF NOT EXISTS inbox_action_requests (
    id text PRIMARY KEY,
    type text NOT NULL,
    requester_user_id text NOT NULL,
    recipient_user_id text NOT NULL,
    project_id text NOT NULL DEFAULT '',
    resource_type text NOT NULL DEFAULT '',
    resource_id text NOT NULL DEFAULT '',
    payload_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    status text NOT NULL DEFAULT 'pending',
    row_version bigint NOT NULL DEFAULT 1,
    expires_at timestamptz,
    responded_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT chk_inbox_action_requests_status CHECK (status IN ('pending', 'processing', 'completed', 'rejected', 'cancelled', 'expired', 'failed')),
    CONSTRAINT chk_inbox_action_requests_row_version CHECK (row_version > 0)
);

CREATE INDEX IF NOT EXISTS idx_inbox_action_requests_type ON inbox_action_requests(type);
CREATE INDEX IF NOT EXISTS idx_inbox_action_requests_requester_user_id ON inbox_action_requests(requester_user_id);
CREATE INDEX IF NOT EXISTS idx_inbox_action_requests_recipient_user_id ON inbox_action_requests(recipient_user_id);
CREATE INDEX IF NOT EXISTS idx_inbox_action_requests_project_id ON inbox_action_requests(project_id);
CREATE INDEX IF NOT EXISTS idx_inbox_action_requests_status ON inbox_action_requests(status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_inbox_action_requests_pending_project_type
    ON inbox_action_requests(project_id, type)
    WHERE project_id <> '' AND status IN ('pending', 'processing');

CREATE TABLE IF NOT EXISTS inbox_messages (
    id text PRIMARY KEY,
    recipient_user_id text NOT NULL,
    type text NOT NULL,
    category text NOT NULL,
    priority text NOT NULL,
    actor_id text NOT NULL DEFAULT '',
    project_id text NOT NULL DEFAULT '',
    resource_type text NOT NULL DEFAULT '',
    resource_id text NOT NULL DEFAULT '',
    title_key text NOT NULL,
    content_key text NOT NULL,
    params_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    action_request_id text NOT NULL DEFAULT '',
    deep_link text NOT NULL DEFAULT '',
    group_key text NOT NULL DEFAULT '',
    dedup_key text,
    read_at timestamptz,
    archived_at timestamptz,
    expires_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT chk_inbox_messages_category CHECK (category IN ('action', 'project', 'billing', 'security', 'delivery', 'system')),
    CONSTRAINT chk_inbox_messages_priority CHECK (priority IN ('low', 'normal', 'high', 'critical'))
);

CREATE INDEX IF NOT EXISTS idx_inbox_messages_recipient_user_id ON inbox_messages(recipient_user_id);
CREATE INDEX IF NOT EXISTS idx_inbox_messages_type ON inbox_messages(type);
CREATE INDEX IF NOT EXISTS idx_inbox_messages_category ON inbox_messages(category);
CREATE INDEX IF NOT EXISTS idx_inbox_messages_priority ON inbox_messages(priority);
CREATE INDEX IF NOT EXISTS idx_inbox_messages_project_id ON inbox_messages(project_id);
CREATE INDEX IF NOT EXISTS idx_inbox_messages_action_request_id ON inbox_messages(action_request_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_inbox_messages_dedup_key ON inbox_messages(dedup_key) WHERE dedup_key IS NOT NULL;
