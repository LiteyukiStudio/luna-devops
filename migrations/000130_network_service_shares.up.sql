-- 跨项目共享：提供方把某个部署配置开放给指定项目空间，使用方确认使用后才在两侧下发放行策略。
-- 提供方在部署配置所在集群持有出站例外时，共享还需该集群维护者批准（防止被当作访问内网的跳板）。
-- 是否需要批准在渲染时按当前集群与当前例外判断；批准只对 approved_runtime_cluster_id 指向的集群有效。
CREATE TABLE IF NOT EXISTS network_service_shares (
    id text PRIMARY KEY,
    provider_project_id text NOT NULL,
    deployment_target_id text NOT NULL,
    consumer_project_id text NOT NULL,
    approved_runtime_cluster_id text DEFAULT ''::text NOT NULL,
    approved_by text DEFAULT ''::text NOT NULL,
    approved_at timestamp with time zone,
    accepted_by text DEFAULT ''::text NOT NULL,
    accepted_at timestamp with time zone,
    created_by text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_network_service_shares_target_consumer ON network_service_shares USING btree (deployment_target_id, consumer_project_id);
CREATE INDEX IF NOT EXISTS idx_network_service_shares_provider ON network_service_shares USING btree (provider_project_id);
CREATE INDEX IF NOT EXISTS idx_network_service_shares_consumer ON network_service_shares USING btree (consumer_project_id);
