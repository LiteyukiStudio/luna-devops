ALTER TABLE deployment_targets
    ADD COLUMN IF NOT EXISTS build_cpu_request text NOT NULL DEFAULT '1',
    ADD COLUMN IF NOT EXISTS build_memory_request text NOT NULL DEFAULT '1Gi';

ALTER TABLE build_runs
    ADD COLUMN IF NOT EXISTS build_cpu_request text NOT NULL DEFAULT '1',
    ADD COLUMN IF NOT EXISTS build_memory_request text NOT NULL DEFAULT '1Gi';
