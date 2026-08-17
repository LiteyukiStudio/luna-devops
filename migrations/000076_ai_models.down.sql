DELETE FROM ai_models
WHERE id LIKE 'aimod_%';
ALTER TABLE ai.runs DROP CONSTRAINT IF EXISTS ai_runs_model_prices_non_negative;
ALTER TABLE ai.runs DROP COLUMN IF EXISTS cached_output_credits_per_million;
ALTER TABLE ai.runs DROP COLUMN IF EXISTS cached_input_credits_per_million;
ALTER TABLE ai.runs DROP COLUMN IF EXISTS output_credits_per_million;
ALTER TABLE ai.runs DROP COLUMN IF EXISTS input_credits_per_million;
ALTER TABLE ai.runs DROP COLUMN IF EXISTS model_name;
ALTER TABLE ai.runs DROP COLUMN IF EXISTS model_id;
ALTER TABLE ai.turns DROP COLUMN IF EXISTS model_id;
DROP TABLE IF EXISTS ai_models;
