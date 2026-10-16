-- Durable Agent run control-flow state. The checkpoint is the authoritative
-- resume fact for luna-agent: it records the run phase, the committed model
-- step and tool call counters, and the executor-owned state that must survive
-- a process restart. Timeline items stay a user-visible projection and
-- ai.tool_calls stays the tool execution fact.

CREATE TABLE ai.run_checkpoints (
    run_id text NOT NULL,
    revision bigint DEFAULT 1 NOT NULL,
    phase text NOT NULL,
    model_step integer DEFAULT 0 NOT NULL,
    tool_call_count integer DEFAULT 0 NOT NULL,
    recoveries integer DEFAULT 0 NOT NULL,
    state jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ai_run_checkpoints_counters_check CHECK (((revision > 0) AND (model_step >= 0) AND (tool_call_count >= 0) AND (recoveries >= 0))),
    CONSTRAINT ai_run_checkpoints_phase_valid CHECK ((phase = ANY (ARRAY['model'::text, 'tool_execution'::text, 'awaiting_input'::text, 'awaiting_approval'::text, 'finalizing'::text]))),
    CONSTRAINT ai_run_checkpoints_state_object_check CHECK ((jsonb_typeof(state) = 'object'::text))
);

COMMENT ON TABLE ai.run_checkpoints IS 'Authoritative Agent run control-flow state; resume reads this instead of inferring position from ai.items or in-process memory.';

COMMENT ON COLUMN ai.run_checkpoints.revision IS 'Optimistic lock for control-flow advances; a replayed write of the same revision is idempotent and an older revision can never overwrite a newer state.';

COMMENT ON COLUMN ai.run_checkpoints.recoveries IS 'Bounded number of startup recoveries for this run; written by restart reconciliation, never by the executor.';

ALTER TABLE ONLY ai.run_checkpoints
    ADD CONSTRAINT run_checkpoints_pkey PRIMARY KEY (run_id);

ALTER TABLE ONLY ai.run_checkpoints
    ADD CONSTRAINT run_checkpoints_run_id_fkey FOREIGN KEY (run_id) REFERENCES ai.runs(id) ON DELETE CASCADE;

ALTER TABLE ai.tool_calls
    ADD COLUMN execution_count integer DEFAULT 0 NOT NULL CHECK (execution_count >= 0);
