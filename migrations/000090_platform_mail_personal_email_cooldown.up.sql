ALTER TABLE platform_mail_settings
    ADD COLUMN personal_email_cooldown_seconds integer DEFAULT 60 NOT NULL,
    ADD CONSTRAINT platform_mail_settings_personal_email_cooldown_seconds_range
        CHECK (personal_email_cooldown_seconds BETWEEN 0 AND 3600);
