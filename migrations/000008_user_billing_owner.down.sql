DROP INDEX IF EXISTS idx_billing_ledger_entries_user_idempotency;

CREATE UNIQUE INDEX IF NOT EXISTS idx_billing_ledger_entries_project_idempotency
  ON billing_ledger_entries(project_id, idempotency_key)
  WHERE idempotency_key <> '';

DROP INDEX IF EXISTS idx_billing_ledger_entries_user_id;
ALTER TABLE billing_ledger_entries
  DROP COLUMN IF EXISTS user_id;

UPDATE billing_ledger_entries
SET project_id = ''
WHERE project_id IS NULL;

ALTER TABLE billing_ledger_entries
  ALTER COLUMN project_id SET NOT NULL;

DROP INDEX IF EXISTS idx_billing_usage_records_billed_user_id;
ALTER TABLE billing_usage_records
  DROP COLUMN IF EXISTS billed_user_id;

DROP TABLE IF EXISTS user_wallets;

DROP INDEX IF EXISTS idx_projects_billing_owner_user_id;
ALTER TABLE projects
  DROP COLUMN IF EXISTS billing_owner_user_id;
