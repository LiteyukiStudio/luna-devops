DROP INDEX IF EXISTS idx_deployment_targets_application_stage_active;

UPDATE deployment_targets AS target
SET stage = 'system'
FROM system_component_installations AS installation
WHERE installation.deployment_target_id = target.id;
