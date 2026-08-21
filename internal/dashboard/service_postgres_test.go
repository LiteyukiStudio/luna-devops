package dashboard

import (
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
)

func TestOverviewAggregatesVisibleDashboardData(t *testing.T) {
	db := openDashboardTestDB(t)
	if err := db.AutoMigrate(
		&model.Project{},
		&model.ProjectMember{},
		&model.ProjectPin{},
		&model.Application{},
		&model.DeploymentTarget{},
		&model.BuildRun{},
		&model.Release{},
		&model.PlatformEvent{},
		&model.RuntimeCluster{},
		&model.ArtifactRegistry{},
		&model.ScopedResourceProjectBinding{},
	); err != nil {
		t.Fatalf("migrate dashboard test schema: %v", err)
	}

	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	project := model.Project{ID: "prj_visible", Name: "Visible", Identifier: "visible", NamespaceStrategy: "project", MaxConcurrentBuilds: 2, WebConsoleEnabled: true, CreatedAt: now.Add(-time.Hour)}
	hiddenProject := model.Project{ID: "prj_hidden", Name: "Hidden", Identifier: "hidden", NamespaceStrategy: "project", MaxConcurrentBuilds: 2, WebConsoleEnabled: true, CreatedAt: now}
	if err := db.Create(&[]model.Project{project, hiddenProject}).Error; err != nil {
		t.Fatalf("create projects: %v", err)
	}
	if err := db.Create(&model.ProjectMember{ID: "pm_1", ProjectID: project.ID, UserID: "usr_1", Role: authz.ProjectRoleOwner, UseCount: 3, CreatedAt: now}).Error; err != nil {
		t.Fatalf("create project member: %v", err)
	}
	if err := db.Create(&model.ProjectPin{ID: "pin_1", ProjectID: project.ID, UserID: "usr_1", PinnedAt: now, CreatedAt: now}).Error; err != nil {
		t.Fatalf("create project pin: %v", err)
	}
	application := model.Application{ID: "app_visible", ProjectID: project.ID, Name: "API", Identifier: "api", CreatedAt: now}
	hiddenApplication := model.Application{ID: "app_hidden", ProjectID: hiddenProject.ID, Name: "Hidden API", Identifier: "hidden-api", CreatedAt: now}
	if err := db.Create(&[]model.Application{application, hiddenApplication}).Error; err != nil {
		t.Fatalf("create applications: %v", err)
	}
	target := model.DeploymentTarget{ID: "dplt_1", ProjectID: project.ID, ApplicationID: application.ID, Name: "Production", Stage: "prod", CreatedAt: now}
	if err := db.Create(&target).Error; err != nil {
		t.Fatalf("create deployment target: %v", err)
	}
	if err := db.Create(&model.BuildRun{ID: "bldr_1", ProjectID: project.ID, ApplicationID: application.ID, DeploymentTargetID: target.ID, Status: "running", CreatedAt: now}).Error; err != nil {
		t.Fatalf("create build: %v", err)
	}
	if err := db.Create(&model.Release{ID: "rel_1", ProjectID: project.ID, ApplicationID: application.ID, DeploymentTargetID: target.ID, ImageRef: "example.test/api:latest", Status: "pending", CreatedAt: now}).Error; err != nil {
		t.Fatalf("create release: %v", err)
	}
	events := []model.PlatformEvent{
		{ID: "evt_2", Type: "build.failed", Category: "build", Severity: "error", Status: "failed", ProjectID: project.ID, ApplicationID: application.ID, DeploymentTargetID: target.ID, Message: "second failure", LinksJSON: `{"primary":"/events/evt_2"}`, OccurredAt: now, CreatedAt: now},
		{ID: "evt_1", Type: "build.failed", Category: "build", Severity: "error", Status: "failed", ProjectID: project.ID, ApplicationID: application.ID, DeploymentTargetID: target.ID, Message: "first failure", OccurredAt: now.Add(-time.Minute), CreatedAt: now.Add(-time.Minute)},
		{ID: "evt_hidden", Type: "build.failed", Category: "build", Severity: "error", Status: "failed", ProjectID: hiddenProject.ID, ApplicationID: hiddenApplication.ID, ActorID: "usr_other", OccurredAt: now, CreatedAt: now},
	}
	if err := db.Create(&events).Error; err != nil {
		t.Fatalf("create events: %v", err)
	}
	service := NewService(db)
	service.now = func() time.Time { return now }
	overview, err := service.Overview(t.Context(), Scope{UserID: "usr_1", VisibleProjectIDs: []string{project.ID}})
	if err != nil {
		t.Fatalf("aggregate dashboard: %v", err)
	}
	if overview.Summary.Projects != 1 || overview.Summary.Applications != 1 {
		t.Fatalf("summary visibility = %#v", overview.Summary)
	}
	if overview.Summary.ActiveBuilds != 1 || overview.Summary.ActiveReleases != 1 {
		t.Fatalf("active workflow summary = %#v", overview.Summary)
	}
	if len(overview.Projects) != 1 || !overview.Projects[0].Pinned || overview.Projects[0].ApplicationCount != 1 {
		t.Fatalf("project shortcuts = %#v", overview.Projects)
	}
	if len(overview.Attention) != 1 || overview.Attention[0].Occurrences != 2 {
		t.Fatalf("attention = %#v", overview.Attention)
	}
	if len(overview.Activities) != 2 || overview.Activities[0].Application == nil || overview.Activities[0].Application.ID != application.ID {
		t.Fatalf("activities = %#v", overview.Activities)
	}
	if overview.Readiness != (Readiness{}) {
		t.Fatalf("database overview must not synthesize live readiness = %#v", overview.Readiness)
	}
}

func openDashboardTestDB(t *testing.T) *gorm.DB {
	return testdb.Open(t, testdb.Options{SchemaPrefix: "dashboard_test"})
}
