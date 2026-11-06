-- Restores the non-null policy snapshot columns. Observations recorded without
-- a render-time policy receive the previous column defaults.

ALTER TABLE public.runtime_observations
    DROP CONSTRAINT runtime_observations_policy_complete;

UPDATE public.runtime_observations
SET cpu_request_percent = 10, memory_request_percent = 25, cpu_limit_percent = 100, memory_limit_percent = 100
WHERE cpu_request_percent IS NULL;

ALTER TABLE public.runtime_observations
    ALTER COLUMN cpu_request_percent SET DEFAULT 10,
    ALTER COLUMN cpu_request_percent SET NOT NULL,
    ALTER COLUMN memory_request_percent SET DEFAULT 25,
    ALTER COLUMN memory_request_percent SET NOT NULL,
    ALTER COLUMN cpu_limit_percent SET DEFAULT 100,
    ALTER COLUMN cpu_limit_percent SET NOT NULL,
    ALTER COLUMN memory_limit_percent SET DEFAULT 100,
    ALTER COLUMN memory_limit_percent SET NOT NULL;
