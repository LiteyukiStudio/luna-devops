-- 构建来源提交的消息，供构建列表展示提交详情。
ALTER TABLE build_runs ADD COLUMN IF NOT EXISTS source_commit_message text DEFAULT ''::text NOT NULL;
