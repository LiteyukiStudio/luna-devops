BEGIN;

UPDATE oauth_applications
SET allowed_scopes = '*',
    updated_at = now()
WHERE id = 'oapp_luna_cli'
  AND client_id = 'luna-cli'
  AND revoked_at IS NULL;

-- Credentials issued under the retired user-selected Scope model must not be
-- widened in place. Revoke them and require a fresh Device Code login instead.
UPDATE oauth_grants AS oauth_grant
SET revoked_at = COALESCE(application.revoked_at, now()),
    updated_at = now()
FROM oauth_applications AS application
WHERE application.id = 'oapp_luna_cli' AND application.client_id = 'luna-cli'
  AND oauth_grant.application_id = application.id
  AND oauth_grant.revoked_at IS NULL;

UPDATE oauth_refresh_tokens AS refresh_token
SET revoked_at = COALESCE(application.revoked_at, now()),
    updated_at = now()
FROM oauth_applications AS application
WHERE application.id = 'oapp_luna_cli' AND application.client_id = 'luna-cli'
  AND refresh_token.application_id = application.id
  AND refresh_token.revoked_at IS NULL;

UPDATE access_tokens AS access_token
SET revoked_at = COALESCE(application.revoked_at, now()),
    updated_at = now()
FROM oauth_applications AS application
WHERE application.id = 'oapp_luna_cli' AND application.client_id = 'luna-cli'
  AND access_token.source = 'oauth' AND access_token.oauth_application_id = application.id
  AND access_token.revoked_at IS NULL;

COMMIT;
