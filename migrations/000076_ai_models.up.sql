CREATE TABLE ai_models (
    id text PRIMARY KEY,
    name text NOT NULL,
    input_credits_per_million numeric(24,8) NOT NULL DEFAULT 0,
    output_credits_per_million numeric(24,8) NOT NULL DEFAULT 0,
    cached_input_credits_per_million numeric(24,8) NOT NULL DEFAULT 0,
    cached_output_credits_per_million numeric(24,8) NOT NULL DEFAULT 0,
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT ai_models_name_not_blank CHECK (btrim(name) <> ''),
    CONSTRAINT ai_models_prices_non_negative CHECK (
        input_credits_per_million >= 0
        AND output_credits_per_million >= 0
        AND cached_input_credits_per_million >= 0
        AND cached_output_credits_per_million >= 0
    )
);
CREATE UNIQUE INDEX ai_models_name_key ON ai_models (name);
CREATE INDEX ai_models_enabled_name_idx ON ai_models (enabled, name);

ALTER TABLE ai.turns ADD COLUMN model_id text;
ALTER TABLE ai.runs ADD COLUMN model_id text;
ALTER TABLE ai.runs ADD COLUMN model_name text;
ALTER TABLE ai.runs ADD COLUMN input_credits_per_million numeric(24,8);
ALTER TABLE ai.runs ADD COLUMN output_credits_per_million numeric(24,8);
ALTER TABLE ai.runs ADD COLUMN cached_input_credits_per_million numeric(24,8);
ALTER TABLE ai.runs ADD COLUMN cached_output_credits_per_million numeric(24,8);
ALTER TABLE ai.runs ADD CONSTRAINT ai_runs_model_prices_non_negative CHECK (
    (input_credits_per_million IS NULL AND output_credits_per_million IS NULL AND cached_input_credits_per_million IS NULL AND cached_output_credits_per_million IS NULL) OR (
        input_credits_per_million IS NOT NULL AND output_credits_per_million IS NOT NULL AND cached_input_credits_per_million IS NOT NULL AND cached_output_credits_per_million IS NOT NULL
        AND
        input_credits_per_million >= 0
        AND output_credits_per_million >= 0
        AND cached_input_credits_per_million >= 0
        AND cached_output_credits_per_million >= 0
    )
);

DO $$
DECLARE
    legacy_model text;
    legacy_input numeric;
    legacy_output numeric;
BEGIN
    SELECT NULLIF(btrim(value), '') INTO legacy_model
    FROM app_configs
    WHERE key = 'ai.provider.default_model';
    IF legacy_model IS NOT NULL THEN
        SELECT COALESCE((SELECT credits_per_unit FROM billing_rate_rules WHERE meter = 'ai.input_tokens_1000'), 1) * 1000
          INTO legacy_input;
        SELECT COALESCE((SELECT credits_per_unit FROM billing_rate_rules WHERE meter = 'ai.output_tokens_1000'), 4) * 1000
          INTO legacy_output;
        INSERT INTO ai_models (id, name, input_credits_per_million, output_credits_per_million, enabled)
        VALUES ('aimod_' || left(md5('ai-model:' || legacy_model), 24), legacy_model, legacy_input, legacy_output, true)
        ON CONFLICT (name) DO NOTHING;
    END IF;
END $$;

-- AI usage is now priced by the immutable model snapshot on each Run.
DELETE FROM billing_rate_rules
WHERE meter IN ('ai.input_tokens_1000', 'ai.output_tokens_1000');
