ALTER TABLE IF EXISTS deployment_targets
  ADD COLUMN IF NOT EXISTS build_timeout_seconds integer NOT NULL DEFAULT 1800;

ALTER TABLE IF EXISTS build_runs
  ADD COLUMN IF NOT EXISTS build_timeout_seconds integer NOT NULL DEFAULT 1800;

UPDATE deployment_targets
SET build_timeout_seconds = 1800
WHERE build_timeout_seconds <= 0;

UPDATE build_runs
SET build_timeout_seconds = 1800
WHERE build_timeout_seconds <= 0;
