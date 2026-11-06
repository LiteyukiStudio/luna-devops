ALTER TABLE public.app_template_installations
    ADD COLUMN status text DEFAULT 'installed'::text NOT NULL,
    ADD COLUMN message text DEFAULT ''::text NOT NULL;

CREATE INDEX idx_app_template_installations_status ON public.app_template_installations USING btree (status);
