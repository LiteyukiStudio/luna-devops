package projectapi

import (
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
)

func TestLoadProjectDiagnosticsSnapshotFiltersWindowAndsummarizesRoutes(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "project_diagnostics_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.Application{}, &model.BuildRun{}, &model.Release{})
		},
	})
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	at := func(offset time.Duration) *time.Time {
		value := now.Add(offset)
		return &value
	}

	apps := []model.Application{
		{ID: "app_1", ProjectID: "prj_diag", Name: "Order", Identifier: "order", DeleteStatus: "active"},
		{ID: "app_del", ProjectID: "prj_diag", Name: "Deleting", Identifier: "deleting", DeleteStatus: "deleting"},
	}
	if err := db.Create(&apps).Error; err != nil {
		t.Fatalf("create apps: %v", err)
	}

	builds := []model.BuildRun{
		{ID: "bld_running", ProjectID: "prj_diag", ApplicationID: "app_1", DeploymentTargetID: "tgt_1", Status: "running", TriggerType: "manual", SourceBranch: "main", CreatedAt: now.Add(-2 * time.Minute)},
		{ID: "bld_failed_recent", ProjectID: "prj_diag", ApplicationID: "app_1", DeploymentTargetID: "tgt_1", Status: "failed", FailureCode: "build.dockerfile_failed", TriggerType: "webhook", CreatedAt: now.Add(-2 * time.Hour), FinishedAt: at(-time.Hour)},
		{ID: "bld_failed_old", ProjectID: "prj_diag", ApplicationID: "app_1", DeploymentTargetID: "tgt_1", Status: "failed", FailureCode: "build.timeout", TriggerType: "manual", CreatedAt: now.Add(-10 * 24 * time.Hour), FinishedAt: at(-9 * 24 * time.Hour)},
		{ID: "bld_succeeded", ProjectID: "prj_diag", ApplicationID: "app_1", DeploymentTargetID: "tgt_1", Status: "succeeded", TriggerType: "manual", CreatedAt: now.Add(-time.Minute), FinishedAt: at(0)},
	}
	if err := db.Create(&builds).Error; err != nil {
		t.Fatalf("create builds: %v", err)
	}

	releases := []model.Release{
		{ID: "rel_running", ProjectID: "prj_diag", ApplicationID: "app_1", DeploymentTargetID: "tgt_1", ImageRef: "ghcr.io/org/order:v2", Type: "deploy", Status: "running", Revision: 2, CreatedAt: now.Add(-time.Minute)},
		{ID: "rel_failed_recent", ProjectID: "prj_diag", ApplicationID: "app_1", DeploymentTargetID: "tgt_1", ImageRef: "ghcr.io/org/order:v1", Type: "deploy", Status: "failed", FailureCode: "release.image_pull_backoff", Revision: 1, CreatedAt: now.Add(-3 * time.Hour), FinishedAt: at(-2 * time.Hour)},
		{ID: "rel_failed_old", ProjectID: "prj_diag", ApplicationID: "app_1", DeploymentTargetID: "tgt_1", ImageRef: "ghcr.io/org/order:v0", Type: "deploy", Status: "failed", Revision: 1, CreatedAt: now.Add(-9 * 24 * time.Hour), FinishedAt: at(-8 * 24 * time.Hour)},
	}
	if err := db.Create(&releases).Error; err != nil {
		t.Fatalf("create releases: %v", err)
	}

	snapshot, err := loadProjectDiagnosticsSnapshot(db, "prj_diag", now)
	if err != nil {
		t.Fatalf("loadProjectDiagnosticsSnapshot() error = %v", err)
	}
	if snapshot.Applications != 1 || snapshot.Builds.ActiveCount != 1 || snapshot.Builds.FailedCount != 1 {
		t.Fatalf("unexpected build/application counts: %#v", snapshot)
	}
	if len(snapshot.Builds.RecentIssues) != 2 || snapshot.Builds.RecentIssues[0].ID != "bld_running" || snapshot.Builds.RecentIssues[1].ID != "bld_failed_recent" {
		t.Fatalf("unexpected build recentIssues: %#v", snapshot.Builds.RecentIssues)
	}
	if snapshot.Releases.ActiveCount != 1 || snapshot.Releases.FailedCount != 1 || len(snapshot.Releases.RecentIssues) != 2 {
		t.Fatalf("unexpected release diagnostics: %#v", snapshot.Releases)
	}

	routes := summarizeProjectDiagnosticsAccessRoutes([]model.GatewayRoute{
		{ID: "rt_ok", ApplicationID: "app_1", DeploymentTargetID: "tgt_1", Host: "ok.example.com", Path: "/", TLSMode: "http-challenge", Status: "ready", CertificateStatus: "issued", AccessURL: "https://ok.example.com/"},
		{ID: "rt_cert_failed", ApplicationID: "app_1", DeploymentTargetID: "tgt_1", Host: "cert.example.com", Path: "/", TLSMode: "http-challenge", Status: "ready", CertificateStatus: "failed", AccessURL: "https://cert.example.com/"},
		{ID: "rt_unavail", ApplicationID: "app_1", DeploymentTargetID: "tgt_1", Host: "bad.example.com", Path: "/api", TLSMode: "http-only", Status: "unavailable", ObservationCode: "gateway_route.cluster_unavailable", CertificateStatus: "disabled"},
	}, now)
	if routes.Summary.Status != "unavailable" || routes.Summary.Ready != nil {
		t.Fatalf("expected unavailable summary without fake ready=0, got %#v", routes.Summary)
	}
	if len(routes.UnhealthyItems) != 2 || routes.UnhealthyItems[0].ID != "rt_cert_failed" || routes.UnhealthyItems[0].CertificateStatus != "failed" || routes.UnhealthyItems[0].AccessURL != "https://cert.example.com/" || routes.UnhealthyItems[1].ID != "rt_unavail" {
		t.Fatalf("expected cert-failed and unavailable routes in unhealthyItems, got %#v", routes.UnhealthyItems)
	}

	degradedCertRoutes := summarizeProjectDiagnosticsAccessRoutes([]model.GatewayRoute{
		{ID: "rt_issued", ApplicationID: "app_1", DeploymentTargetID: "tgt_1", Host: "ok.example.com", Path: "/", TLSMode: "http-challenge", Status: "ready", CertificateStatus: "issued"},
		{ID: "rt_pending_cert", ApplicationID: "app_1", DeploymentTargetID: "tgt_1", Host: "pending.example.com", Path: "/", TLSMode: "http-challenge", Status: "ready", CertificateStatus: "pending"},
	}, now)
	if degradedCertRoutes.Summary.Status != "degraded" || degradedCertRoutes.Summary.Ready == nil || *degradedCertRoutes.Summary.Ready != 1 {
		t.Fatalf("expected degraded summary with ready=1 when http-challenge cert is pending, got %#v", degradedCertRoutes.Summary)
	}
	if len(degradedCertRoutes.UnhealthyItems) != 1 || degradedCertRoutes.UnhealthyItems[0].ID != "rt_pending_cert" {
		t.Fatalf("expected pending-cert route in unhealthyItems, got %#v", degradedCertRoutes.UnhealthyItems)
	}

	unavailCertRoutes := summarizeProjectDiagnosticsAccessRoutes([]model.GatewayRoute{
		{ID: "rt_cert_unavail", ApplicationID: "app_1", DeploymentTargetID: "tgt_1", Host: "unavail.example.com", Path: "/", TLSMode: "http-challenge", Status: "ready", CertificateStatus: "unavailable"},
	}, now)
	if unavailCertRoutes.Summary.Status != "unavailable" || unavailCertRoutes.Summary.Ready != nil {
		t.Fatalf("expected unavailable summary when http-challenge cert status is unavailable, got %#v", unavailCertRoutes.Summary)
	}
}
