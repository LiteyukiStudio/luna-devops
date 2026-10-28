ALTER TABLE ai.run_checkpoints
    DROP CONSTRAINT ai_run_checkpoints_phase_valid;

UPDATE ai.run_checkpoints
SET phase = 'model',
    state = state - 'pendingVerifications'
WHERE phase IN ('verification', 'waiting_external');

ALTER TABLE ai.run_checkpoints
    ADD CONSTRAINT ai_run_checkpoints_phase_valid CHECK ((phase = ANY (ARRAY['model'::text, 'tool_execution'::text, 'awaiting_input'::text, 'awaiting_approval'::text, 'finalizing'::text])));

ALTER TABLE ai.tool_calls
    DROP CONSTRAINT ai_tool_calls_input_mode_valid;

UPDATE ai.tool_calls
SET input_mode = 'model'
WHERE input_mode = 'verification';

ALTER TABLE ai.tool_calls
    ADD CONSTRAINT ai_tool_calls_input_mode_valid CHECK ((input_mode = ANY (ARRAY['model'::text, 'direct'::text])));

ALTER TABLE ai.tool_calls
    DROP COLUMN verification_for_tool_call_id,
    DROP COLUMN idempotency_key;
