package platformevent

import (
	"context"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/testdb"
)

func TestBuildRunDisplayNameUsesSourceRevision(t *testing.T) {
	cases := []struct {
		run  model.BuildRun
		want string
	}{
		{run: model.BuildRun{SourceBranch: "main", SourceCommit: "1a2b3c4d5e6f"}, want: "main@1a2b3c4"},
		{run: model.BuildRun{SourceBranch: "main", SourceTag: "v1.2.0", SourceCommit: "1a2b3c4"}, want: "v1.2.0@1a2b3c4"},
		{run: model.BuildRun{SourceBranch: "main"}, want: "main"},
		{run: model.BuildRun{SourceCommit: "1a2b3c4d5e6f"}, want: "1a2b3c4"},
		{run: model.BuildRun{}, want: ""},
	}
	for _, testCase := range cases {
		if got := buildRunDisplayName(testCase.run); got != testCase.want {
			t.Fatalf("buildRunDisplayName(%+v) = %q, want %q", testCase.run, got, testCase.want)
		}
	}
}

func TestGatewayRouteDisplayNameOmitsRootPath(t *testing.T) {
	if got := gatewayRouteDisplayName(model.GatewayRoute{Host: "blog.example.com", Path: "/"}); got != "blog.example.com" {
		t.Fatalf("root path name = %q", got)
	}
	if got := gatewayRouteDisplayName(model.GatewayRoute{Host: "blog.example.com", Path: "/api"}); got != "blog.example.com/api" {
		t.Fatalf("sub path name = %q", got)
	}
	if got := gatewayRouteDisplayName(model.GatewayRoute{Path: "/api"}); got != "" {
		t.Fatalf("hostless route name = %q, want empty", got)
	}
}

func TestResourceNamesWithoutDatabaseOrEventsIsEmpty(t *testing.T) {
	names, err := (Service{}).ResourceNames(context.Background(), []model.PlatformEvent{{ID: "evt_1", ProjectID: "prj_1", ResourceType: "project", ResourceID: "prj_1"}})
	if err != nil || len(names) != 0 {
		t.Fatalf("names = %v, err = %v; want empty", names, err)
	}
}

