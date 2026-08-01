ALTER TABLE ai.runs
    ADD COLUMN trace_context jsonb NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE ai.runs
    ADD CONSTRAINT ai_runs_trace_context_object_check
    CHECK (jsonb_typeof(trace_context) = 'object');
