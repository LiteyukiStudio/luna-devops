DROP INDEX IF EXISTS idx_notification_deliveries_recipient_user_id;
DROP INDEX IF EXISTS idx_notification_deliveries_event_channel_recipient;

DELETE FROM notification_deliveries
WHERE recipient_user_id <> '';

DELETE FROM notification_channels
WHERE owner_user_id <> '';

CREATE UNIQUE INDEX idx_notification_deliveries_event_channel
    ON notification_deliveries USING btree (event_id, channel_id);

ALTER TABLE notification_deliveries
    DROP COLUMN recipient_user_id;

DROP INDEX IF EXISTS idx_notification_channels_owner_user_id;

ALTER TABLE notification_channels
    DROP COLUMN owner_user_id;

DROP TABLE user_notification_preferences;

ALTER TABLE auth_registration_settings
    ADD COLUMN smtp_host text DEFAULT ''::text NOT NULL,
    ADD COLUMN smtp_port integer DEFAULT 587 NOT NULL,
    ADD COLUMN smtp_security text DEFAULT 'starttls'::text NOT NULL,
    ADD COLUMN smtp_username text DEFAULT ''::text NOT NULL,
    ADD COLUMN smtp_password_ref text DEFAULT ''::text NOT NULL,
    ADD COLUMN smtp_from_address text DEFAULT ''::text NOT NULL,
    ADD COLUMN smtp_from_name text DEFAULT 'Luna DevOps'::text NOT NULL;

UPDATE auth_registration_settings AS registration
SET smtp_host = mail.host,
    smtp_port = mail.port,
    smtp_security = mail.security,
    smtp_username = mail.username,
    smtp_password_ref = mail.password_ref,
    smtp_from_address = mail.from_address,
    smtp_from_name = mail.from_name
FROM platform_mail_settings AS mail
WHERE registration.id = mail.id;

DROP TABLE platform_mail_settings;
