ALTER TABLE deployment_targets
    ADD COLUMN IF NOT EXISTS build_environment_id text NOT NULL DEFAULT '';

UPDATE deployment_targets
SET build_environment_id = environment_id
WHERE build_environment_id = '';

ALTER TABLE build_runs
    ADD COLUMN IF NOT EXISTS build_environment_id text NOT NULL DEFAULT '';

UPDATE build_runs
SET build_environment_id = deployment_targets.build_environment_id
FROM deployment_targets
WHERE build_runs.deployment_target_id = deployment_targets.id
  AND build_runs.build_environment_id = '';
