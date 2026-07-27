DELETE FROM oauth_applications
WHERE id = 'oapp_luna_cli' OR client_id = 'luna-cli';

DROP TABLE IF EXISTS oauth_device_authorizations;

ALTER TABLE oauth_applications
    ALTER COLUMN owner_user_id SET NOT NULL;
