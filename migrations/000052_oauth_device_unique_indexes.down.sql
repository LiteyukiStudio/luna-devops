DROP INDEX IF EXISTS idx_oauth_device_authorizations_device_code_hash;
DROP INDEX IF EXISTS idx_oauth_device_authorizations_user_code_hash;

ALTER TABLE oauth_device_authorizations
    ADD CONSTRAINT oauth_device_authorizations_device_code_hash_key UNIQUE (device_code_hash),
    ADD CONSTRAINT oauth_device_authorizations_user_code_hash_key UNIQUE (user_code_hash);
