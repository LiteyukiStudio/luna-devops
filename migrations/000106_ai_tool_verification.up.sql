ALTER TABLE ai.tool_calls
    DROP CONSTRAINT ai_tool_calls_input_mode_valid;

ALTER TABLE ai.tool_calls
    ADD COLUMN idempotency_key text DEFAULT ''::text NOT NULL;

ALTER TABLE ai.tool_calls
    ADD COLUMN verification_for_tool_call_id text DEFAULT ''::text NOT NULL;

UPDATE ai.tool_calls
SET idempotency_key = id
WHERE idempotency_key = '';

ALTER TABLE ai.tool_calls
    ADD CONSTRAINT ai_tool_calls_input_mode_valid CHECK ((input_mode = ANY (ARRAY['model'::text, 'direct'::text, 'verification'::text])));

ALTER TABLE ai.run_checkpoints
    DROP CONSTRAINT ai_run_checkpoints_phase_valid;

ALTER TABLE ai.run_checkpoints
    ADD CONSTRAINT ai_run_checkpoints_phase_valid CHECK ((phase = ANY (ARRAY['model'::text, 'tool_execution'::text, 'verification'::text, 'awaiting_input'::text, 'awaiting_approval'::text, 'waiting_external'::text, 'finalizing'::text])));

COMMENT ON COLUMN ai.tool_calls.idempotency_key IS 'Stable idempotency key shared by retries of one logical side-effect tool call.';
COMMENT ON COLUMN ai.tool_calls.verification_for_tool_call_id IS 'Source write ToolCall verified by this authoritative readback; empty for model and direct calls.';
