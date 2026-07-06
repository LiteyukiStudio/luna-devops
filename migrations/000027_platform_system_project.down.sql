DROP INDEX IF EXISTS idx_system_component_installations_release_id;
DROP INDEX IF EXISTS idx_system_component_installations_deployment_target_id;
DROP INDEX IF EXISTS idx_system_component_installations_application_id;
DROP INDEX IF EXISTS idx_system_component_installations_project_id;

ALTER TABLE system_component_installations
  DROP COLUMN IF EXISTS release_id,
  DROP COLUMN IF EXISTS deployment_target_id,
  DROP COLUMN IF EXISTS application_id,
  DROP COLUMN IF EXISTS project_id;

ALTER TABLE deployment_targets
  DROP COLUMN IF EXISTS automount_service_account_token,
  DROP COLUMN IF EXISTS service_account_name;

DROP INDEX IF EXISTS idx_projects_system_key;

ALTER TABLE projects
  DROP COLUMN IF EXISTS system_key;
