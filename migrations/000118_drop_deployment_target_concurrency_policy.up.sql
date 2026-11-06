-- The deployment target concurrency policy was stored but never consumed by
-- the build or release pipeline, so the column is removed.

ALTER TABLE public.deployment_targets
    DROP COLUMN concurrency_policy;
