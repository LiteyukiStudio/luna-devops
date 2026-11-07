-- 构建执行中解析的 Dockerfile 指令步骤进度。
ALTER TABLE build_jobs
    ADD COLUMN IF NOT EXISTS steps_completed integer DEFAULT 0 NOT NULL,
    ADD COLUMN IF NOT EXISTS steps_total integer DEFAULT 0 NOT NULL;
