ALTER TABLE build_runs
    DROP COLUMN IF EXISTS build_memory_request,
    DROP COLUMN IF EXISTS build_cpu_request;

ALTER TABLE deployment_targets
    DROP COLUMN IF EXISTS build_memory_request,
    DROP COLUMN IF EXISTS build_cpu_request;
