ALTER TABLE ai.tool_calls
    ADD COLUMN IF NOT EXISTS input_mode text NOT NULL DEFAULT 'model';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'ai_tool_calls_input_mode_valid'
          AND conrelid = 'ai.tool_calls'::regclass
    ) THEN
        ALTER TABLE ai.tool_calls
            ADD CONSTRAINT ai_tool_calls_input_mode_valid
            CHECK (input_mode IN ('model', 'direct'));
    END IF;
END
$$;
