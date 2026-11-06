ALTER TABLE public.applications
    ADD COLUMN delete_code text DEFAULT ''::text NOT NULL;

COMMENT ON COLUMN public.applications.delete_code IS 'Stable failure code of the latest application deletion; empty unless delete_status is delete_failed.';
