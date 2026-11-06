-- Delivery workflows record a stable failure code next to the terminal state.
-- The raw error text stays in job messages, release logs and traces.

ALTER TABLE public.build_runs
    ADD COLUMN failure_code text DEFAULT ''::text NOT NULL;

ALTER TABLE public.build_jobs
    ADD COLUMN failure_code text DEFAULT ''::text NOT NULL;

ALTER TABLE public.releases
    ADD COLUMN failure_code text DEFAULT ''::text NOT NULL;

-- "lost" was only written by build timeout reclamation; timeouts now use the
-- dedicated timeout state.
UPDATE public.build_jobs
SET status = 'timeout', failure_code = 'timeout'
WHERE status = 'lost';

UPDATE public.build_runs
SET status = 'timeout', failure_code = 'timeout'
WHERE status = 'lost';

UPDATE public.build_jobs
SET failure_code = 'canceled_by_user'
WHERE status = 'canceled' AND failure_code = '';

UPDATE public.build_runs
SET failure_code = 'canceled_by_user'
WHERE status = 'canceled' AND failure_code = '';

COMMENT ON COLUMN public.build_runs.failure_code IS 'Stable failure code for terminal non-success states; empty while active or succeeded.';
COMMENT ON COLUMN public.build_jobs.failure_code IS 'Stable failure code for terminal non-success states; empty while active or succeeded.';
COMMENT ON COLUMN public.releases.failure_code IS 'Stable failure code for terminal non-success states; empty while active or succeeded.';
