ALTER TABLE volume_transfers
    ADD COLUMN execution_generation bigint NOT NULL DEFAULT 0,
    ADD COLUMN creation_lease_owner text NOT NULL DEFAULT '',
    ADD COLUMN creation_lease_expires_at timestamptz,
    ADD COLUMN job_created_at timestamptz;

ALTER TABLE volume_transfers
    ADD CONSTRAINT chk_volume_transfers_execution_generation
        CHECK (execution_generation >= 0),
    ADD CONSTRAINT chk_volume_transfers_creation_lease_pair
        CHECK (
            (creation_lease_owner = '' AND creation_lease_expires_at IS NULL)
            OR (creation_lease_owner <> '' AND creation_lease_expires_at IS NOT NULL)
        );

CREATE INDEX idx_volume_transfers_creation_lease_expiry
    ON volume_transfers(creation_lease_expires_at, id)
    WHERE creation_lease_expires_at IS NOT NULL;

COMMENT ON COLUMN volume_transfers.execution_generation IS
    'Monotonic generation fencing concurrent Worker attempts that create the deterministic transfer Job.';

COMMENT ON COLUMN volume_transfers.creation_lease_owner IS
    'Opaque Worker-attempt owner for the short Job creation lease; never exposed through APIs.';

COMMENT ON COLUMN volume_transfers.job_created_at IS
    'Worker authoritatively observed the deterministic Kubernetes Job after creating or adopting it.';
