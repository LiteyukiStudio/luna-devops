ALTER TABLE deployment_targets ADD COLUMN IF NOT EXISTS stage text NOT NULL DEFAULT 'prod';

UPDATE deployment_targets
SET stage = COALESCE(NULLIF(environments.stage, ''), 'prod')
FROM environments
WHERE deployment_targets.environment_id = environments.id;

ALTER TABLE environments DROP COLUMN IF EXISTS stage;

