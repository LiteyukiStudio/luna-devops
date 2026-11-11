-- 项目空间网络模式（只能比集群默认更严格）与部署配置的内部服务开关。
ALTER TABLE projects ADD COLUMN IF NOT EXISTS network_ingress_mode text DEFAULT 'isolated'::text NOT NULL;
ALTER TABLE projects ADD COLUMN IF NOT EXISTS network_egress_mode text DEFAULT 'internet'::text NOT NULL;
ALTER TABLE projects ADD COLUMN IF NOT EXISTS network_egress_allowlist text DEFAULT ''::text NOT NULL;
ALTER TABLE deployment_targets ADD COLUMN IF NOT EXISTS internal_only boolean DEFAULT false NOT NULL;
