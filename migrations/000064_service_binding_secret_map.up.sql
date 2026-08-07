ALTER TABLE service_bindings ADD COLUMN IF NOT EXISTS secret_map text NOT NULL DEFAULT '';
