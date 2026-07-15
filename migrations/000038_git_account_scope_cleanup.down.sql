ALTER TABLE git_accounts
ADD COLUMN IF NOT EXISTS access_scope text NOT NULL DEFAULT 'personal';

UPDATE git_accounts
SET access_scope = CASE
  WHEN scope = 'user' THEN 'personal'
  ELSE 'provider'
END;
