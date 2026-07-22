ALTER TABLE users DROP COLUMN IF EXISTS auth_type;

CREATE TABLE IF NOT EXISTS auth_registration_settings (
  id text PRIMARY KEY,
  allow_email_registration boolean NOT NULL DEFAULT false,
  allow_oidc_registration boolean NOT NULL DEFAULT true,
  allow_external_identity_password boolean NOT NULL DEFAULT false,
  smtp_host text NOT NULL DEFAULT '',
  smtp_port integer NOT NULL DEFAULT 587,
  smtp_security text NOT NULL DEFAULT 'starttls',
  smtp_username text NOT NULL DEFAULT '',
  smtp_password_ref text NOT NULL DEFAULT '',
  smtp_from_address text NOT NULL DEFAULT '',
  smtp_from_name text NOT NULL DEFAULT 'Luna DevOps',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS email_registration_challenges (
  id text PRIMARY KEY,
  email text NOT NULL,
  code_hash text NOT NULL,
  language text NOT NULL DEFAULT 'zh-CN',
  attempts integer NOT NULL DEFAULT 0,
  expires_at timestamptz NOT NULL,
  consumed_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_email_registration_challenges_email ON email_registration_challenges(email);
CREATE INDEX IF NOT EXISTS idx_email_registration_challenges_expires_at ON email_registration_challenges(expires_at);
CREATE INDEX IF NOT EXISTS idx_email_registration_challenges_consumed_at ON email_registration_challenges(consumed_at);
