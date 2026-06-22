ALTER TABLE projects
  ADD COLUMN IF NOT EXISTS billing_owner_user_id text NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_projects_billing_owner_user_id
  ON projects(billing_owner_user_id);

UPDATE projects
SET billing_owner_user_id = owners.user_id
FROM (
  SELECT DISTINCT ON (project_id) project_id, user_id
  FROM project_members
  WHERE role = 'owner'
  ORDER BY project_id, created_at ASC
) AS owners
WHERE projects.id = owners.project_id
  AND projects.billing_owner_user_id = '';

CREATE TABLE IF NOT EXISTS user_wallets (
  id text PRIMARY KEY,
  user_id text NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
  balance_credits numeric(24,8) NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_user_wallets_user_id
  ON user_wallets(user_id);

DO $$
BEGIN
  IF to_regclass('project_wallets') IS NOT NULL THEN
    INSERT INTO user_wallets(id, user_id, balance_credits, created_at, updated_at)
    SELECT
      'wlt_' || md5(projects.billing_owner_user_id),
      projects.billing_owner_user_id,
      COALESCE(SUM(project_wallets.balance_credits), 0),
      MIN(project_wallets.created_at),
      MAX(project_wallets.updated_at)
    FROM project_wallets
    JOIN projects ON projects.id = project_wallets.project_id
    WHERE projects.billing_owner_user_id <> ''
    GROUP BY projects.billing_owner_user_id
    ON CONFLICT (user_id) DO UPDATE
      SET balance_credits = user_wallets.balance_credits + EXCLUDED.balance_credits,
          updated_at = GREATEST(user_wallets.updated_at, EXCLUDED.updated_at);
  END IF;
END $$;

ALTER TABLE billing_usage_records
  ADD COLUMN IF NOT EXISTS billed_user_id text NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_billing_usage_records_billed_user_id
  ON billing_usage_records(billed_user_id);

UPDATE billing_usage_records AS usage
SET billed_user_id = projects.billing_owner_user_id
FROM projects
WHERE usage.project_id = projects.id
  AND usage.billed_user_id = '';

UPDATE billing_usage_records AS usage
SET billed_user_id = owners.user_id
FROM (
  SELECT DISTINCT ON (project_id) project_id, user_id
  FROM project_members
  WHERE role = 'owner'
  ORDER BY project_id, created_at ASC
) AS owners
WHERE usage.project_id = owners.project_id
  AND usage.billed_user_id = '';

ALTER TABLE billing_ledger_entries
  ADD COLUMN IF NOT EXISTS user_id text NOT NULL DEFAULT '';

ALTER TABLE billing_ledger_entries
  ALTER COLUMN project_id DROP NOT NULL,
  ALTER COLUMN project_id SET DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_billing_ledger_entries_user_id
  ON billing_ledger_entries(user_id);

UPDATE billing_ledger_entries AS ledger
SET user_id = projects.billing_owner_user_id
FROM projects
WHERE ledger.project_id = projects.id
  AND ledger.user_id = '';

UPDATE billing_ledger_entries AS ledger
SET user_id = owners.user_id
FROM (
  SELECT DISTINCT ON (project_id) project_id, user_id
  FROM project_members
  WHERE role = 'owner'
  ORDER BY project_id, created_at ASC
) AS owners
WHERE ledger.project_id = owners.project_id
  AND ledger.user_id = '';

DROP INDEX IF EXISTS idx_billing_ledger_entries_project_idempotency;

CREATE UNIQUE INDEX IF NOT EXISTS idx_billing_ledger_entries_user_idempotency
  ON billing_ledger_entries(user_id, idempotency_key)
  WHERE idempotency_key <> '';