func TestResourceNamesResolvesEveryEventResourceTypeWithinProject(t *testing.T) {
	db := testdb.Open(t, testdb.Options{SchemaPrefix: "platformevent_names_test"})
	if err := db.AutoMigrate(
		&model.Project{},
		&model.Application{},
		&model.BuildRun{},
		&model.Release{},
		&model.HookRun{},
		&model.GatewayRoute{},
		&model.ServiceBinding{},
		&model.RepositoryBinding{},
	); err != nil {
		t.Fatalf("migrate resource name schema: %v", err)
	}
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	mustCreate := func(value any) {
		t.Helper()
		if err := db.Create(value).Error; err != nil {
			t.Fatalf("create %T: %v", value, err)
		}
	}
	mustCreate(&[]model.Project{
		{ID: "prj_1", Name: "博客", Identifier: "blog", CreatedAt: now},
		{ID: "prj_other", Name: "Other", Identifier: "other", CreatedAt: now},
	})
	mustCreate(&[]model.Application{
		{ID: "app_api", ProjectID: "prj_1", Name: "blog-api", Identifier: "blog-api", CreatedAt: now},
		{ID: "app_other", ProjectID: "prj_other", Name: "other-api", Identifier: "other-api", CreatedAt: now},
	})
	mustCreate(&model.BuildRun{ID: "bldr_1", ProjectID: "prj_1", SourceBranch: "main", SourceCommit: "1a2b3c4d5e6f", CreatedAt: now})
	mustCreate(&model.Release{ID: "rel_1", ProjectID: "prj_1", ApplicationID: "app_api", ImageRef: "example.test/api:1", Revision: 3, CreatedAt: now})
	mustCreate(&model.HookRun{ID: "hkr_1", ProjectID: "prj_1", Name: "migrate-db", Phase: "pre_deploy", ScriptSnapshot: "true", CreatedAt: now})
	mustCreate(&model.GatewayRoute{ID: "gwr_1", ProjectID: "prj_1", ApplicationID: "app_api", Host: "blog.example.com", Path: "/api", CreatedAt: now})
	mustCreate(&[]model.ServiceBinding{
		{ID: "sb_1", ProjectID: "prj_1", SourceApplicationID: "app_web", SourceDeploymentTargetID: "dplt_web", TargetApplicationID: "app_api", TargetDeploymentTargetID: "dplt_api", TargetPortName: "http", TargetPort: 80, Protocol: "http", InjectionMode: "env", CreatedBy: "usr_1", CreatedAt: now},
		{ID: "sb_cross", ProjectID: "prj_1", SourceApplicationID: "app_web", SourceDeploymentTargetID: "dplt_web", TargetApplicationID: "app_other", TargetDeploymentTargetID: "dplt_other", TargetPortName: "http", TargetPort: 80, Protocol: "http", InjectionMode: "env", CreatedBy: "usr_1", CreatedAt: now},
	})
	mustCreate(&[]model.RepositoryBinding{
		{ID: "rpb_1", ProjectID: "prj_1", ApplicationID: "app_api", GitProviderID: "gp_1", GitAccountID: "ga_1", Owner: "LiteyukiStudio", Repo: "neo-blog", CloneURL: "https://example.test/neo-blog.git", CreatedAt: now},
		{ID: "rpb_deleted", ProjectID: "prj_1", ApplicationID: "app_api", GitProviderID: "gp_1", GitAccountID: "ga_1", Owner: "LiteyukiStudio", Repo: "gone", CloneURL: "https://example.test/gone.git", CreatedAt: now},
	})
	if err := db.Delete(&model.RepositoryBinding{}, "id = ?", "rpb_deleted").Error; err != nil {
		t.Fatalf("soft delete repository binding: %v", err)
	}

	events := []model.PlatformEvent{
		{ID: "evt_project", ProjectID: "prj_1", ResourceType: "project", ResourceID: "prj_1"},
		{ID: "evt_build", ProjectID: "prj_1", ResourceType: "build", ResourceID: "bldr_1"},
		{ID: "evt_release", ProjectID: "prj_1", ResourceType: "release", ResourceID: "rel_1"},
		{ID: "evt_hook", ProjectID: "prj_1", ResourceType: "hook", ResourceID: "hkr_1"},
		{ID: "evt_gateway", ProjectID: "prj_1", ResourceType: "gateway", ResourceID: "gwr_1"},
		{ID: "evt_certificate", ProjectID: "prj_1", ResourceType: "certificate", ResourceID: "gwr_1"},
		{ID: "evt_binding", ProjectID: "prj_1", ResourceType: "service_binding", ResourceID: "sb_1"},
		{ID: "evt_repository", ProjectID: "prj_1", ResourceType: "repository_binding", ResourceID: "rpb_1"},
		{ID: "evt_repository_again", ProjectID: "prj_1", ResourceType: "repository_binding", ResourceID: "rpb_1"},
		// Fallback cases: deleted, missing, cross-project, cross-project target and unknown type.
		{ID: "evt_deleted", ProjectID: "prj_1", ResourceType: "repository_binding", ResourceID: "rpb_deleted"},
		{ID: "evt_missing", ProjectID: "prj_1", ResourceType: "build", ResourceID: "bldr_missing"},
		{ID: "evt_cross_project", ProjectID: "prj_other", ResourceType: "repository_binding", ResourceID: "rpb_1"},
		{ID: "evt_cross_target", ProjectID: "prj_1", ResourceType: "service_binding", ResourceID: "sb_cross"},
		{ID: "evt_other_project", ProjectID: "prj_1", ResourceType: "project", ResourceID: "prj_other"},
		{ID: "evt_unknown", ProjectID: "prj_1", ResourceType: "runtime_cluster", ResourceID: "rcl_1"},
	}
	names, err := (Service{DB: db}).ResourceNames(context.Background(), events)
	if err != nil {
		t.Fatalf("resolve resource names: %v", err)
	}
	want := map[string]string{
		"evt_project":          "博客",
		"evt_build":            "main@1a2b3c4",
		"evt_release":          "#3",
		"evt_hook":             "migrate-db",
		"evt_gateway":          "blog.example.com/api",
		"evt_certificate":      "blog.example.com/api",
		"evt_binding":          "blog-api:http",
		"evt_repository":       "LiteyukiStudio/neo-blog",
		"evt_repository_again": "LiteyukiStudio/neo-blog",
	}
	if len(names) != len(want) {
		t.Fatalf("names = %v, want %v", names, want)
	}
	for eventID, name := range want {
		if names[eventID] != name {
			t.Fatalf("name for %s = %q, want %q (all names %v)", eventID, names[eventID], name, names)
		}
	}
}
