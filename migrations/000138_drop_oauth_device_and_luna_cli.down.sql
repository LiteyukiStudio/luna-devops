CREATE TABLE public.oauth_device_authorizations (
    id text NOT NULL,
    application_id text NOT NULL,
    grant_id text,
    user_id text,
    device_code_hash text NOT NULL,
    user_code_hash text NOT NULL,
    status text NOT NULL,
    interval_seconds integer NOT NULL,
    last_polled_at timestamp with time zone,
    expires_at timestamp with time zone NOT NULL,
    approved_at timestamp with time zone,
    denied_at timestamp with time zone,
    consumed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE ONLY public.oauth_device_authorizations
    ADD CONSTRAINT oauth_device_authorizations_pkey PRIMARY KEY (id);

CREATE INDEX idx_oauth_device_authorizations_application_id ON public.oauth_device_authorizations USING btree (application_id);
CREATE INDEX idx_oauth_device_authorizations_consumed_at ON public.oauth_device_authorizations USING btree (consumed_at);
CREATE UNIQUE INDEX idx_oauth_device_authorizations_device_code_hash ON public.oauth_device_authorizations USING btree (device_code_hash);
CREATE INDEX idx_oauth_device_authorizations_expires_at ON public.oauth_device_authorizations USING btree (expires_at);
CREATE INDEX idx_oauth_device_authorizations_grant_id ON public.oauth_device_authorizations USING btree (grant_id);
CREATE INDEX idx_oauth_device_authorizations_status ON public.oauth_device_authorizations USING btree (status);
CREATE UNIQUE INDEX idx_oauth_device_authorizations_user_code_hash ON public.oauth_device_authorizations USING btree (user_code_hash);
CREATE INDEX idx_oauth_device_authorizations_user_id ON public.oauth_device_authorizations USING btree (user_id);

ALTER TABLE ONLY public.oauth_device_authorizations
    ADD CONSTRAINT oauth_device_authorizations_application_id_fkey FOREIGN KEY (application_id) REFERENCES public.oauth_applications(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.oauth_device_authorizations
    ADD CONSTRAINT oauth_device_authorizations_grant_id_fkey FOREIGN KEY (grant_id) REFERENCES public.oauth_grants(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.oauth_device_authorizations
    ADD CONSTRAINT oauth_device_authorizations_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

INSERT INTO public.oauth_applications (
    id,
    owner_user_id,
    name,
    description,
    homepage_url,
    logo_url,
    client_id,
    client_secret_hash,
    redirect_uris,
    allowed_scopes,
    access_token_lifetime_days,
    created_at,
    updated_at
) VALUES (
    'oapp_luna_cli',
    NULL,
    'Luna CLI',
    'Built-in public OAuth client for Luna CLI device authorization.',
    '',
    '',
    'luna-cli',
    '',
    '[]',
    '*',
    1,
    now(),
    now()
)
ON CONFLICT (id) DO NOTHING;
