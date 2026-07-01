ALTER TABLE IF EXISTS deployment_targets
  ADD COLUMN IF NOT EXISTS runtime_config_refs text NOT NULL DEFAULT '';
