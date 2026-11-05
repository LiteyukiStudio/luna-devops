CREATE TABLE public.app_template_installation_components (
    id text NOT NULL,
    installation_id text NOT NULL,
    component_key text NOT NULL,
    project_id text NOT NULL,
    application_id text NOT NULL,
    deployment_target_id text NOT NULL,
    release_id text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT app_template_installation_components_pkey PRIMARY KEY (id)
);

CREATE UNIQUE INDEX idx_app_template_installation_components_component
    ON public.app_template_installation_components USING btree (installation_id, component_key);

CREATE INDEX idx_app_template_installation_components_project_id
    ON public.app_template_installation_components USING btree (project_id);

CREATE INDEX idx_app_template_installation_components_application_id
    ON public.app_template_installation_components USING btree (application_id);

COMMENT ON TABLE public.app_template_installation_components IS 'Applications, deployment targets, and first releases created by one multi-component app template installation.';
COMMENT ON COLUMN public.app_template_installation_components.component_key IS 'Stable component key declared by the app template catalog.';
