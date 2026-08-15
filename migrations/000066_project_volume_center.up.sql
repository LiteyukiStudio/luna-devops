CREATE TABLE project_volumes (
    id text PRIMARY KEY,
    project_id text NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
    display_name text NOT NULL,
    cluster_id text NOT NULL REFERENCES runtime_clusters(id) ON DELETE RESTRICT,
    namespace text NOT NULL,
    claim_name text NOT NULL,
    ownership_mode text NOT NULL,
    source_kind text NOT NULL,
    source_snapshot_name text NOT NULL DEFAULT '',
    lifecycle_state text NOT NULL DEFAULT 'provisioning',
    pending_operation text NOT NULL DEFAULT 'provision',
    capacity_request text NOT NULL,
    capacity_bytes bigint NOT NULL,
    storage_class_name text NOT NULL DEFAULT '',
    access_mode text NOT NULL,
    volume_mode text NOT NULL,
    source_application_id text REFERENCES applications(id) ON DELETE SET NULL,
    source_application_name text NOT NULL DEFAULT '',
    source_deployment_target_id text REFERENCES deployment_targets(id) ON DELETE SET NULL,
    created_by text NOT NULL,
    revision bigint NOT NULL DEFAULT 1,
    idempotency_key_hash text NOT NULL DEFAULT '',
    idempotency_request_hash text NOT NULL DEFAULT '',
    last_error_code text NOT NULL DEFAULT '',
    last_error_message text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    CONSTRAINT chk_project_volumes_ownership_mode
        CHECK (ownership_mode IN ('managed', 'referenced')),
    CONSTRAINT chk_project_volumes_source_kind
        CHECK (source_kind IN ('blank', 'managed', 'retained', 'archive_import', 'snapshot_restore', 'existing_claim')),
    CONSTRAINT chk_project_volumes_lifecycle_state
        CHECK (lifecycle_state IN ('provisioning', 'ready', 'deleting', 'error')),
    CONSTRAINT chk_project_volumes_pending_operation
        CHECK (pending_operation IN ('', 'provision', 'expand', 'delete', 'import')),
    CONSTRAINT chk_project_volumes_access_mode
        CHECK (access_mode IN ('ReadWriteOnce', 'ReadWriteOncePod', 'ReadOnlyMany', 'ReadWriteMany')),
    CONSTRAINT chk_project_volumes_volume_mode
        CHECK (volume_mode IN ('Filesystem', 'Block')),
    CONSTRAINT chk_project_volumes_capacity CHECK (capacity_bytes > 0),
    CONSTRAINT chk_project_volumes_revision CHECK (revision > 0),
    CONSTRAINT chk_project_volumes_referenced_source
        CHECK (ownership_mode <> 'referenced' OR source_kind = 'existing_claim'),
    CONSTRAINT chk_project_volumes_snapshot_source CHECK (
        (source_kind = 'snapshot_restore' AND source_snapshot_name <> '')
        OR (source_kind <> 'snapshot_restore' AND source_snapshot_name = '')
    ),
    CONSTRAINT chk_project_volumes_idempotency_pair
        CHECK ((idempotency_key_hash = '') = (idempotency_request_hash = ''))
);

CREATE UNIQUE INDEX idx_project_volumes_claim_active
    ON project_volumes(cluster_id, namespace, claim_name)
    WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX idx_project_volumes_display_name_active
    ON project_volumes(project_id, lower(display_name))
    WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX idx_project_volumes_idempotency_active
    ON project_volumes(project_id, idempotency_key_hash)
    WHERE deleted_at IS NULL AND idempotency_key_hash <> '';
CREATE INDEX idx_project_volumes_project_lifecycle_created
    ON project_volumes(project_id, lifecycle_state, created_at DESC)
    WHERE deleted_at IS NULL;
CREATE INDEX idx_project_volumes_cluster
    ON project_volumes(cluster_id)
    WHERE deleted_at IS NULL;
CREATE INDEX idx_project_volumes_deleted_at ON project_volumes(deleted_at);

CREATE TABLE deployment_volume_mounts (
    id text PRIMARY KEY,
    project_id text NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
    application_id text NOT NULL REFERENCES applications(id) ON DELETE RESTRICT,
    deployment_target_id text NOT NULL REFERENCES deployment_targets(id) ON DELETE RESTRICT,
    source_type text NOT NULL,
    project_volume_id text REFERENCES project_volumes(id) ON DELETE RESTRICT,
    logical_name text NOT NULL,
    mount_path text,
    device_path text,
    read_only boolean NOT NULL DEFAULT false,
    exclusive boolean NOT NULL DEFAULT false,
    activation_state text NOT NULL DEFAULT 'reserved',
    empty_dir_medium text NOT NULL DEFAULT '',
    empty_dir_size_limit text NOT NULL DEFAULT '',
    last_error_code text NOT NULL DEFAULT '',
    last_error_message text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    CONSTRAINT chk_deployment_volume_mounts_source_type
        CHECK (source_type IN ('project_volume', 'empty_dir')),
    CONSTRAINT chk_deployment_volume_mounts_activation_state
        CHECK (activation_state IN ('reserved', 'active', 'release_pending', 'error')),
    CONSTRAINT chk_deployment_volume_mounts_source_fields CHECK (
        (
            source_type = 'project_volume'
            AND project_volume_id IS NOT NULL
            AND ((mount_path IS NOT NULL AND device_path IS NULL) OR (mount_path IS NULL AND device_path IS NOT NULL))
            AND empty_dir_medium = ''
            AND empty_dir_size_limit = ''
        )
        OR
        (
            source_type = 'empty_dir'
            AND project_volume_id IS NULL
            AND mount_path IS NOT NULL
            AND device_path IS NULL
        )
    )
);

