-- 构建执行中最近开始的 Dockerfile 指令，供构建列表悬停展示。
ALTER TABLE build_jobs ADD COLUMN IF NOT EXISTS current_step text DEFAULT ''::text NOT NULL;
