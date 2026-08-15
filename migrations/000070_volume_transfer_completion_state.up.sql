ALTER TABLE volume_transfers
    ADD COLUMN completion_reported_at timestamptz,
    ADD COLUMN job_succeeded_at timestamptz,
    ADD COLUMN execution_cleanup_completed_at timestamptz;

UPDATE volume_transfers
SET completion_reported_at = COALESCE(finished_at, updated_at),
    job_succeeded_at = COALESCE(finished_at, updated_at)
WHERE state = 'succeeded';

-- Transfers that reached a terminal state before durable execution-cleanup
-- tracking was introduced cannot be re-associated with their historical
-- Kubernetes resources. Treat those rows as the compatibility boundary; all
-- new terminal transitions leave this marker NULL until the Worker proves the
-- Job prerequisites and any snapshot resources have been removed.
UPDATE volume_transfers
SET execution_cleanup_completed_at = COALESCE(finished_at, updated_at)
WHERE state IN ('succeeded', 'failed', 'cancelled', 'expired');

ALTER TABLE volume_transfers
    ADD CONSTRAINT chk_volume_transfers_succeeded_evidence
        CHECK (
            state <> 'succeeded'
            OR (completion_reported_at IS NOT NULL AND job_succeeded_at IS NOT NULL)
        );

COMMENT ON COLUMN volume_transfers.completion_reported_at IS
    'Transfer Job completion metadata was authenticated and persisted while the workflow remained running.';

COMMENT ON COLUMN volume_transfers.job_succeeded_at IS
    'Worker observed the authoritative Kubernetes Job succeeded condition before finalizing the workflow.';

COMMENT ON COLUMN volume_transfers.execution_cleanup_completed_at IS
    'Worker removed transfer Job prerequisites and temporary snapshot resources after a terminal transition.';

CREATE INDEX idx_volume_transfers_execution_cleanup_pending
    ON volume_transfers(updated_at, id)
    WHERE state IN ('succeeded', 'failed', 'cancelled', 'expired')
      AND execution_cleanup_completed_at IS NULL;
