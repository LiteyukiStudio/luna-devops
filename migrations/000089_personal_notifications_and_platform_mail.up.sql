CREATE TABLE platform_mail_settings (
    id text PRIMARY KEY,
    host text DEFAULT ''::text NOT NULL,
    port integer DEFAULT 587 NOT NULL,
    security text DEFAULT 'starttls'::text NOT NULL,
    username text DEFAULT ''::text NOT NULL,
    password_ref text DEFAULT ''::text NOT NULL,
    from_address text DEFAULT ''::text NOT NULL,
    from_name text DEFAULT 'Luna DevOps'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT platform_mail_settings_port_range CHECK (port BETWEEN 1 AND 65535),
    CONSTRAINT platform_mail_settings_security_check CHECK (security = ANY (ARRAY['none'::text, 'starttls'::text, 'tls'::text]))
);

INSERT INTO platform_mail_settings (
    id,
    host,
    port,
    security,
    username,
    password_ref,
    from_address,
    from_name,
    created_at,
    updated_at
)
SELECT
    id,
    smtp_host,
    smtp_port,
    smtp_security,
    smtp_username,
    smtp_password_ref,
    smtp_from_address,
    smtp_from_name,
    created_at,
    updated_at
FROM auth_registration_settings
ON CONFLICT (id) DO NOTHING;

ALTER TABLE auth_registration_settings
    DROP COLUMN smtp_host,
    DROP COLUMN smtp_port,
    DROP COLUMN smtp_security,
    DROP COLUMN smtp_username,
    DROP COLUMN smtp_password_ref,
    DROP COLUMN smtp_from_address,
    DROP COLUMN smtp_from_name;

CREATE TABLE user_notification_preferences (
    user_id text PRIMARY KEY,
    email_enabled boolean DEFAULT true NOT NULL,
    event_types_json jsonb DEFAULT '["build.failed", "release.failed", "hook.failed", "gateway.apply_failed", "certificate.failed", "certificate.expired"]'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE notification_channels
    ADD COLUMN owner_user_id text DEFAULT ''::text NOT NULL;

CREATE INDEX idx_notification_channels_owner_user_id
    ON notification_channels USING btree (owner_user_id);

ALTER TABLE notification_deliveries
    ADD COLUMN recipient_user_id text DEFAULT ''::text NOT NULL;

DROP INDEX idx_notification_deliveries_event_channel;

CREATE UNIQUE INDEX idx_notification_deliveries_event_channel_recipient
    ON notification_deliveries USING btree (event_id, channel_id, recipient_user_id);

CREATE INDEX idx_notification_deliveries_recipient_user_id
    ON notification_deliveries USING btree (recipient_user_id);
