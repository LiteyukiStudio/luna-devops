ALTER TABLE runtime_clusters
  ADD COLUMN cpu_request_percent integer NOT NULL DEFAULT 10,
  ADD COLUMN memory_request_percent integer NOT NULL DEFAULT 25,
  ADD COLUMN cpu_limit_percent integer NOT NULL DEFAULT 100,
  ADD COLUMN memory_limit_percent integer NOT NULL DEFAULT 100,
  ADD CONSTRAINT runtime_clusters_cpu_request_percent_range CHECK (cpu_request_percent BETWEEN 0 AND 100),
  ADD CONSTRAINT runtime_clusters_memory_request_percent_range CHECK (memory_request_percent BETWEEN 0 AND 100),
  ADD CONSTRAINT runtime_clusters_cpu_limit_percent_range CHECK (cpu_limit_percent BETWEEN 0 AND 100),
  ADD CONSTRAINT runtime_clusters_memory_limit_percent_range CHECK (memory_limit_percent BETWEEN 0 AND 100),
  ADD CONSTRAINT runtime_clusters_cpu_policy_order CHECK (cpu_limit_percent = 0 OR cpu_request_percent <= cpu_limit_percent),
  ADD CONSTRAINT runtime_clusters_memory_policy_order CHECK (memory_limit_percent = 0 OR memory_request_percent <= memory_limit_percent);

ALTER TABLE runtime_observations
  ADD COLUMN runtime_cluster_id text NOT NULL DEFAULT '',
  ADD COLUMN project_id text NOT NULL DEFAULT '',
  ADD COLUMN effective_cpu_request text NOT NULL DEFAULT '',
  ADD COLUMN effective_memory_request text NOT NULL DEFAULT '',
  ADD COLUMN cpu_usage_milli bigint NOT NULL DEFAULT 0,
  ADD COLUMN memory_usage_bytes bigint NOT NULL DEFAULT 0,
  ADD COLUMN metrics_available boolean NOT NULL DEFAULT false,
  ADD COLUMN pod_count integer NOT NULL DEFAULT 0,
  ADD COLUMN container_count integer NOT NULL DEFAULT 0,
  ADD COLUMN cpu_request_percent integer NOT NULL DEFAULT 10,
  ADD COLUMN memory_request_percent integer NOT NULL DEFAULT 25,
  ADD COLUMN cpu_limit_percent integer NOT NULL DEFAULT 100,
  ADD COLUMN memory_limit_percent integer NOT NULL DEFAULT 100;

UPDATE runtime_observations
SET effective_cpu_request = cpu_request,
    effective_memory_request = memory_request;

ALTER TABLE runtime_observations
  DROP COLUMN cpu_request,
  DROP COLUMN memory_request,
  ADD CONSTRAINT runtime_observations_usage_nonnegative CHECK (
    cpu_usage_milli >= 0 AND memory_usage_bytes >= 0 AND pod_count >= 0 AND container_count >= 0
  ),
  ADD CONSTRAINT runtime_observations_policy_range CHECK (
    cpu_request_percent BETWEEN 0 AND 100 AND memory_request_percent BETWEEN 0 AND 100 AND
    cpu_limit_percent BETWEEN 0 AND 100 AND memory_limit_percent BETWEEN 0 AND 100
  );

CREATE INDEX idx_runtime_observations_cluster_period ON runtime_observations(runtime_cluster_id, period_start);
CREATE INDEX idx_runtime_observations_project_period ON runtime_observations(project_id, period_start);
