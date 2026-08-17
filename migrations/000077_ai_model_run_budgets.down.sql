DROP TABLE IF EXISTS ai.model_budget_reservations;
ALTER TABLE ai.runs DROP CONSTRAINT IF EXISTS ai_runs_budget_snapshot_valid;
ALTER TABLE ai.runs DROP COLUMN IF EXISTS total_credit_budget;
ALTER TABLE ai.runs DROP COLUMN IF EXISTS total_token_budget;
ALTER TABLE ai.runs DROP COLUMN IF EXISTS max_output_tokens;
ALTER TABLE ai.runs DROP COLUMN IF EXISTS max_context_tokens;
ALTER TABLE ai_models DROP CONSTRAINT IF EXISTS ai_models_token_limits_valid;
ALTER TABLE ai_models DROP COLUMN IF EXISTS max_output_tokens;
ALTER TABLE ai_models DROP COLUMN IF EXISTS max_context_tokens;
