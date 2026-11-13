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
	project := model.Project{ID: "prj_system", SystemKey: "platform"}
	release := model.Release{ID: "rel_probe", ProjectID: project.ID, ApplicationID: "app_probe", DeploymentTargetID: "dplt_probe"}
	target := model.DeploymentTarget{
		ID: "dplt_probe", ProjectID: release.ProjectID, ApplicationID: release.ApplicationID,
		ClusterID: "rcl_probe", ServiceAccountName: trustedGatewayTrafficProbeServiceAccount,
	}

	if _, err := runner.trustedPlatformApplicationServiceAccount(t.Context(), project, release, target, "luna-system"); err == nil {
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
	got, err := runner.trustedPlatformApplicationServiceAccount(t.Context(), project, release, target, "luna-system")
	if err != nil || got != trustedGatewayTrafficProbeServiceAccount {
		t.Fatalf("trusted service account = %q, err = %v", got, err)
	}
	// Subsequent project-space releases (redeploy/image update/rollback) for the same
	// bound system component target must also be authorized and update installation release_id.
	followUpRelease := model.Release{
		ID: "rel_probe_v2", ProjectID: project.ID, ApplicationID: target.ApplicationID, DeploymentTargetID: target.ID,
	}
	got, err = runner.trustedPlatformApplicationServiceAccount(t.Context(), project, followUpRelease, target, "luna-system")
	if err != nil || got != trustedGatewayTrafficProbeServiceAccount {
		t.Fatalf("follow-up project release service account = %q, err = %v", got, err)
	}
	runner.markSystemComponentDeployment(t.Context(), followUpRelease, "deployed", "updated in project space")
	var updatedInstallation model.SystemComponentInstallation
	if err := db.First(&updatedInstallation, "id = ?", installation.ID).Error; err != nil {
		t.Fatalf("reload installation: %v", err)
	}
	if updatedInstallation.ReleaseID != followUpRelease.ID || updatedInstallation.Status != "deployed" {
		t.Fatalf("installation = %+v, want release %q deployed", updatedInstallation, followUpRelease.ID)
	}
	if _, err := runner.trustedPlatformApplicationServiceAccount(t.Context(), model.Project{ID: project.ID, SystemKey: ""}, followUpRelease, target, "luna-system"); err == nil {
		t.Fatal("non-platform project service account was accepted")
	}
	if _, err := runner.trustedPlatformApplicationServiceAccount(t.Context(), project, release, target, "other-namespace"); err == nil {
		t.Fatal("trusted service account crossed the installation namespace")
	}
}

func TestTrustedPlatformApplicationServiceAccountAllowsDefaultWorkload(t *testing.T) {
	got, err := (&Runner{}).trustedPlatformApplicationServiceAccount(t.Context(), model.Project{}, model.Release{}, model.DeploymentTarget{}, "project-demo")
	if err != nil || got != "" {
		t.Fatalf("default workload service account = %q, err = %v", got, err)
	}
}
