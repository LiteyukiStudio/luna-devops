ALTER TABLE deployment_targets
    ADD COLUMN namespace text NOT NULL DEFAULT '';

DELETE FROM runtime_observations
WHERE deployment_target_id IS NULL OR deployment_target_id = '';

DROP INDEX IF EXISTS idx_runtime_observations_application_id;
DROP INDEX IF EXISTS idx_runtime_observations_resource_uid;
DROP INDEX IF EXISTS idx_runtime_observations_resource_kind;
DROP INDEX IF EXISTS idx_runtime_observations_management_source;
DROP INDEX IF EXISTS idx_runtime_observations_resource_period;

ALTER TABLE runtime_observations
    DROP CONSTRAINT IF EXISTS runtime_observations_management_source_check,
    DROP COLUMN application_id,
    DROP COLUMN resource_uid,
    DROP COLUMN resource_kind,
    DROP COLUMN management_source,
    ALTER COLUMN deployment_target_id SET NOT NULL,
    ADD CONSTRAINT runtime_observations_target_period_unique
        UNIQUE (deployment_target_id, period_start);

ALTER TABLE audit_logs
    DROP COLUMN metadata;

DROP INDEX IF EXISTS idx_runtime_clusters_delete_status;

ALTER TABLE runtime_clusters
    DROP CONSTRAINT IF EXISTS runtime_clusters_delete_status_check,
    DROP COLUMN kube_gateway_cleanup_completed_at,
    DROP COLUMN kube_gateway_drain_until,
    DROP COLUMN delete_finished_at,
    DROP COLUMN delete_started_at,
    DROP COLUMN delete_message,
    DROP COLUMN delete_status,
    DROP COLUMN kube_gateway_extra_resource_rules,
    DROP COLUMN kube_gateway_enabled;

DROP TABLE kube_access_bindings;
