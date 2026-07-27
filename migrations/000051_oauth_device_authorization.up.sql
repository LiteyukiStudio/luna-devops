ALTER TABLE oauth_applications
    ALTER COLUMN owner_user_id DROP NOT NULL;

CREATE TABLE oauth_device_authorizations (
    id text PRIMARY KEY,
    application_id text NOT NULL REFERENCES oauth_applications(id) ON DELETE CASCADE,
    grant_id text REFERENCES oauth_grants(id) ON DELETE CASCADE,
    user_id text REFERENCES users(id) ON DELETE CASCADE,
    device_code_hash text NOT NULL,
    user_code_hash text NOT NULL,
    scope text NOT NULL,
    status text NOT NULL,
    interval_seconds integer NOT NULL,
    last_polled_at timestamptz,
    expires_at timestamptz NOT NULL,
    approved_at timestamptz,
    denied_at timestamptz,
    consumed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_oauth_device_authorizations_application_id ON oauth_device_authorizations(application_id);
CREATE INDEX idx_oauth_device_authorizations_grant_id ON oauth_device_authorizations(grant_id);
CREATE INDEX idx_oauth_device_authorizations_user_id ON oauth_device_authorizations(user_id);
CREATE UNIQUE INDEX idx_oauth_device_authorizations_device_code_hash ON oauth_device_authorizations(device_code_hash);
CREATE UNIQUE INDEX idx_oauth_device_authorizations_user_code_hash ON oauth_device_authorizations(user_code_hash);
CREATE INDEX idx_oauth_device_authorizations_status ON oauth_device_authorizations(status);
CREATE INDEX idx_oauth_device_authorizations_expires_at ON oauth_device_authorizations(expires_at);
CREATE INDEX idx_oauth_device_authorizations_consumed_at ON oauth_device_authorizations(consumed_at);

INSERT INTO oauth_applications (
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
    '',
    1,
    now(),
    now()
) ON CONFLICT (client_id) DO NOTHING;
