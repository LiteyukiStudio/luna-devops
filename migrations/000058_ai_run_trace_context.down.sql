ALTER TABLE ai.runs
    DROP CONSTRAINT IF EXISTS ai_runs_trace_context_object_check,
    DROP COLUMN IF EXISTS trace_context;
