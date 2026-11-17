ALTER TABLE ai.model_usages
    ADD COLUMN max_output_tokens_snapshot bigint NOT NULL DEFAULT 65536,
    ADD COLUMN input_credits_per_million numeric(24,8) NOT NULL DEFAULT 0,
    ADD COLUMN output_credits_per_million numeric(24,8) NOT NULL DEFAULT 0,
    ADD COLUMN cached_input_credits_per_million numeric(24,8) NOT NULL DEFAULT 0;

UPDATE ai.model_usages AS usage
SET max_output_tokens_snapshot = hold.max_output_tokens_snapshot,
    input_credits_per_million = hold.input_credits_per_million,
    output_credits_per_million = hold.output_credits_per_million,
    cached_input_credits_per_million = hold.cached_input_credits_per_million
FROM ai.model_credit_holds AS hold
WHERE hold.id = usage.credit_hold_id;

UPDATE ai.model_usages
SET settlement_status = 'pending'
WHERE settlement_status = 'reconciliation_required';

ALTER TABLE ai.model_usages
    ADD CONSTRAINT model_usages_max_output_tokens_snapshot_check CHECK ((max_output_tokens_snapshot > 0)),
    ADD CONSTRAINT model_usages_input_credits_per_million_check CHECK ((input_credits_per_million >= (0)::numeric)),
    ADD CONSTRAINT model_usages_output_credits_per_million_check CHECK ((output_credits_per_million >= (0)::numeric)),
    ADD CONSTRAINT model_usages_cached_input_credits_per_million_check CHECK ((cached_input_credits_per_million >= (0)::numeric)),
    DROP CONSTRAINT ai_model_usages_official_relationships_valid,
    ADD CONSTRAINT ai_model_usages_official_relationships_valid CHECK (
        (total_tokens >= (prompt_tokens + completion_tokens))
        AND ((cached_prompt_tokens IS NULL) OR (cached_prompt_tokens <= prompt_tokens))
        AND ((cache_write_prompt_tokens IS NULL) OR (cache_write_prompt_tokens <= prompt_tokens))
        AND ((COALESCE(cached_prompt_tokens, (0)::bigint) + COALESCE(cache_write_prompt_tokens, (0)::bigint)) <= prompt_tokens)
    ),
    DROP CONSTRAINT model_usages_settlement_status_check,
    ADD CONSTRAINT model_usages_settlement_status_check CHECK (
        (settlement_status = ANY (ARRAY['pending'::text, 'settled'::text]))
    );

ALTER TABLE ai.model_usages
    DROP CONSTRAINT IF EXISTS model_usages_credit_hold_id_fkey,
    DROP CONSTRAINT IF EXISTS model_usages_credit_hold_id_key,
    DROP CONSTRAINT IF EXISTS model_usages_run_id_fkey,
    DROP COLUMN credit_hold_id;

CREATE INDEX ai_model_usages_owner_settlement_idx ON ai.model_usages USING btree (owner_user_id, settlement_status);

DROP TABLE ai.model_credit_holds;
