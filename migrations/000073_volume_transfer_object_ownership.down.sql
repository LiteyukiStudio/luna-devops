DROP INDEX IF EXISTS idx_volume_transfers_expired_owned_objects;
DROP INDEX IF EXISTS idx_volume_transfers_object_owner;

ALTER TABLE volume_transfers
    DROP CONSTRAINT IF EXISTS chk_volume_transfer_deleted_not_owned,
    DROP CONSTRAINT IF EXISTS chk_volume_transfer_object_cleanup_started_owner,
    DROP CONSTRAINT IF EXISTS chk_volume_transfer_object_cleanup_owner,
    DROP CONSTRAINT IF EXISTS chk_volume_transfer_object_cleanup_lease_pair,
    DROP COLUMN IF EXISTS object_cleanup_lease_expires_at,
    DROP COLUMN IF EXISTS object_cleanup_lease_token,
    DROP COLUMN IF EXISTS object_cleanup_started_at,
    DROP COLUMN IF EXISTS object_owned;
