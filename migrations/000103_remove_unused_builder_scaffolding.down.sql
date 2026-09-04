ALTER TABLE deployment_targets
    ADD COLUMN IF NOT EXISTS build_labels text NOT NULL DEFAULT '';

ALTER TABLE build_runs
    ADD COLUMN IF NOT EXISTS build_labels text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS cache_config text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS cpu_core_seconds bigint NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS memory_mb_seconds bigint NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS credit_cost bigint NOT NULL DEFAULT 0;

ALTER TABLE build_jobs
    ADD COLUMN IF NOT EXISTS type text NOT NULL DEFAULT 'build',
    ADD COLUMN IF NOT EXISTS builder_id text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS lease_token text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS lease_until timestamp with time zone,
    ADD COLUMN IF NOT EXISTS last_heartbeat_at timestamp with time zone;

CREATE INDEX IF NOT EXISTS idx_build_jobs_builder_id ON build_jobs USING btree (builder_id);
CREATE INDEX IF NOT EXISTS idx_build_jobs_lease_token ON build_jobs USING btree (lease_token);
CREATE INDEX IF NOT EXISTS idx_build_jobs_lease_until ON build_jobs USING btree (lease_until);
CREATE INDEX IF NOT EXISTS idx_build_jobs_last_heartbeat_at ON build_jobs USING btree (last_heartbeat_at);
