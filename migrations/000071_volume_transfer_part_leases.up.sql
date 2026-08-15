ALTER TABLE volume_transfer_parts
    ADD COLUMN state text NOT NULL DEFAULT 'completed',
    ADD COLUMN lease_token text NOT NULL DEFAULT '',
    ADD COLUMN lease_expires_at timestamptz,
    ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now();

ALTER TABLE volume_transfer_parts
    ADD CONSTRAINT chk_volume_transfer_parts_state
        CHECK (state IN ('reserved', 'completed')),
    ADD CONSTRAINT chk_volume_transfer_parts_completion
        CHECK (
            (state = 'completed' AND etag <> '' AND lease_token = '' AND lease_expires_at IS NULL)
            OR
            (state = 'reserved' AND etag = '' AND lease_token <> '' AND lease_expires_at IS NOT NULL)
        );

DROP INDEX idx_volume_transfer_parts_transfer_offset;
CREATE UNIQUE INDEX idx_volume_transfer_parts_transfer_offset
    ON volume_transfer_parts(transfer_id, byte_offset);

CREATE INDEX idx_volume_transfer_parts_stale_leases
    ON volume_transfer_parts(lease_expires_at, transfer_id, part_number)
    WHERE state = 'reserved';

COMMENT ON COLUMN volume_transfer_parts.state IS
    'A reserved row assigns a stable S3 part number before network I/O; completed rows advance the authoritative upload offset.';
COMMENT ON COLUMN volume_transfer_parts.lease_token IS
    'Opaque API-instance lease used to CAS the transaction-external object-store write.';
COMMENT ON COLUMN volume_transfer_parts.lease_expires_at IS
    'After this time an identical retry may take over and safely rewrite the same S3 part number.';
