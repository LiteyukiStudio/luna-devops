-- Template installation progress is derived from the Releases it created; the
-- persisted status/message columns were written once and never converged.
DROP INDEX IF EXISTS public.idx_app_template_installations_status;

ALTER TABLE public.app_template_installations
    DROP COLUMN status,
    DROP COLUMN message;
