package authz

import "testing"

func TestProjectRoleAllowsAction(t *testing.T) {
	if !ProjectRoleAllows(ProjectRoleDeveloper, ActionDeploymentRelease) {
		t.Fatal("expected developer to release deployments")
	}
	if ProjectRoleAllows(ProjectRoleViewer, ActionDeploymentRelease) {
		t.Fatal("expected viewer to be blocked from deployment release")
	}
	if !ProjectRoleAllows(ProjectRoleAdmin, ActionSecretViewValue) {
		t.Fatal("expected admin to view secret values")
	}
	if ProjectRoleAllows(ProjectRoleDeveloper, ActionSecretViewValue) {
		t.Fatal("expected developer to be blocked from secret values")
	}
	for _, role := range []string{ProjectRoleOwner, ProjectRoleAdmin} {
		if !ProjectRoleAllows(role, ActionDeploymentDataExport) {
			t.Fatalf("expected project %s to export deployment data", role)
		}
	}
	for _, role := range []string{ProjectRoleDeveloper, ProjectRoleViewer} {
		if ProjectRoleAllows(role, ActionDeploymentDataExport) {
			t.Fatalf("expected project %s to be blocked from deployment data export", role)
		}
	}
}

func TestProjectActionForLegacyRoles(t *testing.T) {
	action, ok := ProjectActionForLegacyRoles([]string{ProjectRoleDeveloper, ProjectRoleOwner, ProjectRoleAdmin})
	if !ok || action != ActionProjectWrite {
		t.Fatalf("legacy write roles mapped to %q, ok=%t", action, ok)
	}

	if !ProjectRoleAllowsLegacyRoles(ProjectRoleOwner, []string{ProjectRoleOwner}) {
		t.Fatal("expected owner-only legacy role check to allow owner")
	}
	if ProjectRoleAllowsLegacyRoles(ProjectRoleAdmin, []string{ProjectRoleOwner}) {
		t.Fatal("expected owner-only legacy role check to block admin")
	}
}

func TestAccessTokenScopeRules(t *testing.T) {
	if scope := NormalizeAccessTokenScope("deployment:exec,build:trigger"); scope != "deployment:exec,build:trigger" {
		t.Fatalf("normalized scope = %q", scope)
	}
	if scope := NormalizeAccessTokenScope("secret:read_summary,cluster:read"); scope != "secret:read_summary,cluster:read" {
		t.Fatalf("normalized sensitive scope = %q", scope)
	}
	if AccessTokenAllows("project:write", string(ActionDeploymentExec)) {
		t.Fatal("expected project:write to be too broad for deployment exec")
	}
	if !AccessTokenAllows("deployment:*", string(ActionDeploymentExec)) {
		t.Fatal("expected deployment wildcard to allow deployment exec")
	}
	if !AccessTokenAllows("secret:*", string(ActionSecretUpdate)) {
		t.Fatal("expected secret wildcard to allow secret update")
	}
	if UserCanCreateAccessTokenScope(PlatformRoleUser, "deployment:exec") {
		t.Fatal("expected regular user to be blocked from creating write scopes")
	}
	if !UserCanCreateAccessTokenScope(PlatformRoleUser, "build:trigger,deployment:release") {
		t.Fatal("expected regular user to create automation trigger scopes")
	}
	if !UserCanCreateAccessTokenScope(PlatformRoleUser, "project:read,build:read") {
		t.Fatal("expected regular user to create read scopes")
	}
	if scope := NormalizeAccessTokenScope(string(ActionDeploymentDataExport)); scope != "" {
		t.Fatalf("deployment data export must not be accepted as a personal access token scope, got %q", scope)
	}
}

func TestAccessTokenScopeCatalogMarksAdminOnlyScopes(t *testing.T) {
	userCatalog := AccessTokenScopeCatalog(PlatformRoleUser)
	adminCatalog := AccessTokenScopeCatalog(PlatformRoleAdmin)

	if !catalogScopeRequiresAdmin(userCatalog, string(ActionDeploymentExec)) {
		t.Fatal("expected deployment exec to require admin for regular users")
	}
	if catalogScopeRequiresAdmin(adminCatalog, string(ActionDeploymentExec)) {
		t.Fatal("expected deployment exec to be available for platform admins")
	}
	if catalogContainsScope(userCatalog, string(ActionDeploymentDataExport)) || catalogContainsScope(adminCatalog, string(ActionDeploymentDataExport)) {
		t.Fatal("deployment data export must not be exposed as a personal access token scope")
	}
	if catalogScopeRequiresAdmin(userCatalog, string(ActionBuildTrigger)) {
		t.Fatal("expected build trigger to be creatable by regular users")
	}
}

func catalogContainsScope(catalog []AccessTokenScopeDefinition, value string) bool {
	for _, scope := range catalog {
		if scope.Value == value {
			return true
		}
	}
	return false
}

func TestRequiredAccessTokenScopeUsesFineGrainedProjectRoutes(t *testing.T) {
	tests := []struct {
		path   string
		method string
		want   string
	}{
		{"/api/v1/runtime/clusters/:clusterId/resources", "DELETE", string(ActionClusterManage)},
		{"/api/v1/runtime/clusters/:clusterId/pods/terminal", "GET", string(ActionClusterManage)},
		{"/api/v1/runtime/clusters/:clusterId/pods/terminal/authorize", "POST", string(ActionClusterManage)},
		{"/api/v1/build/variable-sets", "POST", string(ActionSecretUpdate)},
		{"/api/v1/projects/:projectId/runtime-config-sets", "GET", string(ActionSecretReadSummary)},
		{"/api/v1/projects/:projectId/members", "POST", string(ActionProjectManage)},
		{"/api/v1/projects/:projectId/applications", "POST", string(ActionApplicationCreate)},
		{"/api/v1/projects/:projectId/applications/:applicationId/deployment-targets/:targetId/restart", "POST", string(ActionDeploymentRestart)},
		{"/api/v1/projects/:projectId/applications/:applicationId/deployment-targets/:targetId/data-export", "GET", string(ActionDeploymentDataExport)},
		{"/api/v1/projects/:projectId/applications/:applicationId/deployment-targets/:targetId/data-export/authorize", "POST", string(ActionDeploymentDataExport)},
		{"/api/v1/projects/:projectId/build-runs/trigger", "POST", string(ActionBuildTrigger)},
		{"/api/v1/projects/:projectId/build-runs/:runId/cancel", "POST", string(ActionBuildCancel)},
		{"/api/v1/projects/:projectId/releases", "POST", string(ActionDeploymentRelease)},
		{"/api/v1/projects/:projectId/releases/:releaseId/terminal/authorize", "POST", string(ActionDeploymentExec)},
		{"/api/v1/projects/:projectId/releases/:releaseId/rollback", "POST", string(ActionDeploymentRollback)},
		{"/api/v1/projects/:projectId/gateway-routes", "POST", string(ActionGatewayManage)},
		{"/api/v1/projects/:projectId/repository-bindings", "POST", string(ActionGitWrite)},
		{"/api/v1/events", "GET", string(ActionEventRead)},
		{"/api/v1/events/:eventId", "GET", string(ActionEventRead)},
	}

	for _, test := range tests {
		if got := RequiredAccessTokenScope(test.path, test.method); got != test.want {
			t.Fatalf("RequiredAccessTokenScope(%q, %q) = %q, want %q", test.path, test.method, got, test.want)
		}
	}
}

func catalogScopeRequiresAdmin(catalog []AccessTokenScopeDefinition, scope string) bool {
	for _, item := range catalog {
		if item.Value == scope {
			return item.RequiresAdminRole
		}
	}
	return false
}
