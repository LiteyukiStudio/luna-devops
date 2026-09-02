ALTER TABLE deployment_targets
    ADD COLUMN namespace text NOT NULL DEFAULT '';

ALTER TABLE audit_logs
    DROP COLUMN metadata;

DROP INDEX idx_runtime_clusters_delete_status;

ALTER TABLE runtime_clusters
    DROP CONSTRAINT runtime_clusters_delete_status_check,
    DROP COLUMN delete_finished_at,
    DROP COLUMN delete_started_at,
    DROP COLUMN delete_message,
    DROP COLUMN delete_status;
