ALTER TABLE public.oauth_applications
    ADD COLUMN IF NOT EXISTS token_endpoint_auth_method text DEFAULT 'client_secret_basic'::text NOT NULL,
    ADD COLUMN IF NOT EXISTS registration_source text DEFAULT 'manual'::text NOT NULL,
    ADD COLUMN IF NOT EXISTS grant_types text DEFAULT '["authorization_code","refresh_token"]'::text NOT NULL;

UPDATE public.oauth_applications
SET client_secret_hash = 'invalid_legacy_unusable_secret_hash'
WHERE token_endpoint_auth_method <> 'none' AND client_secret_hash = '';

ALTER TABLE public.oauth_applications
    DROP CONSTRAINT IF EXISTS chk_oauth_applications_token_endpoint_auth_method,
    ADD CONSTRAINT chk_oauth_applications_token_endpoint_auth_method
        CHECK (token_endpoint_auth_method IN ('none', 'client_secret_basic', 'client_secret_post')),
    DROP CONSTRAINT IF EXISTS chk_oauth_applications_registration_source,
    ADD CONSTRAINT chk_oauth_applications_registration_source
        CHECK (registration_source IN ('manual', 'dynamic')),
    DROP CONSTRAINT IF EXISTS chk_oauth_applications_public_secret_consistency,
    ADD CONSTRAINT chk_oauth_applications_public_secret_consistency
        CHECK ((token_endpoint_auth_method = 'none') = (client_secret_hash = ''));