CREATE UNIQUE INDEX idx_deployment_volume_mounts_logical_name_active
    ON deployment_volume_mounts(deployment_target_id, logical_name)
    WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX idx_deployment_volume_mounts_mount_path_active
    ON deployment_volume_mounts(deployment_target_id, mount_path)
    WHERE deleted_at IS NULL AND mount_path IS NOT NULL;
CREATE UNIQUE INDEX idx_deployment_volume_mounts_device_path_active
    ON deployment_volume_mounts(deployment_target_id, device_path)
    WHERE deleted_at IS NULL AND device_path IS NOT NULL;
CREATE UNIQUE INDEX idx_deployment_volume_mounts_exclusive_active
    ON deployment_volume_mounts(project_volume_id)
    WHERE deleted_at IS NULL
      AND exclusive = true
      AND activation_state IN ('reserved', 'active', 'release_pending', 'error');
CREATE INDEX idx_deployment_volume_mounts_project_volume
    ON deployment_volume_mounts(project_volume_id, activation_state)
    WHERE deleted_at IS NULL;
CREATE INDEX idx_deployment_volume_mounts_project_target
    ON deployment_volume_mounts(project_id, deployment_target_id)
    WHERE deleted_at IS NULL;
CREATE INDEX idx_deployment_volume_mounts_deleted_at ON deployment_volume_mounts(deleted_at);

CREATE TABLE volume_transfers (
    id text PRIMARY KEY,
    project_id text NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
    project_volume_id text NOT NULL REFERENCES project_volumes(id) ON DELETE RESTRICT,
    direction text NOT NULL,
    format text NOT NULL,
    consistency_mode text NOT NULL,
    state text NOT NULL DEFAULT 'created',
    object_key text NOT NULL,
    multipart_upload_id text NOT NULL DEFAULT '',
    source_filename text NOT NULL DEFAULT '',
    expected_bytes bigint NOT NULL DEFAULT 0,
    transferred_bytes bigint NOT NULL DEFAULT 0,
    processed_files bigint NOT NULL DEFAULT 0,
    phase text NOT NULL DEFAULT '',
    sha256 text NOT NULL DEFAULT '',
    actor_id text NOT NULL,
    callback_token_hash text NOT NULL DEFAULT '',
    callback_token_expires_at timestamptz,
    expires_at timestamptz NOT NULL,
    started_at timestamptz,
    finished_at timestamptz,
    object_deleted_at timestamptz,
    last_error_code text NOT NULL DEFAULT '',
    last_error_message text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chk_volume_transfers_direction CHECK (direction IN ('import', 'export')),
    CONSTRAINT chk_volume_transfers_format CHECK (format IN ('tar_gz', 'raw_zst')),
    CONSTRAINT chk_volume_transfers_consistency_mode CHECK (consistency_mode IN ('snapshot', 'live', 'unmounted')),
    CONSTRAINT chk_volume_transfers_state CHECK (state IN ('created', 'uploading', 'queued', 'running', 'succeeded', 'failed', 'cancelled', 'expired')),
    CONSTRAINT chk_volume_transfers_expected_bytes CHECK (expected_bytes >= 0),
    CONSTRAINT chk_volume_transfers_transferred_bytes CHECK (transferred_bytes >= 0),
    CONSTRAINT chk_volume_transfers_processed_files CHECK (processed_files >= 0),
    CONSTRAINT chk_volume_transfers_sha256 CHECK (sha256 = '' OR sha256 ~ '^[0-9a-f]{64}$')
);

CREATE INDEX idx_volume_transfers_project_state_created
    ON volume_transfers(project_id, state, created_at DESC);
CREATE INDEX idx_volume_transfers_volume_created
    ON volume_transfers(project_volume_id, created_at DESC);
CREATE INDEX idx_volume_transfers_expired_objects
    ON volume_transfers(expires_at, id)
    WHERE object_deleted_at IS NULL
      AND state IN ('created', 'uploading', 'succeeded', 'failed', 'cancelled', 'expired');
CREATE UNIQUE INDEX idx_volume_transfers_active_import
    ON volume_transfers(project_volume_id)
    WHERE direction = 'import' AND state IN ('created', 'uploading', 'queued', 'running');
CREATE UNIQUE INDEX idx_volume_transfers_active_export
    ON volume_transfers(project_volume_id)
    WHERE direction = 'export' AND state IN ('created', 'uploading', 'queued', 'running');

CREATE TABLE volume_transfer_parts (
    transfer_id text NOT NULL REFERENCES volume_transfers(id) ON DELETE CASCADE,
    part_number integer NOT NULL,
    byte_offset bigint NOT NULL,
    size bigint NOT NULL,
    etag text NOT NULL,
    sha256 text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (transfer_id, part_number),
    CONSTRAINT chk_volume_transfer_parts_number CHECK (part_number > 0),
    CONSTRAINT chk_volume_transfer_parts_offset CHECK (byte_offset >= 0),
    CONSTRAINT chk_volume_transfer_parts_size CHECK (size > 0),
    CONSTRAINT chk_volume_transfer_parts_sha256 CHECK (sha256 ~ '^[0-9a-f]{64}$')
);

CREATE INDEX idx_volume_transfer_parts_transfer_offset
    ON volume_transfer_parts(transfer_id, byte_offset);
