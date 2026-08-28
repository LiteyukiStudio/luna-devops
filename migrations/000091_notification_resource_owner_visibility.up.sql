ALTER TABLE platform_events
    ADD COLUMN resource_owner_user_id text DEFAULT ''::text NOT NULL;

CREATE INDEX idx_platform_events_resource_owner_user_id
    ON platform_events USING btree (resource_owner_user_id);
