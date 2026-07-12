CREATE TABLE IF NOT EXISTS user_mfa_configs (
  id text PRIMARY KEY,
  user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  totp_secret_ref text NOT NULL,
  enabled boolean NOT NULL DEFAULT false,
  confirmed_at timestamptz,
  recovery_codes_generated_at timestamptz,
  last_totp_counter bigint,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_user_mfa_configs_user_id ON user_mfa_configs(user_id);
CREATE INDEX IF NOT EXISTS idx_user_mfa_configs_enabled ON user_mfa_configs(enabled);

CREATE TABLE IF NOT EXISTS mfa_recovery_codes (
  id text PRIMARY KEY,
  user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  code_hash text NOT NULL,
  used_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_mfa_recovery_codes_user_id ON mfa_recovery_codes(user_id);
CREATE INDEX IF NOT EXISTS idx_mfa_recovery_codes_used_at ON mfa_recovery_codes(used_at);

ALTER TABLE step_up_assertions ADD COLUMN IF NOT EXISTS verified_at timestamptz;
ALTER TABLE step_up_assertions ADD COLUMN IF NOT EXISTS last_activity_at timestamptz;
ALTER TABLE step_up_assertions ADD COLUMN IF NOT EXISTS idle_expires_at timestamptz;
ALTER TABLE step_up_assertions ADD COLUMN IF NOT EXISTS absolute_expires_at timestamptz;

UPDATE step_up_assertions
SET verified_at = COALESCE(verified_at, created_at),
    last_activity_at = COALESCE(last_activity_at, updated_at, created_at),
    idle_expires_at = COALESCE(idle_expires_at, expires_at),
    absolute_expires_at = COALESCE(absolute_expires_at, expires_at)
WHERE verified_at IS NULL
   OR last_activity_at IS NULL
   OR idle_expires_at IS NULL
   OR absolute_expires_at IS NULL;

ALTER TABLE step_up_assertions ALTER COLUMN verified_at SET NOT NULL;
ALTER TABLE step_up_assertions ALTER COLUMN last_activity_at SET NOT NULL;
ALTER TABLE step_up_assertions ALTER COLUMN idle_expires_at SET NOT NULL;
ALTER TABLE step_up_assertions ALTER COLUMN absolute_expires_at SET NOT NULL;
ALTER TABLE step_up_assertions DROP COLUMN IF EXISTS expires_at;

CREATE INDEX IF NOT EXISTS idx_step_up_assertions_last_activity_at ON step_up_assertions(last_activity_at);
CREATE INDEX IF NOT EXISTS idx_step_up_assertions_idle_expires_at ON step_up_assertions(idle_expires_at);
CREATE INDEX IF NOT EXISTS idx_step_up_assertions_absolute_expires_at ON step_up_assertions(absolute_expires_at);
