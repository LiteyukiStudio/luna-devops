ALTER TABLE ai.tool_calls
    DROP COLUMN IF EXISTS approval_preview,
    DROP COLUMN IF EXISTS preview_lock;
