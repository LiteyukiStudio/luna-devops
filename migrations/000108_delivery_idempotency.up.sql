ALTER TABLE public.build_runs
    ADD COLUMN idempotency_key text DEFAULT ''::text NOT NULL,
    ADD COLUMN idempotency_request_hash text DEFAULT ''::text NOT NULL;

ALTER TABLE public.releases
    ADD COLUMN idempotency_key text DEFAULT ''::text NOT NULL,
    ADD COLUMN idempotency_request_hash text DEFAULT ''::text NOT NULL;

CREATE UNIQUE INDEX idx_build_runs_actor_idempotency
    ON public.build_runs (created_by, idempotency_key)
    WHERE idempotency_key <> '';

CREATE UNIQUE INDEX idx_releases_actor_idempotency
    ON public.releases (created_by, idempotency_key)
    WHERE idempotency_key <> '';

COMMENT ON COLUMN public.build_runs.idempotency_key IS 'Scoped delivery command key; empty for legacy runs and non-idempotent producers.';
COMMENT ON COLUMN public.releases.idempotency_key IS 'Scoped delivery command key; empty for legacy releases and non-idempotent producers.';
