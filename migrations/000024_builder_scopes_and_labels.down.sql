ALTER TABLE build_runs DROP COLUMN IF EXISTS build_labels;
ALTER TABLE applications DROP COLUMN IF EXISTS build_labels;
ALTER TABLE builder_agents DROP COLUMN IF EXISTS scopes;
