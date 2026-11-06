-- Restores the previous build timeout state and release state set. Failure
-- codes cannot be represented by the previous schema and are dropped.

UPDATE public.build_jobs
SET status = 'lost'
WHERE status = 'timeout';

UPDATE public.build_runs
SET status = 'lost'
WHERE status = 'timeout';

UPDATE public.releases
SET status = 'failed'
WHERE status = 'canceled';

ALTER TABLE public.releases
    DROP COLUMN failure_code;

ALTER TABLE public.build_jobs
    DROP COLUMN failure_code;

ALTER TABLE public.build_runs
    DROP COLUMN failure_code;
