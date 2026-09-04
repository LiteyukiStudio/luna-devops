package dashboard

import (
	"slices"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
)

func TestDashboardVisibilityControlsOverviewAndReadinessRange(t *testing.T) {
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
	project := model.Project{ID: "prj_visible", Name: "Visible", Identifier: "visible", MaxConcurrentBuilds: 2, WebConsoleEnabled: true, CreatedAt: now.Add(-time.Hour)}
	hiddenProject := model.Project{ID: "prj_hidden", Name: "Hidden", Identifier: "hidden", MaxConcurrentBuilds: 2, WebConsoleEnabled: true, CreatedAt: now}
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
	if err := db.Create(&model.BuildRun{ID: "bldr_hidden", ProjectID: hiddenProject.ID, ApplicationID: hiddenApplication.ID, Status: "queued", CreatedAt: now}).Error; err != nil {
		t.Fatalf("create hidden build: %v", err)
	}
	if err := db.Create(&model.Release{ID: "rel_1", ProjectID: project.ID, ApplicationID: application.ID, DeploymentTargetID: target.ID, ImageRef: "example.test/api:latest", Status: "pending", CreatedAt: now}).Error; err != nil {
		t.Fatalf("create release: %v", err)
	}
	if err := db.Create(&model.Release{ID: "rel_hidden", ProjectID: hiddenProject.ID, ApplicationID: hiddenApplication.ID, ImageRef: "example.test/hidden-api:latest", Status: "running", CreatedAt: now}).Error; err != nil {
		t.Fatalf("create hidden release: %v", err)
	}
	events := []model.PlatformEvent{
		{ID: "evt_2", Type: "build.failed", Category: "build", Severity: "error", Status: "failed", ProjectID: project.ID, ApplicationID: application.ID, DeploymentTargetID: target.ID, Message: "second failure", LinksJSON: `{"primary":"/events/evt_2"}`, OccurredAt: now, CreatedAt: now},
		{ID: "evt_1", Type: "build.failed", Category: "build", Severity: "error", Status: "failed", ProjectID: project.ID, ApplicationID: application.ID, DeploymentTargetID: target.ID, Message: "first failure", OccurredAt: now.Add(-time.Minute), CreatedAt: now.Add(-time.Minute)},
		{ID: "evt_hidden", Type: "build.failed", Category: "build", Severity: "error", Status: "failed", ProjectID: hiddenProject.ID, ApplicationID: hiddenApplication.ID, ActorID: "usr_other", OccurredAt: now, CreatedAt: now},
	}
	if err := db.Create(&events).Error; err != nil {
		t.Fatalf("create events: %v", err)
	}
	clusters := []model.RuntimeCluster{
		{ID: "rcl_global", Name: "Global", Scope: "global"},
		{ID: "rcl_user", Name: "User", Scope: "user", OwnerRef: "usr_1"},
		{ID: "rcl_other_user", Name: "Other user", Scope: "user", OwnerRef: "usr_other"},
		{ID: "rcl_visible", Name: "Visible project", Scope: "project"},
		{ID: "rcl_hidden", Name: "Hidden project", Scope: "project"},
		{ID: "rcl_unbound", Name: "Unbound project", Scope: "project"},
	}
	if err := db.Create(&clusters).Error; err != nil {
		t.Fatalf("create runtime clusters: %v", err)
	}
	registries := []model.ArtifactRegistry{
		{ID: "reg_global", Name: "Global", Provider: "generic", Endpoint: "https://global.example.test", Scope: "global"},
		{ID: "reg_user", Name: "User", Provider: "generic", Endpoint: "https://user.example.test", Scope: "user", OwnerRef: "usr_1"},
		{ID: "reg_other_user", Name: "Other user", Provider: "generic", Endpoint: "https://other.example.test", Scope: "user", OwnerRef: "usr_other"},
		{ID: "reg_visible", Name: "Visible project", Provider: "generic", Endpoint: "https://visible.example.test", Scope: "project"},
		{ID: "reg_hidden", Name: "Hidden project", Provider: "generic", Endpoint: "https://hidden.example.test", Scope: "project"},
		{ID: "reg_unbound", Name: "Unbound project", Provider: "generic", Endpoint: "https://unbound.example.test", Scope: "project"},
	}
	if err := db.Create(&registries).Error; err != nil {
		t.Fatalf("create artifact registries: %v", err)
	}
	bindings := []model.ScopedResourceProjectBinding{
		{ID: "srpb_rcl_visible", ResourceType: "runtime_cluster", ResourceID: "rcl_visible", ProjectID: project.ID},
		{ID: "srpb_rcl_hidden", ResourceType: "runtime_cluster", ResourceID: "rcl_hidden", ProjectID: hiddenProject.ID},
		{ID: "srpb_reg_visible", ResourceType: "artifact_registry", ResourceID: "reg_visible", ProjectID: project.ID},
		{ID: "srpb_reg_hidden", ResourceType: "artifact_registry", ResourceID: "reg_hidden", ProjectID: hiddenProject.ID},
	}
	if err := db.Create(&bindings).Error; err != nil {
		t.Fatalf("create scoped resource bindings: %v", err)
	}
	service := NewService(db)
	service.now = func() time.Time { return now }

	relatedScope := Scope{UserID: "usr_1", VisibleProjectIDs: []string{project.ID}}
	overview, err := service.Overview(t.Context(), relatedScope)
	if err != nil {
		t.Fatalf("aggregate related dashboard: %v", err)
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
	relatedClusters, relatedRegistries, err := service.ReadinessResources(t.Context(), relatedScope)
	if err != nil {
		t.Fatalf("list related readiness resources: %v", err)
	}
	assertDashboardResourceIDs(t, runtimeClusterIDs(relatedClusters), []string{"rcl_global", "rcl_user", "rcl_visible"})
	assertDashboardResourceIDs(t, artifactRegistryIDs(relatedRegistries), []string{"reg_global", "reg_user", "reg_visible"})

	allScope := Scope{UserID: "usr_1", AllProjects: true, VisibleProjectIDs: []string{project.ID}}
	overview, err = service.Overview(t.Context(), allScope)
	if err != nil {
		t.Fatalf("aggregate all-project dashboard: %v", err)
	}
	if overview.Summary.Projects != 2 || overview.Summary.Applications != 2 {
		t.Fatalf("all-project summary visibility = %#v", overview.Summary)
	}
	if overview.Summary.ActiveBuilds != 2 || overview.Summary.ActiveReleases != 2 {
		t.Fatalf("all-project workflow summary = %#v", overview.Summary)
	}
	if len(overview.Projects) != 2 {
		t.Fatalf("all-project shortcuts = %#v", overview.Projects)
	}
	if len(overview.Attention) != 2 || len(overview.Activities) != 3 {
		t.Fatalf("all-project events: attention = %#v, activities = %#v", overview.Attention, overview.Activities)
	}
	allClusters, allRegistries, err := service.ReadinessResources(t.Context(), allScope)
	if err != nil {
		t.Fatalf("list all-project readiness resources: %v", err)
	}
	assertDashboardResourceIDs(t, runtimeClusterIDs(allClusters), []string{"rcl_global", "rcl_hidden", "rcl_unbound", "rcl_user", "rcl_visible"})
	assertDashboardResourceIDs(t, artifactRegistryIDs(allRegistries), []string{"reg_global", "reg_hidden", "reg_unbound", "reg_user", "reg_visible"})
}

func runtimeClusterIDs(clusters []model.RuntimeCluster) []string {
	ids := make([]string, 0, len(clusters))
	for _, cluster := range clusters {
		ids = append(ids, cluster.ID)
	}
	return ids
}

func artifactRegistryIDs(registries []model.ArtifactRegistry) []string {
	ids := make([]string, 0, len(registries))
	for _, registry := range registries {
		ids = append(ids, registry.ID)
	}
	return ids
}

func assertDashboardResourceIDs(t *testing.T, got []string, want []string) {
	t.Helper()
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("resource IDs = %v, want %v", got, want)
	}
}

func openDashboardTestDB(t *testing.T) *gorm.DB {
	return testdb.Open(t, testdb.Options{SchemaPrefix: "dashboard_test"})
}
