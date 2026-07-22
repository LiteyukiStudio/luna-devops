ALTER TABLE build_runs
    DROP COLUMN IF EXISTS build_secret_refs_snapshot,
    DROP COLUMN IF EXISTS build_variables_snapshot;

DROP TABLE IF EXISTS build_environment_configs;
