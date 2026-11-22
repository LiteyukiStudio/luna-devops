DELETE FROM public.access_tokens
WHERE oauth_application_id IN (
    SELECT id FROM public.oauth_applications
    WHERE token_endpoint_auth_method = 'none' OR registration_source = 'dynamic'
);

DELETE FROM public.oauth_applications
WHERE token_endpoint_auth_method = 'none' OR registration_source = 'dynamic';

ALTER TABLE public.oauth_applications
    DROP CONSTRAINT IF EXISTS chk_oauth_applications_public_secret_consistency,
    DROP CONSTRAINT IF EXISTS chk_oauth_applications_registration_source,
    DROP CONSTRAINT IF EXISTS chk_oauth_applications_token_endpoint_auth_method;

ALTER TABLE public.oauth_applications
    DROP COLUMN IF EXISTS grant_types,
    DROP COLUMN IF EXISTS registration_source,
    DROP COLUMN IF EXISTS token_endpoint_auth_method;
