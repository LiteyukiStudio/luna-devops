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
	for _, role := range []string{ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper, ProjectRoleViewer} {
		if !ProjectRoleAllows(role, ActionVolumeRead) {
			t.Fatalf("expected project %s to read volumes", role)
		}
	}
	for _, role := range []string{ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper} {
		if !ProjectRoleAllows(role, ActionVolumeWrite) || !ProjectRoleAllows(role, ActionVolumeImport) {
			t.Fatalf("expected project %s to write and import volumes", role)
		}
	}
	for _, action := range []Action{ActionVolumeExport, ActionVolumeDelete} {
		if !ProjectRoleAllows(ProjectRoleOwner, action) || !ProjectRoleAllows(ProjectRoleAdmin, action) {
			t.Fatalf("expected owner and admin to use %s", action)
		}
		if ProjectRoleAllows(ProjectRoleDeveloper, action) || ProjectRoleAllows(ProjectRoleViewer, action) {
			t.Fatalf("expected developer and viewer to be blocked from %s", action)
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
	if !UserCanCreateAccessTokenScope(PlatformRoleUser, string(ActionDashboardRead)) {
		t.Fatal("expected regular user to create dashboard read scope")
	}
	if UserCanCreateAccessTokenScope(PlatformRoleUser, string(ActionDataRetentionRead)) {
		t.Fatal("expected regular user to be blocked from creating retention scopes")
	}
	for _, action := range []Action{ActionDashboardRead, ActionAgentObservabilityRead, ActionDataRetentionRead, ActionDataRetentionManage} {
		if !AccessTokenAllows("*", string(action)) {
			t.Fatalf("expected full scope token to allow %s", action)
		}
	}
	if scope := NormalizeAccessTokenScope(string(ActionVolumeRead)); scope != string(ActionVolumeRead) {
		t.Fatalf("volume read must be accepted as a personal access token scope, got %q", scope)
	}
	for _, action := range []Action{ActionVolumeImport, ActionVolumeExport, ActionVolumeDelete} {
		if scope := NormalizeAccessTokenScope(string(action)); scope != "" {
			t.Fatalf("high-risk volume scope %s must not be exposed to personal access tokens, got %q", action, scope)
		}
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
	if catalogScopeRequiresAdmin(userCatalog, string(ActionBuildTrigger)) {
		t.Fatal("expected build trigger to be creatable by regular users")
	}
	if catalogScopeRequiresAdmin(userCatalog, string(ActionDashboardRead)) {
		t.Fatal("expected dashboard read to be creatable by regular users")
	}
	for _, scope := range []Action{ActionAgentObservabilityRead, ActionDataRetentionRead, ActionDataRetentionManage} {
		if !catalogScopeRequiresAdmin(userCatalog, string(scope)) {
			t.Fatalf("expected %s to require a platform administrator", scope)
		}
		if catalogScopeRequiresAdmin(adminCatalog, string(scope)) {
			t.Fatalf("expected %s to be available to platform administrators", scope)
		}
	}
}

func TestOAuthScopeRules(t *testing.T) {
	if scope := NormalizeOAuthScope("project:write,deployment:exec,volume:import,volume:export,volume:delete"); scope != "project:write,deployment:exec,volume:import,volume:export,volume:delete" {
		t.Fatalf("normalized OAuth project scopes = %q", scope)
	}
	if scope := NormalizeOAuthScope("*"); scope != "" {
		t.Fatalf("OAuth full wildcard must be rejected, got %q", scope)
	}

	for _, scope := range []string{
		"project:write",
		"deployment:exec",
		"secret:update",
		"volume:import",
		"volume:export",
		"volume:delete",
	} {
		if !UserCanAuthorizeOAuthScope(PlatformRoleUser, scope) {
			t.Fatalf("expected regular user to authorize project-scoped OAuth scope %q", scope)
		}
	}
	for _, scope := range []string{
		string(ActionConfigWrite),
		string(ActionUserManage),
		string(ActionAgentObservabilityRead),
		string(ActionDataRetentionManage),
	} {
		if UserCanAuthorizeOAuthScope(PlatformRoleUser, scope) {
			t.Fatalf("expected regular user to be blocked from platform OAuth scope %q", scope)
		}
		if !UserCanAuthorizeOAuthScope(PlatformRoleAdmin, scope) {
			t.Fatalf("expected platform administrator to authorize OAuth scope %q", scope)
		}
	}
}

func TestRecommendedOAuthScopesExcludeHighRiskOperations(t *testing.T) {
	scopes := RecommendedOAuthScopes(PlatformRoleUser)
	for _, scope := range []Action{
		ActionProjectRead,
		ActionBuildTrigger,
		ActionDeploymentRelease,
	} {
		if !contains(scopes, string(scope)) {
			t.Fatalf("expected recommended OAuth scopes to include %q", scope)
		}
	}
	if contains(scopes, string(ActionAgentObservabilityRead)) {
		t.Fatal("regular users must not receive the cross-user Agent observability scope")
	}
	if !contains(RecommendedOAuthScopes(PlatformRoleAdmin), string(ActionAgentObservabilityRead)) {
		t.Fatal("platform administrators need Agent observability in the default CLI OAuth grant")
	}
	for _, scope := range []Action{
		ActionDeploymentExec,
		ActionSecretUpdate,
		ActionConfigWrite,
		ActionVolumeImport,
		ActionVolumeExport,
		ActionVolumeDelete,
	} {
		if contains(scopes, string(scope)) {
			t.Fatalf("expected recommended OAuth scopes to exclude high-risk scope %q", scope)
		}
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
		{"/api/v1/build/templates/node/preview", "POST", string(ActionBuildRead)},
		{"/api/v1/registries/:registryId/test", "POST", string(ActionRegistryRead)},
		{"/api/v1/projects/:projectId/service-bindings/:bindingId/check", "POST", string(ActionProjectRead)},
		{"/api/v1/runtime/clusters/:clusterId/resources", "GET", string(ActionClusterRead)},
		{"/api/v1/runtime/clusters/pressure", "GET", string(ActionClusterRead)},
		{"/api/v1/runtime/clusters/:clusterId/resource-yaml", "GET", string(ActionClusterRead)},
		{"/api/v1/runtime/clusters/:clusterId/resource-events", "GET", string(ActionClusterRead)},
		{"/api/v1/runtime/clusters/:clusterId/test", "POST", string(ActionClusterUse)},
		{"/api/v1/runtime/clusters/:clusterId/resources", "DELETE", string(ActionClusterManage)},
		{"/api/v1/runtime/clusters/:clusterId/pods/terminal", "GET", string(ActionClusterManage)},
		{"/api/v1/runtime/clusters/:clusterId/pods/terminal/authorize", "POST", string(ActionClusterManage)},
		{"/api/v1/build/variable-sets", "POST", string(ActionSecretUpdate)},
		{"/api/v1/projects/:projectId/runtime-config-sets", "GET", string(ActionSecretReadSummary)},
		{"/api/v1/projects/:projectId/members", "POST", string(ActionProjectManage)},
		{"/api/v1/projects/:projectId/applications", "POST", string(ActionApplicationCreate)},
		{"/api/v1/projects/:projectId/applications/:applicationId/deployment-targets/:targetId/restart", "POST", string(ActionDeploymentRestart)},
		{"/api/v1/projects/:projectId/applications/:applicationId/deployment-target-imports/preview", "POST", string(ActionDeploymentUpdate)},
		{"/api/v1/projects/:projectId/applications/:applicationId/deployment-target-imports", "POST", string(ActionDeploymentUpdate)},
		{"/api/v1/projects/:projectId/build-runs/trigger", "POST", string(ActionBuildTrigger)},
		{"/api/v1/projects/:projectId/build-runs/:runId/cancel", "POST", string(ActionBuildCancel)},
		{"/api/v1/projects/:projectId/releases", "POST", string(ActionDeploymentRelease)},
		{"/api/v1/projects/:projectId/releases/:releaseId/terminal/authorize", "POST", string(ActionDeploymentExec)},
		{"/api/v1/projects/:projectId/releases/:releaseId/rollback", "POST", string(ActionDeploymentRollback)},
		{"/api/v1/projects/:projectId/gateway-routes", "POST", string(ActionGatewayManage)},
		{"/api/v1/projects/:projectId/repository-bindings", "POST", string(ActionGitWrite)},
		{"/api/v1/projects/:projectId/volumes", "GET", string(ActionVolumeRead)},
		{"/api/v1/projects/:projectId/volumes", "POST", string(ActionVolumeWrite)},
		{"/api/v1/projects/:projectId/volumes/:volumeId", "PATCH", string(ActionVolumeWrite)},
		{"/api/v1/projects/:projectId/volumes/:volumeId/retry", "POST", string(ActionVolumeRead)},
		{"/api/v1/projects/:projectId/volumes/:volumeId", "DELETE", string(ActionVolumeDelete)},
		{"/api/v1/projects/:projectId/volumes/:volumeId/deletion-preview", "POST", string(ActionVolumeDelete)},
		{"/api/v1/projects/:projectId/volumes/:volumeId/exports", "POST", string(ActionVolumeExport)},
		{"/api/v1/projects/:projectId/volume-storage-classes", "GET", string(ActionVolumeRead)},
		{"/api/v1/projects/:projectId/volume-imports", "POST", string(ActionVolumeImport)},
		{"/api/v1/projects/:projectId/volume-imports/:transferId/content", "PATCH", string(ActionVolumeImport)},
		{"/api/v1/projects/:projectId/volume-transfers", "GET", string(ActionVolumeRead)},
		{"/api/v1/projects/:projectId/volume-transfers/:transferId/retry", "POST", string(ActionVolumeRead)},
		{"/api/v1/projects/:projectId/volume-transfers/:transferId/cancel", "POST", string(ActionVolumeRead)},
		{"/api/v1/projects/:projectId/volume-transfers/:transferId/download-authorizations", "POST", string(ActionVolumeExport)},
		{"/api/v1/projects/:projectId/volume-transfers/:transferId/content", "GET", string(ActionVolumeExport)},
		{"/api/v1/projects/:projectId/volume-transfers/:transferId/manifest", "GET", string(ActionVolumeExport)},
		{"/api/v1/events", "GET", string(ActionEventRead)},
		{"/api/v1/events/:eventId", "GET", string(ActionEventRead)},
		{"/api/v1/dashboard", "GET", string(ActionDashboardRead)},
		{"/api/v1/ai/observability/overview", "GET", string(ActionAgentObservabilityRead)},
		{"/api/v1/ai/observability/tools/listProjectVolumes/calls", "GET", string(ActionAgentObservabilityRead)},
		{"/api/v1/configs/ai/observability/test", "POST", string(ActionAgentObservabilityRead)},
		{"/api/v1/data-retention/catalog", "GET", string(ActionDataRetentionRead)},
		{"/api/v1/data-retention/preview", "POST", string(ActionDataRetentionRead)},
		{"/api/v1/data-retention/cleanup", "POST", string(ActionDataRetentionManage)},
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
