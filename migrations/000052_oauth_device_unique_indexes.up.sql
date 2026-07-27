ALTER TABLE oauth_device_authorizations
    DROP CONSTRAINT IF EXISTS oauth_device_authorizations_device_code_hash_key,
    DROP CONSTRAINT IF EXISTS oauth_device_authorizations_user_code_hash_key;

CREATE UNIQUE INDEX IF NOT EXISTS idx_oauth_device_authorizations_device_code_hash
    ON oauth_device_authorizations(device_code_hash);

CREATE UNIQUE INDEX IF NOT EXISTS idx_oauth_device_authorizations_user_code_hash
    ON oauth_device_authorizations(user_code_hash);
