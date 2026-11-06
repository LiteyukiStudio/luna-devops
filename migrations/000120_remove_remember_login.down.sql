-- 恢复结构；已删除的记住我令牌无法恢复，用户需要重新登录。
ALTER TABLE user_sessions ADD COLUMN IF NOT EXISTS remember_family_id text DEFAULT ''::text NOT NULL;
CREATE INDEX IF NOT EXISTS idx_user_sessions_remember_family_id ON user_sessions USING btree (remember_family_id);
CREATE TABLE IF NOT EXISTS user_remember_tokens (
    id text NOT NULL PRIMARY KEY,
    user_id text NOT NULL,
    token_hash text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    family_id text NOT NULL,
    consumed_at timestamp with time zone,
    revoked_at timestamp with time zone
);
CREATE INDEX IF NOT EXISTS idx_user_remember_tokens_user_id ON user_remember_tokens USING btree (user_id);
CREATE INDEX IF NOT EXISTS idx_user_remember_tokens_family_id ON user_remember_tokens USING btree (family_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_user_remember_tokens_token_hash ON user_remember_tokens USING btree (token_hash);
CREATE INDEX IF NOT EXISTS idx_user_remember_tokens_expires_at ON user_remember_tokens USING btree (expires_at);
CREATE INDEX IF NOT EXISTS idx_user_remember_tokens_consumed_at ON user_remember_tokens USING btree (consumed_at);
CREATE INDEX IF NOT EXISTS idx_user_remember_tokens_revoked_at ON user_remember_tokens USING btree (revoked_at);
CREATE INDEX IF NOT EXISTS idx_user_remember_tokens_retention_expiry ON user_remember_tokens USING btree (expires_at, id);
