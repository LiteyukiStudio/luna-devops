ALTER TABLE deployment_targets
    ADD COLUMN IF NOT EXISTS cluster_id text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS namespace text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS replicas integer NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS cpu_request text NOT NULL DEFAULT '1',
    ADD COLUMN IF NOT EXISTS memory_request text NOT NULL DEFAULT '1Gi';

UPDATE deployment_targets
SET
    cluster_id = COALESCE(NULLIF(environments.cluster_id, ''), deployment_targets.cluster_id),
    namespace = COALESCE(NULLIF(environments.namespace, ''), deployment_targets.namespace),
    replicas = COALESCE(NULLIF(environments.replicas, 0), deployment_targets.replicas),
    cpu_request = COALESCE(NULLIF(environments.cpu_request, ''), deployment_targets.cpu_request),
    memory_request = COALESCE(NULLIF(environments.memory_request, ''), deployment_targets.memory_request)
FROM environments
WHERE deployment_targets.environment_id = environments.id;

UPDATE deployment_targets
SET
    replicas = CASE WHEN replicas <= 0 THEN 1 ELSE replicas END,
    cpu_request = COALESCE(NULLIF(cpu_request, ''), '1'),
    memory_request = COALESCE(NULLIF(memory_request, ''), '1Gi');

CREATE INDEX IF NOT EXISTS idx_deployment_targets_cluster_id ON deployment_targets(cluster_id);
