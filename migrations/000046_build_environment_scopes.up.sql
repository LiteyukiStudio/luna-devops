CREATE TABLE IF NOT EXISTS build_environment_configs (
    id text PRIMARY KEY,
    scope text NOT NULL,
    scope_ref text NOT NULL,
    variables text NOT NULL DEFAULT '{}',
    secret_refs text NOT NULL DEFAULT '{}',
    updated_by text NOT NULL,
    created_at timestamptz,
    updated_at timestamptz
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_build_environment_scope_ref
    ON build_environment_configs (scope, scope_ref);
CREATE INDEX IF NOT EXISTS idx_build_environment_configs_updated_by
    ON build_environment_configs (updated_by);

ALTER TABLE build_runs
    ADD COLUMN IF NOT EXISTS build_variables_snapshot text NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS build_secret_refs_snapshot text NOT NULL DEFAULT '{}';
