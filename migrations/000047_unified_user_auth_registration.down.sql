DROP TABLE IF EXISTS email_registration_challenges;
DROP TABLE IF EXISTS auth_registration_settings;

ALTER TABLE users ADD COLUMN IF NOT EXISTS auth_type text NOT NULL DEFAULT 'local';
UPDATE users
SET auth_type = 'oidc'
WHERE password = ''
  AND EXISTS (SELECT 1 FROM external_identities WHERE external_identities.user_id = users.id);
