ALTER TABLE volume_transfers
    ADD COLUMN object_owned boolean NOT NULL DEFAULT true,
    ADD COLUMN object_cleanup_started_at timestamptz,
    ADD COLUMN object_cleanup_lease_token text NOT NULL DEFAULT '',
    ADD COLUMN object_cleanup_lease_expires_at timestamptz;

-- Existing retry rows may reference the same object. The newest surviving
-- transfer becomes the sole owner before the uniqueness fence is installed.
WITH ranked_references AS (
    SELECT id,
           row_number() OVER (
               PARTITION BY object_key
               ORDER BY created_at DESC, id DESC
           ) AS reference_rank
    FROM volume_transfers
    WHERE object_deleted_at IS NULL
)
UPDATE volume_transfers AS transfer
SET object_owned = false
FROM ranked_references AS reference
WHERE transfer.id = reference.id
  AND reference.reference_rank > 1;

UPDATE volume_transfers
SET object_owned = false
WHERE object_deleted_at IS NOT NULL;

ALTER TABLE volume_transfers
    ADD CONSTRAINT chk_volume_transfer_object_cleanup_lease_pair CHECK (
        (object_cleanup_lease_token = '') = (object_cleanup_lease_expires_at IS NULL)
    ),
    ADD CONSTRAINT chk_volume_transfer_object_cleanup_owner CHECK (
        object_owned OR object_cleanup_lease_token = ''
    ),
    ADD CONSTRAINT chk_volume_transfer_object_cleanup_started_owner CHECK (
        object_cleanup_started_at IS NULL OR object_owned OR object_deleted_at IS NOT NULL
    ),
    ADD CONSTRAINT chk_volume_transfer_deleted_not_owned CHECK (
        object_deleted_at IS NULL OR NOT object_owned
    );

CREATE UNIQUE INDEX idx_volume_transfers_object_owner
    ON volume_transfers(object_key)
    WHERE object_owned = true AND object_deleted_at IS NULL;

CREATE INDEX idx_volume_transfers_expired_owned_objects
    ON volume_transfers(expires_at, id)
    WHERE object_owned = true
      AND object_deleted_at IS NULL
      AND state IN ('created', 'uploading', 'succeeded', 'failed', 'cancelled', 'expired');
