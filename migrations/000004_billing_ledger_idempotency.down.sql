DROP INDEX IF EXISTS idx_billing_ledger_entries_project_idempotency;
DROP INDEX IF EXISTS idx_billing_ledger_entries_idempotency_key;

ALTER TABLE billing_ledger_entries
  DROP COLUMN IF EXISTS idempotency_key;
