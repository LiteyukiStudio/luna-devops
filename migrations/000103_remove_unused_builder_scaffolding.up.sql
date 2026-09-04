DROP INDEX IF EXISTS idx_build_jobs_builder_id;
DROP INDEX IF EXISTS idx_build_jobs_lease_token;
DROP INDEX IF EXISTS idx_build_jobs_lease_until;
DROP INDEX IF EXISTS idx_build_jobs_last_heartbeat_at;

ALTER TABLE build_jobs
    DROP COLUMN IF EXISTS type,
    DROP COLUMN IF EXISTS builder_id,
    DROP COLUMN IF EXISTS lease_token,
    DROP COLUMN IF EXISTS lease_until,
    DROP COLUMN IF EXISTS last_heartbeat_at;

ALTER TABLE build_runs
    DROP COLUMN IF EXISTS build_labels,
    DROP COLUMN IF EXISTS cache_config,
    DROP COLUMN IF EXISTS cpu_core_seconds,
    DROP COLUMN IF EXISTS memory_mb_seconds,
    DROP COLUMN IF EXISTS credit_cost;

ALTER TABLE deployment_targets
    DROP COLUMN IF EXISTS build_labels;
