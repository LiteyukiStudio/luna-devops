ALTER TABLE ai.runs DROP CONSTRAINT IF EXISTS ai_runs_budget_snapshot_valid;
ALTER TABLE ai.runs DROP COLUMN IF EXISTS total_credit_budget;
ALTER TABLE ai.runs DROP COLUMN IF EXISTS total_token_budget;
ALTER TABLE ai.runs
    ADD CONSTRAINT ai_runs_model_snapshot_valid CHECK (
        (max_context_tokens IS NULL OR max_context_tokens BETWEEN 4096 AND 2097152)
        AND (max_output_tokens IS NULL OR max_output_tokens BETWEEN 256 AND 262144)
        AND (max_context_tokens IS NULL OR max_output_tokens IS NULL OR max_output_tokens < max_context_tokens)
    );
