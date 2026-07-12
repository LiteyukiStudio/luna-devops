ALTER TABLE user_remember_tokens ADD COLUMN IF NOT EXISTS family_id text;
ALTER TABLE user_remember_tokens ADD COLUMN IF NOT EXISTS consumed_at timestamptz;
ALTER TABLE user_remember_tokens ADD COLUMN IF NOT EXISTS revoked_at timestamptz;

UPDATE user_remember_tokens SET family_id = id WHERE family_id IS NULL OR family_id = '';

ALTER TABLE user_remember_tokens ALTER COLUMN family_id SET NOT NULL;

CREATE INDEX IF NOT EXISTS idx_user_remember_tokens_family_id ON user_remember_tokens(family_id);
CREATE INDEX IF NOT EXISTS idx_user_remember_tokens_consumed_at ON user_remember_tokens(consumed_at);
CREATE INDEX IF NOT EXISTS idx_user_remember_tokens_revoked_at ON user_remember_tokens(revoked_at);
CREATE INDEX IF NOT EXISTS idx_user_remember_tokens_user_family ON user_remember_tokens(user_id, family_id);

ALTER TABLE user_sessions ADD COLUMN IF NOT EXISTS remember_family_id text NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_user_sessions_remember_family_id ON user_sessions(remember_family_id);
CREATE INDEX IF NOT EXISTS idx_user_sessions_user_remember_family ON user_sessions(user_id, remember_family_id);
