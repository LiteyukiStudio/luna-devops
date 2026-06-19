ALTER TABLE build_runs
    DROP COLUMN IF EXISTS build_environment_id;

ALTER TABLE deployment_targets
    DROP COLUMN IF EXISTS build_environment_id;
