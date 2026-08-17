ALTER TABLE ai.tool_calls
    DROP CONSTRAINT IF EXISTS ai_tool_calls_input_mode_valid;

ALTER TABLE ai.tool_calls
    DROP COLUMN IF EXISTS input_mode;
