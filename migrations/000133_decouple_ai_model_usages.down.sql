CREATE TABLE ai.model_credit_holds (
    id text NOT NULL,
    run_id text NOT NULL,
    owner_user_id text NOT NULL,
    operation text NOT NULL,
    attempt integer NOT NULL,
    state text NOT NULL,
    model_id text NOT NULL,
    model_name text NOT NULL,
    max_context_tokens_snapshot bigint NOT NULL,
    max_output_tokens_snapshot bigint NOT NULL,
    input_credits_per_million numeric(24,8) NOT NULL,
    output_credits_per_million numeric(24,8) NOT NULL,
    cached_input_credits_per_million numeric(24,8) NOT NULL,
    max_risk_credits numeric(24,8) NOT NULL,
    actual_credits numeric(24,8),
    provider_request_id text,
    response_id text,
    response_model text,
    failure_stage text,
    reconciliation_reason text,
    expires_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT ai_model_credit_holds_state_valid CHECK ((((state = ANY (ARRAY['held'::text, 'released'::text])) AND (actual_credits IS NULL)) OR ((state = ANY (ARRAY['usage_recorded'::text, 'hold_deficit'::text, 'settled'::text])) AND (actual_credits IS NOT NULL)) OR (state = 'reconciliation_required'::text))),
    CONSTRAINT model_credit_holds_actual_credits_check CHECK (((actual_credits IS NULL) OR (actual_credits >= (0)::numeric))),
    CONSTRAINT model_credit_holds_attempt_check CHECK ((attempt > 0)),
    CONSTRAINT model_credit_holds_cached_input_credits_per_million_check CHECK ((cached_input_credits_per_million >= (0)::numeric)),
    CONSTRAINT model_credit_holds_input_credits_per_million_check CHECK ((input_credits_per_million >= (0)::numeric)),
    CONSTRAINT model_credit_holds_max_context_tokens_snapshot_check CHECK ((max_context_tokens_snapshot > 0)),
    CONSTRAINT model_credit_holds_max_output_tokens_snapshot_check CHECK ((max_output_tokens_snapshot > 0)),
    CONSTRAINT model_credit_holds_max_risk_credits_check CHECK ((max_risk_credits >= (0)::numeric)),
    CONSTRAINT model_credit_holds_operation_check CHECK ((operation = ANY (ARRAY['assistant'::text, 'summary'::text, 'title'::text]))),
    CONSTRAINT model_credit_holds_output_credits_per_million_check CHECK ((output_credits_per_million >= (0)::numeric)),
    CONSTRAINT model_credit_holds_state_check CHECK ((state = ANY (ARRAY['held'::text, 'released'::text, 'usage_recorded'::text, 'hold_deficit'::text, 'reconciliation_required'::text, 'settled'::text])))
);

COMMENT ON TABLE ai.model_credit_holds IS 'Credit risk holds per provider attempt. Token columns are intentionally absent.';

ALTER TABLE ONLY ai.model_credit_holds
    ADD CONSTRAINT model_credit_holds_pkey PRIMARY KEY (id);

ALTER TABLE ONLY ai.model_credit_holds
    ADD CONSTRAINT model_credit_holds_run_id_operation_attempt_key UNIQUE (run_id, operation, attempt);

CREATE INDEX ai_model_credit_holds_expiry_idx ON ai.model_credit_holds USING btree (expires_at) WHERE (state = 'held'::text);

CREATE INDEX ai_model_credit_holds_owner_state_idx ON ai.model_credit_holds USING btree (owner_user_id, state);

ALTER TABLE ONLY ai.model_credit_holds
    ADD CONSTRAINT model_credit_holds_run_id_fkey FOREIGN KEY (run_id) REFERENCES ai.runs(id) ON DELETE CASCADE;

DELETE FROM ai.model_usages AS usage
WHERE NOT EXISTS (SELECT 1 FROM ai.runs AS run WHERE run.id = usage.run_id)
   OR usage.total_tokens <> (usage.prompt_tokens + usage.completion_tokens)
   OR (usage.reasoning_completion_tokens IS NOT NULL AND usage.reasoning_completion_tokens > usage.completion_tokens);

