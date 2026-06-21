ALTER TABLE deployment_targets DROP COLUMN IF EXISTS stage;
ALTER TABLE environments ADD COLUMN IF NOT EXISTS stage text NOT NULL DEFAULT 'dev';

