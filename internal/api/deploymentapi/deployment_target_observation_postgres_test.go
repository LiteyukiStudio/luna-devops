package deploymentapi

import (
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/observation"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
)

// Observation must resolve the same cluster the Worker deploys to. Before the
// unified resolver the fallback lookup inherited "is_default = true" and an
// implicit target always looked unconfigured when no default was marked.
func TestObserveDeploymentTargetResolvesTheWorkerCluster(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "deployment_target_observation",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.RuntimeCluster{})
		},
	})
	handler := &Handler{host: deploymentTargetPersistenceTestHost{db: db}}
	project := model.Project{ID: "prj_observe", KubernetesNamespace: "observe"}
	implicit := model.DeploymentTarget{ID: "dplt_implicit", ProjectID: project.ID, KubernetesName: "api-prod", Enabled: true}

	observed := handler.observeDeploymentTarget(t.Context(), project, implicit)
	if observed.Status != observation.StatusNotConfigured || observed.ObservationCode != "deployment_target.runtime_cluster_not_configured" || observed.EffectiveClusterID != "" {
		t.Fatalf("without clusters observed = %q/%q/%q, want runtime cluster not configured", observed.Status, observed.ObservationCode, observed.EffectiveClusterID)
	}

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clusters := []model.RuntimeCluster{
		{ID: "rc_project", Name: "Project", Scope: "project", DeleteStatus: "active", CreatedAt: base},
		{ID: "rc_global_old", Name: "Old", Scope: "global", DeleteStatus: "active", CreatedAt: base.Add(time.Minute)},
		{ID: "rc_global_new", Name: "New", Scope: "global", DeleteStatus: "active", CreatedAt: base.Add(2 * time.Minute)},
	}
	if err := db.Create(&clusters).Error; err != nil {
		t.Fatalf("create clusters: %v", err)
	}

	observed = handler.observeDeploymentTarget(t.Context(), project, implicit)
	if observed.EffectiveClusterID != "rc_global_old" {
		t.Fatalf("implicit target effective cluster = %q, want earliest global rc_global_old", observed.EffectiveClusterID)
	}
	if observed.Status != observation.StatusNotConfigured || observed.ObservationCode != "deployment_target.kubeconfig_not_configured" {
		t.Fatalf("implicit target observed = %q/%q, want kubeconfig not configured", observed.Status, observed.ObservationCode)
	}

	explicit := implicit
	explicit.ClusterID = "rc_project"
	if observed = handler.observeDeploymentTarget(t.Context(), project, explicit); observed.EffectiveClusterID != "rc_project" {
		t.Fatalf("explicit target effective cluster = %q, want rc_project", observed.EffectiveClusterID)
	}

	disabled := implicit
	disabled.Enabled = false
	observed = handler.observeDeploymentTarget(t.Context(), project, disabled)
	if observed.Status != observation.StatusDisabled || observed.EffectiveClusterID != "rc_global_old" || observed.ObservationCode != "deployment_target.kubeconfig_not_configured" {
		t.Fatalf("disabled target observed = %q/%q/%q, want disabled but still observed", observed.Status, observed.ObservationCode, observed.EffectiveClusterID)
	}
}
