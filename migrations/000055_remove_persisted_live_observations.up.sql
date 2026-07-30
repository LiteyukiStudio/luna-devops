ALTER TABLE runtime_clusters
    DROP COLUMN IF EXISTS status,
    DROP COLUMN IF EXISTS last_checked_at;

ALTER TABLE service_bindings
    DROP COLUMN IF EXISTS last_check_status,
    DROP COLUMN IF EXISTS last_checked_at;

ALTER TABLE gateway_routes
    DROP COLUMN IF EXISTS certificate_status,
    DROP COLUMN IF EXISTS certificate_message,
    DROP COLUMN IF EXISTS certificate_not_after,
    DROP COLUMN IF EXISTS certificate_issuer_kind,
    DROP COLUMN IF EXISTS certificate_issuer_name,
    DROP COLUMN IF EXISTS dns_status,
    DROP COLUMN IF EXISTS status;

ALTER TABLE git_accounts
    DROP COLUMN IF EXISTS status;

ALTER TABLE repository_bindings
    DROP COLUMN IF EXISTS webhook_status,
    ADD COLUMN IF NOT EXISTS webhook_enabled boolean NOT NULL DEFAULT true;

ALTER TABLE notification_channels
    DROP COLUMN IF EXISTS last_delivery_status,
    DROP COLUMN IF EXISTS last_delivery_error,
    DROP COLUMN IF EXISTS last_delivered_at;

ALTER TABLE notification_rules
    DROP COLUMN IF EXISTS last_matched_event_id;
