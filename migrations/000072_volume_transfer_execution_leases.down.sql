DROP INDEX IF EXISTS idx_volume_transfers_creation_lease_expiry;

ALTER TABLE volume_transfers
    DROP CONSTRAINT IF EXISTS chk_volume_transfers_creation_lease_pair,
    DROP CONSTRAINT IF EXISTS chk_volume_transfers_execution_generation,
    DROP COLUMN IF EXISTS job_created_at,
    DROP COLUMN IF EXISTS creation_lease_expires_at,
    DROP COLUMN IF EXISTS creation_lease_owner,
    DROP COLUMN IF EXISTS execution_generation;
