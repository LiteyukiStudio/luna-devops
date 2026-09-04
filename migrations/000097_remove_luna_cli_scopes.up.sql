BEGIN;

-- Do not lock the OAuth application in this transaction. Runtime approval and
-- exchange paths reach the application and device tables in different orders;
-- keeping this migration device-only avoids a rolling-upgrade deadlock.
LOCK TABLE oauth_device_authorizations IN ACCESS EXCLUSIVE MODE;

-- A pending or approved legacy Device Code still carries the old consent.
-- Consume it before dropping the Scope column so it cannot mint a full-access
-- credential after this migration.
UPDATE oauth_device_authorizations AS device_authorization
SET status = 'denied',
    denied_at = COALESCE(device_authorization.denied_at, now()),
    consumed_at = COALESCE(device_authorization.consumed_at, now()),
    updated_at = now()
WHERE device_authorization.application_id = 'oapp_luna_cli'
  AND device_authorization.consumed_at IS NULL;

ALTER TABLE oauth_device_authorizations
    DROP COLUMN IF EXISTS scope;

COMMIT;
