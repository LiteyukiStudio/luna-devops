-- Runtime observations now record the resource policy annotated on the
-- rendered Pod template instead of the cluster's current policy. Workloads
-- rendered before the annotation existed have no recorded policy, so the
-- snapshot columns become nullable. Existing rows are kept unchanged.

ALTER TABLE public.runtime_observations
    ALTER COLUMN cpu_request_percent DROP DEFAULT,
    ALTER COLUMN cpu_request_percent DROP NOT NULL,
    ALTER COLUMN memory_request_percent DROP DEFAULT,
    ALTER COLUMN memory_request_percent DROP NOT NULL,
    ALTER COLUMN cpu_limit_percent DROP DEFAULT,
    ALTER COLUMN cpu_limit_percent DROP NOT NULL,
    ALTER COLUMN memory_limit_percent DROP DEFAULT,
    ALTER COLUMN memory_limit_percent DROP NOT NULL;

ALTER TABLE public.runtime_observations
    ADD CONSTRAINT runtime_observations_policy_complete CHECK (
        (cpu_request_percent IS NULL AND memory_request_percent IS NULL AND cpu_limit_percent IS NULL AND memory_limit_percent IS NULL)
        OR (cpu_request_percent IS NOT NULL AND memory_request_percent IS NOT NULL AND cpu_limit_percent IS NOT NULL AND memory_limit_percent IS NOT NULL)
    );
