CREATE TABLE IF NOT EXISTS system_component_installations (
    id text PRIMARY KEY,
    component_id text NOT NULL,
    component_version text NOT NULL DEFAULT '',
    runtime_cluster_id text NOT NULL,
    namespace text NOT NULL DEFAULT 'liteyuki-system',
    status text NOT NULL DEFAULT 'pending',
    message text NOT NULL DEFAULT '',
    controller_type text NOT NULL DEFAULT '',
    mode text NOT NULL DEFAULT '',
    config text NOT NULL DEFAULT '{}',
    report_token_hash text NOT NULL DEFAULT '',
    last_error text NOT NULL DEFAULT '',
    installed_by text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_system_component_cluster
    ON system_component_installations(component_id, runtime_cluster_id);
CREATE INDEX IF NOT EXISTS idx_system_component_installations_component_id
    ON system_component_installations(component_id);
CREATE INDEX IF NOT EXISTS idx_system_component_installations_runtime_cluster_id
    ON system_component_installations(runtime_cluster_id);
CREATE INDEX IF NOT EXISTS idx_system_component_installations_status
    ON system_component_installations(status);
CREATE INDEX IF NOT EXISTS idx_system_component_installations_controller_type
    ON system_component_installations(controller_type);
CREATE INDEX IF NOT EXISTS idx_system_component_installations_installed_by
    ON system_component_installations(installed_by);
