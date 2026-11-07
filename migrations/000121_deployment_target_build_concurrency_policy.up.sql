-- 同一部署配置的构建并发策略；现有部署配置默认按顺序排队执行。
ALTER TABLE deployment_targets
    ADD COLUMN IF NOT EXISTS build_concurrency_policy text DEFAULT 'queue'::text NOT NULL,
    ADD CONSTRAINT chk_deployment_targets_build_concurrency_policy
        CHECK (build_concurrency_policy IN ('queue', 'cancel_in_progress', 'parallel'));
