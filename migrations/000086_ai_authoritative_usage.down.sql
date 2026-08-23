DROP TABLE ai.model_usages;
DROP TABLE ai.model_credit_holds;

ALTER TABLE ai.runs DROP CONSTRAINT ai_runs_model_prices_non_negative;
ALTER TABLE ai.runs ADD COLUMN cached_output_credits_per_million numeric(24,8);
ALTER TABLE ai.runs ADD CONSTRAINT ai_runs_model_prices_non_negative CHECK (
    (input_credits_per_million IS NULL AND output_credits_per_million IS NULL
        AND cached_input_credits_per_million IS NULL AND cached_output_credits_per_million IS NULL)
    OR (
        input_credits_per_million IS NOT NULL AND output_credits_per_million IS NOT NULL
        AND cached_input_credits_per_million IS NOT NULL AND cached_output_credits_per_million IS NOT NULL
        AND input_credits_per_million >= 0 AND output_credits_per_million >= 0
        AND cached_input_credits_per_million >= 0 AND cached_output_credits_per_million >= 0
    )
);

ALTER TABLE ai_models DROP CONSTRAINT ai_models_prices_non_negative;
ALTER TABLE ai_models ADD COLUMN cached_output_credits_per_million numeric(24,8) NOT NULL DEFAULT 0;
ALTER TABLE ai_models ADD CONSTRAINT ai_models_prices_non_negative CHECK (
    input_credits_per_million >= 0 AND output_credits_per_million >= 0
    AND cached_input_credits_per_million >= 0 AND cached_output_credits_per_million >= 0
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

INSERT INTO app_configs(key, value)
SELECT entry.key, entry.value
FROM (VALUES
    ('ai.runtime.context_input_k_tokens', '1024'),
    ('ai.context.summary_input_k_tokens', '256')
) AS entry(key, value)
WHERE NOT EXISTS (SELECT 1 FROM app_configs WHERE app_configs.key = entry.key);
