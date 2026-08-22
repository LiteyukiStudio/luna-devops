-- Direct volume transfer deliberately removes persisted archive ownership and
-- multipart upload state. Existing in-flight object-backed transfers cannot be
-- resumed under the new protocol and are closed with a stable terminal code.
ALTER TABLE volume_transfers
    DROP CONSTRAINT IF EXISTS chk_volume_transfers_state,
    DROP CONSTRAINT IF EXISTS chk_volume_transfers_succeeded_evidence,
    DROP CONSTRAINT IF EXISTS chk_volume_transfer_object_cleanup_lease_pair,
    DROP CONSTRAINT IF EXISTS chk_volume_transfer_object_cleanup_owner,
    DROP CONSTRAINT IF EXISTS chk_volume_transfer_object_cleanup_started_owner,
    DROP CONSTRAINT IF EXISTS chk_volume_transfer_deleted_not_owned;

UPDATE volume_transfers
SET state = 'failed',
    finished_at = COALESCE(finished_at, now()),
    last_error_code = 'volume_transfer.protocol_replaced',
    last_error_message = 'the persisted-archive transfer protocol was removed',
    updated_at = now()
WHERE state IN ('created', 'uploading', 'queued', 'running');

-- An archive-import volume was deliberately left provisioning until its
-- transfer succeeded. Close those legacy parents as well so an upgrade cannot
-- strand a volume in a permanently pending import state.
UPDATE project_volumes AS project_volume
SET lifecycle_state = 'error',
    pending_operation = '',
    last_error_code = 'volume_transfer.protocol_replaced',
    last_error_message = 'the persisted-archive transfer protocol was removed',
    revision = revision + 1,
    updated_at = now()
WHERE lifecycle_state = 'provisioning'
  AND pending_operation = 'import'
  AND EXISTS (
      SELECT 1
      FROM volume_transfers AS transfer
      WHERE transfer.project_volume_id = project_volume.id
        AND transfer.direction = 'import'
        AND transfer.state = 'failed'
        AND transfer.last_error_code = 'volume_transfer.protocol_replaced'
  );

DROP INDEX IF EXISTS idx_volume_transfers_active_export;
DROP INDEX IF EXISTS idx_volume_transfers_active_import;
DROP INDEX IF EXISTS idx_volume_transfers_expired_objects;
DROP INDEX IF EXISTS idx_volume_transfers_expired_owned_objects;
DROP INDEX IF EXISTS idx_volume_transfers_object_owner;

DROP TABLE IF EXISTS volume_transfer_parts;

ALTER TABLE volume_transfers
    DROP COLUMN IF EXISTS object_key,
    DROP COLUMN IF EXISTS object_owned,
    DROP COLUMN IF EXISTS object_cleanup_started_at,
    DROP COLUMN IF EXISTS object_cleanup_lease_token,
    DROP COLUMN IF EXISTS object_cleanup_lease_expires_at,
    DROP COLUMN IF EXISTS multipart_upload_id,
    DROP COLUMN IF EXISTS callback_token_hash,
    DROP COLUMN IF EXISTS callback_token_expires_at,
    DROP COLUMN IF EXISTS completion_reported_at,
    DROP COLUMN IF EXISTS job_succeeded_at,
    DROP COLUMN IF EXISTS object_deleted_at;

ALTER TABLE volume_transfers
    ADD CONSTRAINT chk_volume_transfers_state CHECK (
        state IN ('created', 'preparing', 'ready', 'streaming', 'succeeded', 'failed', 'cancelled', 'expired')
    );

CREATE UNIQUE INDEX idx_volume_transfers_active_export
    ON volume_transfers(project_volume_id)
    WHERE direction = 'export' AND state IN ('created', 'preparing', 'ready', 'streaming');

CREATE UNIQUE INDEX idx_volume_transfers_active_import
    ON volume_transfers(project_volume_id)
    WHERE direction = 'import' AND state IN ('created', 'preparing', 'ready', 'streaming');

CREATE INDEX idx_volume_transfers_ready_expiry
    ON volume_transfers(expires_at, id)
    WHERE state IN ('created', 'preparing', 'ready');

COMMENT ON TABLE volume_transfers IS
    'Direct client-to-runtime volume transfer workflow history; archive bytes are never persisted by the control plane.';
COMMENT ON COLUMN volume_transfers.expires_at IS
    'Deadline for an unclaimed prepared transfer session, not an archive object retention deadline.';
