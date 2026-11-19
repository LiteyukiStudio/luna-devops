ALTER TABLE ai.tool_calls
    ADD COLUMN IF NOT EXISTS preview_lock text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS approval_preview jsonb;
