-- Restores the concurrency policy column with its previous default. Values
-- stored before the up migration cannot be recovered.

ALTER TABLE public.deployment_targets
    ADD COLUMN concurrency_policy text DEFAULT 'queue'::text NOT NULL;
