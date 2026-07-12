ALTER TABLE step_up_assertions ADD COLUMN IF NOT EXISTS expires_at timestamptz;
UPDATE step_up_assertions
SET expires_at = LEAST(idle_expires_at, absolute_expires_at)
WHERE expires_at IS NULL;
ALTER TABLE step_up_assertions ALTER COLUMN expires_at SET NOT NULL;
CREATE INDEX IF NOT EXISTS idx_step_up_assertions_expires_at ON step_up_assertions(expires_at);

DROP INDEX IF EXISTS idx_step_up_assertions_absolute_expires_at;
DROP INDEX IF EXISTS idx_step_up_assertions_idle_expires_at;
DROP INDEX IF EXISTS idx_step_up_assertions_last_activity_at;
ALTER TABLE step_up_assertions DROP COLUMN IF EXISTS absolute_expires_at;
ALTER TABLE step_up_assertions DROP COLUMN IF EXISTS idle_expires_at;
ALTER TABLE step_up_assertions DROP COLUMN IF EXISTS last_activity_at;
ALTER TABLE step_up_assertions DROP COLUMN IF EXISTS verified_at;

DELETE FROM secret_values WHERE resource LIKE 'mfa:%:totp';

DROP TABLE IF EXISTS mfa_recovery_codes;
DROP TABLE IF EXISTS user_mfa_configs;
