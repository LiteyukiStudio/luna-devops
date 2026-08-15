DROP INDEX IF EXISTS idx_volume_transfer_parts_stale_leases;
DROP INDEX IF EXISTS idx_volume_transfer_parts_transfer_offset;

ALTER TABLE volume_transfer_parts
    DROP CONSTRAINT IF EXISTS chk_volume_transfer_parts_completion,
    DROP CONSTRAINT IF EXISTS chk_volume_transfer_parts_state,
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS lease_expires_at,
    DROP COLUMN IF EXISTS lease_token,
    DROP COLUMN IF EXISTS state;

CREATE INDEX idx_volume_transfer_parts_transfer_offset
    ON volume_transfer_parts(transfer_id, byte_offset);
