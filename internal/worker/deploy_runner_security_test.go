package worker

import (
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
)

func TestTrustedPlatformApplicationServiceAccountRequiresInstallationPlan(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "trusted_platform_service_account",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.SystemComponentInstallation{})
		},
	})
	runner := &Runner{db: db}
	release := model.Release{ID: "rel_probe", ProjectID: "prj_system", ApplicationID: "app_probe"}
	target := model.DeploymentTarget{
		ID: "dplt_probe", ProjectID: release.ProjectID, ApplicationID: release.ApplicationID,
		ClusterID: "rcl_probe", ServiceAccountName: trustedGatewayTrafficProbeServiceAccount,
	}

	if _, err := runner.trustedPlatformApplicationServiceAccount(t.Context(), release, target, "luna-system"); err == nil {
		t.Fatal("unlinked deployment service account was accepted")
	}
	installation := model.SystemComponentInstallation{
		ID: "scmp_probe", ComponentID: trustedGatewayTrafficProbeComponentID, RuntimeClusterID: target.ClusterID,
		ProjectID: release.ProjectID, ApplicationID: release.ApplicationID, DeploymentTargetID: target.ID,
		ReleaseID: release.ID, Namespace: "luna-system", Status: "deploying",
	}
	if err := db.Create(&installation).Error; err != nil {
		t.Fatalf("create trusted installation: %v", err)
	}
	got, err := runner.trustedPlatformApplicationServiceAccount(t.Context(), release, target, "luna-system")
	if err != nil || got != trustedGatewayTrafficProbeServiceAccount {
		t.Fatalf("trusted service account = %q, err = %v", got, err)
	}
	if _, err := runner.trustedPlatformApplicationServiceAccount(t.Context(), release, target, "other-namespace"); err == nil {
		t.Fatal("trusted service account crossed the installation namespace")
	}
}

func TestTrustedPlatformApplicationServiceAccountAllowsDefaultWorkload(t *testing.T) {
	got, err := (&Runner{}).trustedPlatformApplicationServiceAccount(t.Context(), model.Release{}, model.DeploymentTarget{}, "project-demo")
	if err != nil || got != "" {
		t.Fatalf("default workload service account = %q, err = %v", got, err)
	}
}
