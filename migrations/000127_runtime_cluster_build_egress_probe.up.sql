-- 构建出站配置从 Worker 环境变量迁入集群；记录 NetworkPolicy 执行能力的最近一次主动探测结果。
ALTER TABLE runtime_clusters ADD COLUMN IF NOT EXISTS build_egress_mode text DEFAULT 'restricted'::text NOT NULL;
ALTER TABLE runtime_clusters ADD COLUMN IF NOT EXISTS build_private_egress_cidrs text DEFAULT ''::text NOT NULL;
ALTER TABLE runtime_clusters ADD COLUMN IF NOT EXISTS build_private_egress_ports text DEFAULT '443'::text NOT NULL;
ALTER TABLE runtime_clusters ADD COLUMN IF NOT EXISTS network_policy_enforcement text DEFAULT 'unknown'::text NOT NULL;
ALTER TABLE runtime_clusters ADD COLUMN IF NOT EXISTS network_policy_probe_code text DEFAULT ''::text NOT NULL;
ALTER TABLE runtime_clusters ADD COLUMN IF NOT EXISTS network_policy_probed_at timestamp with time zone;
