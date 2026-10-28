DROP INDEX IF EXISTS public.idx_releases_actor_idempotency;
DROP INDEX IF EXISTS public.idx_build_runs_actor_idempotency;

ALTER TABLE public.releases
    DROP COLUMN idempotency_request_hash,
    DROP COLUMN idempotency_key;

ALTER TABLE public.build_runs
    DROP COLUMN idempotency_request_hash,
    DROP COLUMN idempotency_key;
