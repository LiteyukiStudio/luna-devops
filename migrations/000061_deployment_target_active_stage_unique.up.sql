UPDATE deployment_targets AS target
SET stage = 'sys-' || LEFT(REGEXP_REPLACE(installation.runtime_cluster_id, '^.*_', ''), 8)
FROM system_component_installations AS installation
WHERE installation.deployment_target_id = target.id;

CREATE UNIQUE INDEX IF NOT EXISTS idx_deployment_targets_application_stage_active
  ON deployment_targets(application_id, stage)
  WHERE deleted_at IS NULL;
