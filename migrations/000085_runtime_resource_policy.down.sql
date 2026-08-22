DROP INDEX IF EXISTS idx_runtime_observations_project_period;
DROP INDEX IF EXISTS idx_runtime_observations_cluster_period;

ALTER TABLE runtime_observations
  ADD COLUMN cpu_request text NOT NULL DEFAULT '',
  ADD COLUMN memory_request text NOT NULL DEFAULT '';

UPDATE runtime_observations
SET cpu_request = effective_cpu_request,
    memory_request = effective_memory_request;

ALTER TABLE runtime_observations
  DROP CONSTRAINT runtime_observations_policy_range,
  DROP CONSTRAINT runtime_observations_usage_nonnegative,
  DROP COLUMN runtime_cluster_id,
  DROP COLUMN project_id,
  DROP COLUMN effective_cpu_request,
  DROP COLUMN effective_memory_request,
  DROP COLUMN cpu_usage_milli,
  DROP COLUMN memory_usage_bytes,
  DROP COLUMN metrics_available,
  DROP COLUMN pod_count,
  DROP COLUMN container_count,
  DROP COLUMN cpu_request_percent,
  DROP COLUMN memory_request_percent,
  DROP COLUMN cpu_limit_percent,
  DROP COLUMN memory_limit_percent;

ALTER TABLE runtime_clusters
  DROP CONSTRAINT runtime_clusters_memory_policy_order,
  DROP CONSTRAINT runtime_clusters_cpu_policy_order,
  DROP CONSTRAINT runtime_clusters_memory_limit_percent_range,
  DROP CONSTRAINT runtime_clusters_cpu_limit_percent_range,
  DROP CONSTRAINT runtime_clusters_memory_request_percent_range,
  DROP CONSTRAINT runtime_clusters_cpu_request_percent_range,
  DROP COLUMN memory_limit_percent,
  DROP COLUMN cpu_limit_percent,
  DROP COLUMN memory_request_percent,
  DROP COLUMN cpu_request_percent;
