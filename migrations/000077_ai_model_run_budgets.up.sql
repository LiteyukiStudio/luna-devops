ALTER TABLE ai_models
    ADD COLUMN max_context_tokens bigint NOT NULL DEFAULT 524288,
    ADD COLUMN max_output_tokens bigint NOT NULL DEFAULT 65536,
    ADD CONSTRAINT ai_models_token_limits_valid CHECK (
        max_context_tokens BETWEEN 4096 AND 2097152
        AND max_output_tokens BETWEEN 256 AND 262144
        AND max_output_tokens < max_context_tokens
    );

ALTER TABLE ai.runs
    ADD COLUMN max_context_tokens bigint,
    ADD COLUMN max_output_tokens bigint,
    ADD COLUMN total_token_budget bigint,
    ADD COLUMN total_credit_budget numeric(24,8),
    ADD CONSTRAINT ai_runs_budget_snapshot_valid CHECK (
        (max_context_tokens IS NULL OR max_context_tokens BETWEEN 4096 AND 2097152)
        AND (max_output_tokens IS NULL OR max_output_tokens BETWEEN 256 AND 262144)
        AND (max_context_tokens IS NULL OR max_output_tokens IS NULL OR max_output_tokens < max_context_tokens)
        AND (total_token_budget IS NULL OR total_token_budget > 0)
        AND (total_credit_budget IS NULL OR total_credit_budget > 0)
    );

CREATE TABLE ai.model_budget_reservations (
    id text PRIMARY KEY,
    run_id text NOT NULL REFERENCES ai.runs(id) ON DELETE CASCADE,
    owner_user_id text NOT NULL,
    operation text NOT NULL CHECK (operation IN ('assistant', 'summary', 'title', 'next_steps')),
    state text NOT NULL CHECK (state IN ('reserved', 'confirmed', 'released', 'settled')),
    model_id text NOT NULL,
    model_name text NOT NULL,
    input_credits_per_million numeric(24,8) NOT NULL CHECK (input_credits_per_million >= 0),
    output_credits_per_million numeric(24,8) NOT NULL CHECK (output_credits_per_million >= 0),
    cached_input_credits_per_million numeric(24,8) NOT NULL CHECK (cached_input_credits_per_million >= 0),
    cached_output_credits_per_million numeric(24,8) NOT NULL CHECK (cached_output_credits_per_million >= 0),
    reserved_tokens bigint NOT NULL CHECK (reserved_tokens > 0),
    reserved_input_tokens bigint NOT NULL CHECK (reserved_input_tokens > 0),
    reserved_output_tokens bigint NOT NULL CHECK (reserved_output_tokens > 0),
    confirmed_tokens bigint CHECK (confirmed_tokens IS NULL OR confirmed_tokens >= 0),
    reserved_credits numeric(24,8) NOT NULL CHECK (reserved_credits >= 0),
    confirmed_credits numeric(24,8) CHECK (confirmed_credits IS NULL OR confirmed_credits >= 0),
    input_tokens bigint CHECK (input_tokens IS NULL OR input_tokens >= 0),
    output_tokens bigint CHECK (output_tokens IS NULL OR output_tokens >= 0),
    cached_input_tokens bigint CHECK (cached_input_tokens IS NULL OR cached_input_tokens >= 0),
    cached_output_tokens bigint CHECK (cached_output_tokens IS NULL OR cached_output_tokens >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    CONSTRAINT ai_model_budget_reservations_usage_valid CHECK (
        reserved_tokens = reserved_input_tokens + reserved_output_tokens
        AND (confirmed_tokens IS NULL OR confirmed_tokens = input_tokens + output_tokens)
        AND (cached_input_tokens IS NULL OR input_tokens IS NOT NULL AND cached_input_tokens <= input_tokens)
        AND (cached_output_tokens IS NULL OR output_tokens IS NOT NULL AND cached_output_tokens <= output_tokens)
        AND (
            state IN ('reserved', 'released')
            OR (confirmed_tokens IS NOT NULL AND confirmed_credits IS NOT NULL
                AND input_tokens IS NOT NULL AND output_tokens IS NOT NULL
                AND cached_input_tokens IS NOT NULL AND cached_output_tokens IS NOT NULL)
        )
    )
);
CREATE INDEX ai_model_budget_reservations_run_idx
    ON ai.model_budget_reservations(run_id, state);
CREATE INDEX ai_model_budget_reservations_owner_idx
    ON ai.model_budget_reservations(owner_user_id, state);
CREATE INDEX ai_model_budget_reservations_expiry_idx
    ON ai.model_budget_reservations(owner_user_id, expires_at)
    WHERE state = 'reserved';

COMMENT ON TABLE ai.model_budget_reservations IS
    'Authoritative per-provider-call holds. Prompt or wallet balances never enter telemetry or event payloads.';
