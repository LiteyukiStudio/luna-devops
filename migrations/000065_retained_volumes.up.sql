CREATE TABLE IF NOT EXISTS retained_volumes (
    id text PRIMARY KEY,
    project_id text NOT NULL,
    source_application_id text NOT NULL,
    source_application_name text NOT NULL DEFAULT '',
    source_deployment_target_id text NOT NULL,
    cluster_id text NOT NULL,
    namespace text NOT NULL,
    claim_name text NOT NULL,
    volume_name text NOT NULL DEFAULT 'data',
    mount_path text NOT NULL DEFAULT '/data',
    capacity text NOT NULL DEFAULT '',
    storage_class_name text NOT NULL DEFAULT '',
    access_mode text NOT NULL DEFAULT '',
    volume_mode text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'retained',
    claimed_by_application_id text NOT NULL DEFAULT '',
    claimed_by_target_id text NOT NULL DEFAULT '',
    last_error text NOT NULL DEFAULT '',
    retained_at timestamptz NOT NULL,
    claimed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT retained_volumes_status_check CHECK (status IN ('retaining', 'retained', 'reserved', 'claimed', 'deleting', 'delete_failed')),
    CONSTRAINT retained_volumes_claim_unique UNIQUE (cluster_id, namespace, claim_name)
);

CREATE INDEX IF NOT EXISTS idx_retained_volumes_project_status ON retained_volumes(project_id, status);
CREATE INDEX IF NOT EXISTS idx_retained_volumes_source_application ON retained_volumes(source_application_id);
CREATE INDEX IF NOT EXISTS idx_retained_volumes_claimed_target ON retained_volumes(claimed_by_target_id);
