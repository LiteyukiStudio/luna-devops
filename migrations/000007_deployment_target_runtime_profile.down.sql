DROP INDEX IF EXISTS idx_deployment_targets_cluster_id;

ALTER TABLE deployment_targets
    DROP COLUMN IF EXISTS cluster_id,
    DROP COLUMN IF EXISTS namespace,
    DROP COLUMN IF EXISTS replicas,
    DROP COLUMN IF EXISTS cpu_request,
    DROP COLUMN IF EXISTS memory_request;
