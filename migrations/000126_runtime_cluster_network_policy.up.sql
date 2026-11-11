-- 集群网络隔离：管理员追加的出站拒绝网段与网关控制器 Pod 所在 Namespace。
ALTER TABLE runtime_clusters ADD COLUMN IF NOT EXISTS network_blocked_cidrs text DEFAULT ''::text NOT NULL;
ALTER TABLE runtime_clusters ADD COLUMN IF NOT EXISTS network_gateway_namespace text DEFAULT ''::text NOT NULL;
