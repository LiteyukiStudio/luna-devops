DROP TABLE ai.model_budget_reservations;

ALTER TABLE ai_models DROP CONSTRAINT ai_models_prices_non_negative;
ALTER TABLE ai_models DROP COLUMN cached_output_credits_per_million;
ALTER TABLE ai_models ADD CONSTRAINT ai_models_prices_non_negative CHECK (
    input_credits_per_million >= 0
    AND output_credits_per_million >= 0
    AND cached_input_credits_per_million >= 0
);

ALTER TABLE ai.runs DROP CONSTRAINT ai_runs_model_prices_non_negative;
ALTER TABLE ai.runs DROP COLUMN cached_output_credits_per_million;
ALTER TABLE ai.runs ADD CONSTRAINT ai_runs_model_prices_non_negative CHECK (
    (input_credits_per_million IS NULL AND output_credits_per_million IS NULL AND cached_input_credits_per_million IS NULL)
    OR (
        input_credits_per_million IS NOT NULL
        AND output_credits_per_million IS NOT NULL
        AND cached_input_credits_per_million IS NOT NULL
        AND input_credits_per_million >= 0
        AND output_credits_per_million >= 0
        AND cached_input_credits_per_million >= 0
    )
);

CREATE TABLE ai.model_credit_holds (
    id text PRIMARY KEY,
    run_id text NOT NULL REFERENCES ai.runs(id) ON DELETE CASCADE,
    owner_user_id text NOT NULL,
    operation text NOT NULL CHECK (operation IN ('assistant', 'summary', 'title')),
    attempt integer NOT NULL CHECK (attempt > 0),
    state text NOT NULL CHECK (state IN (
        'held', 'released', 'usage_recorded', 'hold_deficit',
        'reconciliation_required', 'settled'
    )),
    model_id text NOT NULL,
    model_name text NOT NULL,
    max_context_tokens_snapshot bigint NOT NULL CHECK (max_context_tokens_snapshot > 0),
    max_output_tokens_snapshot bigint NOT NULL CHECK (max_output_tokens_snapshot > 0),
    input_credits_per_million numeric(24,8) NOT NULL CHECK (input_credits_per_million >= 0),
    output_credits_per_million numeric(24,8) NOT NULL CHECK (output_credits_per_million >= 0),
    cached_input_credits_per_million numeric(24,8) NOT NULL CHECK (cached_input_credits_per_million >= 0),
    max_risk_credits numeric(24,8) NOT NULL CHECK (max_risk_credits >= 0),
    actual_credits numeric(24,8) CHECK (actual_credits IS NULL OR actual_credits >= 0),
    provider_request_id text,
    response_id text,
    response_model text,
    failure_stage text,
    reconciliation_reason text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    UNIQUE (run_id, operation, attempt),
    CONSTRAINT ai_model_credit_holds_state_valid CHECK (
        (state IN ('held', 'released') AND actual_credits IS NULL)
        OR (state IN ('usage_recorded', 'hold_deficit', 'settled') AND actual_credits IS NOT NULL)
        OR state = 'reconciliation_required'
    )
);
CREATE INDEX ai_model_credit_holds_owner_state_idx
    ON ai.model_credit_holds(owner_user_id, state);
CREATE INDEX ai_model_credit_holds_expiry_idx
    ON ai.model_credit_holds(expires_at) WHERE state = 'held';

CREATE TABLE ai.model_usages (
    id text PRIMARY KEY,
    credit_hold_id text NOT NULL UNIQUE REFERENCES ai.model_credit_holds(id) ON DELETE RESTRICT,
    run_id text NOT NULL REFERENCES ai.runs(id) ON DELETE CASCADE,
    owner_user_id text NOT NULL,
    operation text NOT NULL CHECK (operation IN ('assistant', 'summary', 'title')),
    attempt integer NOT NULL CHECK (attempt > 0),
    status text NOT NULL CHECK (status = 'reported'),
    settlement_status text NOT NULL CHECK (settlement_status IN ('pending', 'reconciliation_required', 'settled')),
    model_id text NOT NULL,
    model_name text NOT NULL,
    max_context_tokens_snapshot bigint NOT NULL CHECK (max_context_tokens_snapshot > 0),
    prompt_tokens bigint NOT NULL CHECK (prompt_tokens >= 0),
    completion_tokens bigint NOT NULL CHECK (completion_tokens >= 0),
    total_tokens bigint NOT NULL CHECK (total_tokens >= 0),
    cached_prompt_tokens bigint CHECK (cached_prompt_tokens IS NULL OR cached_prompt_tokens >= 0),
    cache_write_prompt_tokens bigint CHECK (cache_write_prompt_tokens IS NULL OR cache_write_prompt_tokens >= 0),
    reasoning_completion_tokens bigint CHECK (reasoning_completion_tokens IS NULL OR reasoning_completion_tokens >= 0),
    provider_request_id text,
    response_id text,
    response_model text,
    finish_reason text,
    call_type text NOT NULL CHECK (call_type IN ('stream', 'complete')),
    official_details jsonb NOT NULL DEFAULT '{}'::jsonb,
    occurred_at timestamptz NOT NULL DEFAULT now(),
    settled_at timestamptz,
    CONSTRAINT ai_model_usages_official_relationships_valid CHECK (
        total_tokens = prompt_tokens + completion_tokens
        AND (cached_prompt_tokens IS NULL OR cached_prompt_tokens <= prompt_tokens)
        AND (cache_write_prompt_tokens IS NULL OR cache_write_prompt_tokens <= prompt_tokens)
        AND (COALESCE(cached_prompt_tokens, 0) + COALESCE(cache_write_prompt_tokens, 0) <= prompt_tokens)
        AND (reasoning_completion_tokens IS NULL OR reasoning_completion_tokens <= completion_tokens)
    ),
    UNIQUE (run_id, operation, attempt)
);
CREATE INDEX ai_model_usages_settlement_idx
    ON ai.model_usages(settlement_status, occurred_at);
CREATE INDEX ai_model_usages_run_idx
    ON ai.model_usages(run_id, operation, occurred_at DESC);

DELETE FROM app_configs
WHERE key IN ('ai.runtime.context_input_k_tokens', 'ai.context.summary_input_k_tokens');

COMMENT ON TABLE ai.model_credit_holds IS
    'Credit risk holds per provider attempt. Token columns are intentionally absent.';
COMMENT ON TABLE ai.model_usages IS
    'Authoritative usage rows created only from schema-validated provider-reported usage.';
