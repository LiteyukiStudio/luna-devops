-- 集群级出站例外规则与其项目空间授权：只有授权的项目空间（可收窄到部署配置）能访问规则中的网段。
CREATE TABLE IF NOT EXISTS network_egress_exceptions (
    id text PRIMARY KEY,
    runtime_cluster_id text NOT NULL,
    name text NOT NULL,
    cidr text NOT NULL,
    ports text DEFAULT ''::text NOT NULL,
    protocol text DEFAULT 'tcp'::text NOT NULL,
    reason text NOT NULL,
    created_by text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_network_egress_exceptions_cluster ON network_egress_exceptions USING btree (runtime_cluster_id);

CREATE TABLE IF NOT EXISTS network_egress_exception_grants (
    id text PRIMARY KEY,
    exception_id text NOT NULL REFERENCES network_egress_exceptions (id) ON DELETE CASCADE,
    project_id text NOT NULL,
    deployment_target_ids text DEFAULT ''::text NOT NULL,
    include_builds boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_network_egress_exception_grants_exception_project ON network_egress_exception_grants USING btree (exception_id, project_id);
CREATE INDEX IF NOT EXISTS idx_network_egress_exception_grants_project ON network_egress_exception_grants USING btree (project_id);
