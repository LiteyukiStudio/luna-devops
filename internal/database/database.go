package database

import (
	"fmt"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/billing"
	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func Open(databaseURL string) (*gorm.DB, error) {
	if strings.HasPrefix(databaseURL, "postgres://") || strings.HasPrefix(databaseURL, "postgresql://") {
		return gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
	}

	return nil, fmt.Errorf("unsupported database url: %s", databaseURL)
}

func Migrate(db *gorm.DB) error {
	if err := cleanupApplicationDeliveryColumns(db); err != nil {
		return err
	}
	if err := db.AutoMigrate(
		&model.User{},
		&model.UserSession{},
		&model.UserRememberToken{},
		&model.AuthProvider{},
		&model.ExternalIdentity{},
		&model.AuthAdmissionPolicy{},
		&model.Project{},
		&model.ProjectMember{},
		&model.ProjectPin{},
		&model.UserWallet{},
		&model.ProjectHookConfig{},
		&model.HookRun{},
		&model.HookRunLog{},
		&model.AccessToken{},
		&model.AuditLog{},
		&model.WorkerTaskEvent{},
		&model.SecretValue{},
		&model.ScopedResourceProjectBinding{},
		&model.Application{},
		&model.GitProvider{},
		&model.GitAccount{},
		&model.RepositoryBinding{},
		&model.ArtifactRegistry{},
		&model.RegistryCredential{},
		&model.ContainerImage{},
		&model.DeploymentTargetHookBinding{},
		&model.BuildVariableSet{},
		&model.BuildRun{},
		&model.BuildJob{},
		&model.BuildLog{},
		&model.BillingRateRule{},
		&model.BillingUsageRecord{},
		&model.BillingLedgerEntry{},
		&model.RuntimeCluster{},
		&model.Environment{},
		&model.Release{},
		&model.ReleaseLog{},
		&model.ProjectRuntimeConfigSet{},
		&model.DeploymentTarget{},
		&model.GatewayRoute{},
		&model.AppConfig{},
	); err != nil {
		return err
	}
	if err := migrateUserBillingOwnership(db); err != nil {
		return err
	}
	if err := backfillReleaseDeploymentTargets(db); err != nil {
		return err
	}
	return (billing.Service{DB: db}).EnsureDefaultRateRules()
}

func cleanupApplicationDeliveryColumns(db *gorm.DB) error {
	for _, statement := range cleanupApplicationDeliveryStatements() {
		if err := db.Exec(statement).Error; err != nil {
			return fmt.Errorf("cleanup application delivery columns: %w", err)
		}
	}
	return nil
}

func cleanupApplicationDeliveryStatements() []string {
	return []string{
		"DROP INDEX IF EXISTS idx_applications_git_account",
		"ALTER TABLE IF EXISTS applications DROP COLUMN IF EXISTS source_type",
		"ALTER TABLE IF EXISTS applications DROP COLUMN IF EXISTS repository_url",
		"ALTER TABLE IF EXISTS applications DROP COLUMN IF EXISTS image_reference",
		"ALTER TABLE IF EXISTS applications DROP COLUMN IF EXISTS git_account_id",
		"ALTER TABLE IF EXISTS applications DROP COLUMN IF EXISTS service_port",
		"ALTER TABLE IF EXISTS deployment_targets DROP COLUMN IF EXISTS build_config_id",
	}
}

func migrateUserBillingOwnership(db *gorm.DB) error {
	for _, statement := range userBillingOwnershipStatements() {
		if err := db.Exec(statement).Error; err != nil {
			return fmt.Errorf("migrate user billing ownership: %w", err)
		}
	}
	return nil
}

func userBillingOwnershipStatements() []string {
	return []string{
		`UPDATE projects
SET billing_owner_user_id = owners.user_id
FROM (
  SELECT DISTINCT ON (project_id) project_id, user_id
  FROM project_members
  WHERE role = 'owner'
  ORDER BY project_id, created_at ASC
) AS owners
WHERE projects.id = owners.project_id
  AND projects.billing_owner_user_id = ''`,
		`DO $$
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
END $$`,
		`UPDATE billing_usage_records AS usage
SET billed_user_id = projects.billing_owner_user_id
FROM projects
WHERE usage.project_id = projects.id
  AND usage.billed_user_id = ''`,
		`UPDATE billing_usage_records AS usage
SET billed_user_id = owners.user_id
FROM (
  SELECT DISTINCT ON (project_id) project_id, user_id
  FROM project_members
  WHERE role = 'owner'
  ORDER BY project_id, created_at ASC
) AS owners
WHERE usage.project_id = owners.project_id
  AND usage.billed_user_id = ''`,
		`UPDATE billing_ledger_entries AS ledger
SET user_id = projects.billing_owner_user_id
FROM projects
WHERE ledger.project_id = projects.id
  AND ledger.user_id = ''`,
		`UPDATE billing_ledger_entries AS ledger
SET user_id = owners.user_id
FROM (
  SELECT DISTINCT ON (project_id) project_id, user_id
  FROM project_members
  WHERE role = 'owner'
  ORDER BY project_id, created_at ASC
) AS owners
WHERE ledger.project_id = owners.project_id
  AND ledger.user_id = ''`,
		`ALTER TABLE billing_ledger_entries
ALTER COLUMN project_id DROP NOT NULL`,
		`ALTER TABLE billing_ledger_entries
ALTER COLUMN project_id SET DEFAULT ''`,
		`DROP INDEX IF EXISTS idx_billing_ledger_entries_project_idempotency`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_billing_ledger_entries_user_idempotency
ON billing_ledger_entries(user_id, idempotency_key)
WHERE idempotency_key <> ''`,
	}
}

func backfillReleaseDeploymentTargets(db *gorm.DB) error {
	statement := `UPDATE releases AS rel
SET deployment_target_id = target.id
FROM deployment_targets AS target
WHERE rel.deployment_target_id = ''
  AND rel.project_id = target.project_id
  AND rel.application_id = target.application_id
  AND rel.environment_id = target.environment_id
  AND target.enabled = true
  AND target.delete_status = 'active'
  AND (
    SELECT COUNT(*)
    FROM deployment_targets AS candidate
    WHERE candidate.project_id = rel.project_id
      AND candidate.application_id = rel.application_id
      AND candidate.environment_id = rel.environment_id
      AND candidate.enabled = true
      AND candidate.delete_status = 'active'
  ) = 1`
	if err := db.Exec(statement).Error; err != nil {
		return fmt.Errorf("backfill release deployment targets: %w", err)
	}
	return nil
}
