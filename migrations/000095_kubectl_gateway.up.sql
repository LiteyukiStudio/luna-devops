CREATE TABLE kube_access_bindings (
    id text PRIMARY KEY,
    access_token_id text NOT NULL REFERENCES access_tokens(id) ON DELETE CASCADE,
    project_id text NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    runtime_cluster_id text NOT NULL REFERENCES runtime_clusters(id) ON DELETE CASCADE,
    application_id text REFERENCES applications(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_kube_access_bindings_access_token_id ON kube_access_bindings(access_token_id);
CREATE INDEX idx_kube_access_bindings_project_id ON kube_access_bindings(project_id);
CREATE INDEX idx_kube_access_bindings_runtime_cluster_id ON kube_access_bindings(runtime_cluster_id);
CREATE INDEX idx_kube_access_bindings_application_id ON kube_access_bindings(application_id);
CREATE UNIQUE INDEX idx_kube_access_bindings_context
    ON kube_access_bindings(access_token_id, project_id, runtime_cluster_id, COALESCE(application_id, ''));

ALTER TABLE runtime_clusters
    ADD COLUMN kube_gateway_enabled boolean NOT NULL DEFAULT false,
    ADD COLUMN kube_gateway_extra_resource_rules jsonb NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN delete_status text NOT NULL DEFAULT 'active',
    ADD COLUMN delete_message text NOT NULL DEFAULT '',
    ADD COLUMN delete_started_at timestamptz,
    ADD COLUMN delete_finished_at timestamptz,
    ADD COLUMN kube_gateway_drain_until timestamptz,
    ADD COLUMN kube_gateway_cleanup_completed_at timestamptz,
    ADD CONSTRAINT runtime_clusters_delete_status_check
        CHECK (delete_status IN ('active', 'deleting', 'delete_failed', 'deleted'));

CREATE INDEX idx_runtime_clusters_delete_status ON runtime_clusters(delete_status);

ALTER TABLE audit_logs
    ADD COLUMN metadata jsonb;

ALTER TABLE runtime_observations
    DROP CONSTRAINT runtime_observations_target_period_unique,
    ALTER COLUMN deployment_target_id DROP NOT NULL,
    ADD COLUMN management_source text NOT NULL DEFAULT 'platform',
    ADD COLUMN resource_kind text,
    ADD COLUMN resource_uid text,
    ADD COLUMN application_id text REFERENCES applications(id) ON DELETE SET NULL,
    ADD CONSTRAINT runtime_observations_management_source_check
        CHECK (management_source IN ('platform', 'kubectl'));

UPDATE runtime_observations AS observation
SET resource_kind = COALESCE(NULLIF(target.workload_type, ''), 'Deployment'),
    resource_uid = observation.deployment_target_id,
    application_id = target.application_id
FROM deployment_targets AS target
WHERE target.id = observation.deployment_target_id;

UPDATE runtime_observations
SET resource_kind = COALESCE(NULLIF(resource_kind, ''), 'Deployment'),
    resource_uid = COALESCE(NULLIF(resource_uid, ''), deployment_target_id, id);

ALTER TABLE runtime_observations
    ALTER COLUMN resource_kind SET NOT NULL,
    ALTER COLUMN resource_uid SET NOT NULL;

CREATE UNIQUE INDEX idx_runtime_observations_resource_period
    ON runtime_observations(runtime_cluster_id, project_id, resource_uid, period_start);
CREATE INDEX idx_runtime_observations_management_source ON runtime_observations(management_source);
CREATE INDEX idx_runtime_observations_resource_kind ON runtime_observations(resource_kind);
CREATE INDEX idx_runtime_observations_resource_uid ON runtime_observations(resource_uid);
CREATE INDEX idx_runtime_observations_application_id ON runtime_observations(application_id);

ALTER TABLE deployment_targets
    DROP COLUMN namespace;