INSERT INTO ai.model_credit_holds (
    id, run_id, owner_user_id, operation, attempt, state, model_id, model_name,
    max_context_tokens_snapshot, max_output_tokens_snapshot,
    input_credits_per_million, output_credits_per_million, cached_input_credits_per_million,
    max_risk_credits, actual_credits, provider_request_id, response_id, response_model,
    expires_at, created_at, updated_at
)
SELECT
    'aihold_' || usage.id,
    usage.run_id,
    usage.owner_user_id,
    usage.operation,
    usage.attempt,
    CASE WHEN usage.settlement_status = 'settled' THEN 'settled' ELSE 'usage_recorded' END,
    usage.model_id,
    usage.model_name,
    usage.max_context_tokens_snapshot,
    usage.max_output_tokens_snapshot,
    usage.input_credits_per_million,
    usage.output_credits_per_million,
    usage.cached_input_credits_per_million,
    (((usage.prompt_tokens - COALESCE(usage.cached_prompt_tokens, 0) - COALESCE(usage.cache_write_prompt_tokens, 0))::numeric * usage.input_credits_per_million)
     + ((COALESCE(usage.cached_prompt_tokens, 0) + COALESCE(usage.cache_write_prompt_tokens, 0))::numeric * usage.cached_input_credits_per_million)
     + (usage.completion_tokens::numeric * usage.output_credits_per_million)) / 1000000,
    (((usage.prompt_tokens - COALESCE(usage.cached_prompt_tokens, 0) - COALESCE(usage.cache_write_prompt_tokens, 0))::numeric * usage.input_credits_per_million)
     + ((COALESCE(usage.cached_prompt_tokens, 0) + COALESCE(usage.cache_write_prompt_tokens, 0))::numeric * usage.cached_input_credits_per_million)
     + (usage.completion_tokens::numeric * usage.output_credits_per_million)) / 1000000,
    usage.provider_request_id,
    usage.response_id,
    usage.response_model,
    usage.occurred_at + interval '1 hour',
    usage.occurred_at,
    COALESCE(usage.settled_at, usage.occurred_at)
FROM ai.model_usages AS usage;

DROP INDEX IF EXISTS ai.ai_model_usages_owner_settlement_idx;

ALTER TABLE ai.model_usages
    ADD COLUMN credit_hold_id text;

UPDATE ai.model_usages
SET credit_hold_id = 'aihold_' || id;

ALTER TABLE ai.model_usages
    ALTER COLUMN credit_hold_id SET NOT NULL,
    ADD CONSTRAINT model_usages_credit_hold_id_key UNIQUE (credit_hold_id),
    ADD CONSTRAINT model_usages_credit_hold_id_fkey FOREIGN KEY (credit_hold_id) REFERENCES ai.model_credit_holds(id) ON DELETE RESTRICT,
    ADD CONSTRAINT model_usages_run_id_fkey FOREIGN KEY (run_id) REFERENCES ai.runs(id) ON DELETE CASCADE,
    DROP CONSTRAINT ai_model_usages_official_relationships_valid,
    ADD CONSTRAINT ai_model_usages_official_relationships_valid CHECK ((((total_tokens = (prompt_tokens + completion_tokens)) AND ((cached_prompt_tokens IS NULL) OR (cached_prompt_tokens <= prompt_tokens)) AND ((cache_write_prompt_tokens IS NULL) OR (cache_write_prompt_tokens <= prompt_tokens)) AND ((COALESCE(cached_prompt_tokens, (0)::bigint) + COALESCE(cache_write_prompt_tokens, (0)::bigint)) <= prompt_tokens) AND ((reasoning_completion_tokens IS NULL) OR (reasoning_completion_tokens <= completion_tokens))))),
    DROP CONSTRAINT model_usages_settlement_status_check,
    ADD CONSTRAINT model_usages_settlement_status_check CHECK ((settlement_status = ANY (ARRAY['pending'::text, 'reconciliation_required'::text, 'settled'::text]))),
    DROP CONSTRAINT model_usages_max_output_tokens_snapshot_check,
    DROP CONSTRAINT model_usages_input_credits_per_million_check,
    DROP CONSTRAINT model_usages_output_credits_per_million_check,
    DROP CONSTRAINT model_usages_cached_input_credits_per_million_check,
    DROP COLUMN max_output_tokens_snapshot,
    DROP COLUMN input_credits_per_million,
    DROP COLUMN output_credits_per_million,
    DROP COLUMN cached_input_credits_per_million;
