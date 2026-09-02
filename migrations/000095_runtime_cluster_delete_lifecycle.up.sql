ALTER TABLE runtime_clusters
    ADD COLUMN delete_status text NOT NULL DEFAULT 'active',
    ADD COLUMN delete_message text NOT NULL DEFAULT '',
    ADD COLUMN delete_started_at timestamptz,
    ADD COLUMN delete_finished_at timestamptz,
    ADD CONSTRAINT runtime_clusters_delete_status_check
        CHECK (delete_status IN ('active', 'deleting', 'delete_failed', 'deleted'));

CREATE INDEX idx_runtime_clusters_delete_status ON runtime_clusters(delete_status);

ALTER TABLE audit_logs
    ADD COLUMN metadata jsonb;

ALTER TABLE deployment_targets
    DROP COLUMN namespace;
