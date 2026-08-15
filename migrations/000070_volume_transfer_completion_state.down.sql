DROP INDEX IF EXISTS idx_volume_transfers_execution_cleanup_pending;

ALTER TABLE volume_transfers
    DROP CONSTRAINT IF EXISTS chk_volume_transfers_succeeded_evidence,
    DROP COLUMN IF EXISTS execution_cleanup_completed_at,
    DROP COLUMN IF EXISTS job_succeeded_at,
    DROP COLUMN IF EXISTS completion_reported_at;
