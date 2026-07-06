ALTER TABLE projects
  ADD COLUMN IF NOT EXISTS system_key text NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_projects_system_key
  ON projects(system_key)
  WHERE system_key <> '';

ALTER TABLE deployment_targets
  ADD COLUMN IF NOT EXISTS service_account_name text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS automount_service_account_token text NOT NULL DEFAULT '';

ALTER TABLE system_component_installations
  ADD COLUMN IF NOT EXISTS project_id text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS application_id text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS deployment_target_id text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS release_id text NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_system_component_installations_project_id
  ON system_component_installations(project_id);
CREATE INDEX IF NOT EXISTS idx_system_component_installations_application_id
  ON system_component_installations(application_id);
CREATE INDEX IF NOT EXISTS idx_system_component_installations_deployment_target_id
  ON system_component_installations(deployment_target_id);
CREATE INDEX IF NOT EXISTS idx_system_component_installations_release_id
  ON system_component_installations(release_id);
