ALTER TABLE runtime_clusters
    ADD COLUMN IF NOT EXISTS status text NOT NULL DEFAULT 'unknown',
    ADD COLUMN IF NOT EXISTS last_checked_at timestamptz;

ALTER TABLE service_bindings
    ADD COLUMN IF NOT EXISTS last_check_status text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS last_checked_at timestamptz;

ALTER TABLE gateway_routes
    ADD COLUMN IF NOT EXISTS certificate_status text NOT NULL DEFAULT 'disabled',
    ADD COLUMN IF NOT EXISTS certificate_message text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS certificate_not_after timestamptz,
    ADD COLUMN IF NOT EXISTS certificate_issuer_kind text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS certificate_issuer_name text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS dns_status text NOT NULL DEFAULT 'pending',
    ADD COLUMN IF NOT EXISTS status text NOT NULL DEFAULT 'pending';

ALTER TABLE git_accounts
    ADD COLUMN IF NOT EXISTS status text NOT NULL DEFAULT 'connected';

ALTER TABLE repository_bindings
    DROP COLUMN IF EXISTS webhook_enabled,
    ADD COLUMN IF NOT EXISTS webhook_status text NOT NULL DEFAULT 'pending';

ALTER TABLE notification_channels
    ADD COLUMN IF NOT EXISTS last_delivery_status text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS last_delivery_error text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS last_delivered_at timestamptz;

ALTER TABLE notification_rules
    ADD COLUMN IF NOT EXISTS last_matched_event_id text NOT NULL DEFAULT '';
