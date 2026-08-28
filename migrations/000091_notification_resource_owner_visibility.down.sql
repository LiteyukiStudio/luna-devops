DROP INDEX IF EXISTS idx_platform_events_resource_owner_user_id;

ALTER TABLE platform_events
    DROP COLUMN resource_owner_user_id;
