ALTER TABLE platform_mail_settings
    DROP CONSTRAINT IF EXISTS platform_mail_settings_personal_email_cooldown_seconds_range,
    DROP COLUMN personal_email_cooldown_seconds;
