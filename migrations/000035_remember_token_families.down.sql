DROP INDEX IF EXISTS idx_user_sessions_user_remember_family;
DROP INDEX IF EXISTS idx_user_sessions_remember_family_id;
ALTER TABLE user_sessions DROP COLUMN IF EXISTS remember_family_id;

DROP INDEX IF EXISTS idx_user_remember_tokens_user_family;
DROP INDEX IF EXISTS idx_user_remember_tokens_revoked_at;
DROP INDEX IF EXISTS idx_user_remember_tokens_consumed_at;
DROP INDEX IF EXISTS idx_user_remember_tokens_family_id;
ALTER TABLE user_remember_tokens DROP COLUMN IF EXISTS revoked_at;
ALTER TABLE user_remember_tokens DROP COLUMN IF EXISTS consumed_at;
ALTER TABLE user_remember_tokens DROP COLUMN IF EXISTS family_id;
