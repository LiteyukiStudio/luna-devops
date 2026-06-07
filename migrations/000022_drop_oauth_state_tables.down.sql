CREATE TABLE IF NOT EXISTS oidc_auth_states (
  id text PRIMARY KEY,
  state_hash text NOT NULL UNIQUE,
  nonce text NOT NULL,
  provider_id text NOT NULL,
  user_id text,
  mode text NOT NULL,
  redirect_path text NOT NULL DEFAULT '/projects',
  expires_at timestamptz NOT NULL,
  created_at timestamptz,
  updated_at timestamptz
);

CREATE INDEX IF NOT EXISTS idx_oidc_auth_states_expires_at ON oidc_auth_states (expires_at);

CREATE TABLE IF NOT EXISTS git_oauth_states (
  id text PRIMARY KEY,
  state_hash text NOT NULL UNIQUE,
  provider_id text NOT NULL,
  user_id text NOT NULL,
  redirect_path text NOT NULL DEFAULT '/projects',
  frontend_origin text NOT NULL DEFAULT '',
  callback_origin text NOT NULL DEFAULT '',
  expires_at timestamptz NOT NULL,
  created_at timestamptz,
  updated_at timestamptz
);

CREATE INDEX IF NOT EXISTS idx_git_oauth_states_expires_at ON git_oauth_states (expires_at);
