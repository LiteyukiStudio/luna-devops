-- Finalize the pre-release transition from startup-time GORM mutations to
-- versioned SQL migrations. This migration intentionally does not depend on
-- the project volume tables introduced by migration 000066.

ALTER TABLE IF EXISTS ai.runs
    DROP COLUMN IF EXISTS graph_version;

DROP INDEX IF EXISTS idx_applications_git_account;

ALTER TABLE IF EXISTS applications
    DROP COLUMN IF EXISTS source_type,
    DROP COLUMN IF EXISTS repository_url,
    DROP COLUMN IF EXISTS image_reference,
    DROP COLUMN IF EXISTS git_account_id,
    DROP COLUMN IF EXISTS service_port;

ALTER TABLE IF EXISTS deployment_targets
    DROP COLUMN IF EXISTS build_config_id,
    ADD COLUMN IF NOT EXISTS service_ports text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS runtime_config_set_ids text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS config_files text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS secret_files text NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS project_runtime_config_sets (
    id text PRIMARY KEY,
    project_id text NOT NULL,
    name text NOT NULL,
    env_vars text NOT NULL DEFAULT '',
    config_files text NOT NULL DEFAULT '',
    secret_refs text NOT NULL DEFAULT '',
    secret_files text NOT NULL DEFAULT '',
    enabled boolean NOT NULL DEFAULT true,
    delete_status text NOT NULL DEFAULT 'active',
    delete_message text NOT NULL DEFAULT '',
    delete_started_at timestamptz,
    delete_finished_at timestamptz,
    created_by text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);

CREATE INDEX IF NOT EXISTS idx_project_runtime_config_sets_project_id
    ON project_runtime_config_sets(project_id);
CREATE INDEX IF NOT EXISTS idx_project_runtime_config_sets_delete_status
    ON project_runtime_config_sets(delete_status);
CREATE INDEX IF NOT EXISTS idx_project_runtime_config_sets_created_by
    ON project_runtime_config_sets(created_by);
CREATE INDEX IF NOT EXISTS idx_project_runtime_config_sets_deleted_at
    ON project_runtime_config_sets(deleted_at);

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
        ON CONFLICT (user_id) DO NOTHING;
    END IF;
END
$$;

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

ALTER TABLE billing_ledger_entries
    ALTER COLUMN project_id DROP NOT NULL,
    ALTER COLUMN project_id SET DEFAULT '';

DROP INDEX IF EXISTS idx_billing_ledger_entries_project_idempotency;

CREATE UNIQUE INDEX IF NOT EXISTS idx_billing_ledger_entries_user_idempotency
    ON billing_ledger_entries(user_id, idempotency_key)
    WHERE idempotency_key <> '';

UPDATE deployment_targets
SET delete_status = 'active'
WHERE delete_status = ''
  AND deleted_at IS NULL;

UPDATE applications
SET delete_status = 'active'
WHERE delete_status = ''
  AND deleted_at IS NULL;

UPDATE projects
SET delete_status = 'active'
WHERE delete_status = ''
  AND deleted_at IS NULL;

UPDATE releases AS rel
SET deployment_target_id = target.id
FROM deployment_targets AS target
WHERE rel.deployment_target_id = ''
  AND rel.project_id = target.project_id
  AND rel.application_id = target.application_id
  AND rel.environment_id = target.environment_id
  AND target.enabled = true
  AND target.delete_status IN ('active', '')
  AND (
      SELECT COUNT(*)
      FROM deployment_targets AS candidate
      WHERE candidate.project_id = rel.project_id
        AND candidate.application_id = rel.application_id
        AND candidate.environment_id = rel.environment_id
        AND candidate.enabled = true
        AND candidate.delete_status IN ('active', '')
  ) = 1;

INSERT INTO billing_rate_rules (
    id,
    meter,
    unit,
    credits_per_unit,
    enabled,
    description,
    created_at,
    updated_at
)
VALUES
    ('brte_' || LEFT(md5('build.cpu_vcpu_minute'), 24), 'build.cpu_vcpu_minute', 'vcpu_minute', 10, true, 'Build CPU usage', now(), now()),
    ('brte_' || LEFT(md5('build.memory_gib_minute'), 24), 'build.memory_gib_minute', 'gib_minute', 2, true, 'Build memory usage', now(), now()),
    ('brte_' || LEFT(md5('runtime.cpu_vcpu_hour'), 24), 'runtime.cpu_vcpu_hour', 'vcpu_hour', 30, true, 'Runtime CPU usage', now(), now()),
    ('brte_' || LEFT(md5('runtime.memory_gib_hour'), 24), 'runtime.memory_gib_hour', 'gib_hour', 6, true, 'Runtime memory usage', now(), now()),
    ('brte_' || LEFT(md5('storage.gib_day'), 24), 'storage.gib_day', 'gib_day', 1, true, 'Persistent storage usage', now(), now()),
    ('brte_' || LEFT(md5('gateway.egress_gib'), 24), 'gateway.egress_gib', 'gib', 1, true, 'Gateway response egress traffic', now(), now()),
    ('brte_' || LEFT(md5('gateway.requests_1000'), 24), 'gateway.requests_1000', '1000_requests', 0, false, 'Gateway request count', now(), now()),
    ('brte_' || LEFT(md5('ai.input_tokens_1000'), 24), 'ai.input_tokens_1000', '1000_tokens', 1, true, 'AI model input tokens', now(), now()),
    ('brte_' || LEFT(md5('ai.output_tokens_1000'), 24), 'ai.output_tokens_1000', '1000_tokens', 4, true, 'AI model output tokens', now(), now())
ON CONFLICT (meter) DO NOTHING;
