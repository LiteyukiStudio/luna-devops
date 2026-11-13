DROP TABLE IF EXISTS runtime_alert_incidents;

ALTER TABLE notification_deliveries
    DROP COLUMN IF EXISTS time_zone,
    DROP COLUMN IF EXISTS target_override_json;

ALTER TABLE notification_rules
    DROP COLUMN IF EXISTS time_zone,
    DROP COLUMN IF EXISTS notify_on_recovery,
    DROP COLUMN IF EXISTS max_reminders,
    DROP COLUMN IF EXISTS remind_interval_minutes,
    DROP COLUMN IF EXISTS channel_targets_json;

DELETE FROM scoped_resource_project_bindings
WHERE resource_type = 'notification_channel';

DROP INDEX IF EXISTS idx_notification_channels_scope_owner_ref;

ALTER TABLE notification_channels
    DROP COLUMN IF EXISTS preset_id,
    DROP COLUMN IF EXISTS owner_ref,
    DROP COLUMN IF EXISTS scope;
