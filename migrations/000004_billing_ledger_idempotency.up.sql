ALTER TABLE billing_ledger_entries
  ADD COLUMN IF NOT EXISTS idempotency_key text NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_billing_ledger_entries_idempotency_key
  ON billing_ledger_entries(idempotency_key);

CREATE UNIQUE INDEX IF NOT EXISTS idx_billing_ledger_entries_project_idempotency
  ON billing_ledger_entries(project_id, idempotency_key)
  WHERE idempotency_key <> '';
