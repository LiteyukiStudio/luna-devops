DROP TRIGGER IF EXISTS trg_project_volumes_quota_update ON project_volumes;
DROP TRIGGER IF EXISTS trg_project_volumes_quota_insert ON project_volumes;
DROP FUNCTION IF EXISTS luna_sync_project_volume_quota();
DROP FUNCTION IF EXISTS luna_project_volume_quota_limit_bytes();
DROP TABLE IF EXISTS project_volume_quota_reservations;
DROP TABLE IF EXISTS project_volume_quota_usage;
DELETE FROM app_configs WHERE key = 'storage.projectManagedCapacityLimitGiB';
DELETE FROM billing_rate_rules
WHERE meter = 'storage.transfer_gib'
  AND id = 'brte_' || LEFT(md5('storage.transfer_gib'), 24);
