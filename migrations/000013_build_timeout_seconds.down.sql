ALTER TABLE IF EXISTS build_runs
  DROP COLUMN IF EXISTS build_timeout_seconds;

ALTER TABLE IF EXISTS deployment_targets
  DROP COLUMN IF EXISTS build_timeout_seconds;
