ALTER TABLE public.deployment_targets
    ADD COLUMN delete_code text DEFAULT ''::text NOT NULL;

COMMENT ON COLUMN public.deployment_targets.delete_code IS 'Stable failure code of the latest deployment target deletion; empty unless delete_status is delete_failed.';
