ALTER TABLE volume_transfers
    DROP CONSTRAINT IF EXISTS chk_volume_transfers_data_digest_pair,
    DROP CONSTRAINT IF EXISTS chk_volume_transfers_data_sha256,
    DROP CONSTRAINT IF EXISTS chk_volume_transfers_logical_bytes,
    DROP COLUMN IF EXISTS data_sha256,
    DROP COLUMN IF EXISTS logical_bytes;
