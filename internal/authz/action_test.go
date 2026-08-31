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
	if scope := NormalizeAccessTokenScope(string(ActionGatewayDelete)); scope != string(ActionGatewayDelete) {
		t.Fatalf("gateway delete must be accepted as a personal access token scope, got %q", scope)
	}
	if !catalogContainsScope(AccessTokenScopeCatalog(PlatformRoleAdmin), string(ActionGatewayDelete)) {
		t.Fatal("gateway delete must be present in the personal access token scope catalog")
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
	if scope := NormalizeOAuthScope("project:write,deployment:exec,gateway:delete,volume:import,volume:export,volume:delete"); scope != "project:write,deployment:exec,gateway:delete,volume:import,volume:export,volume:delete" {
		t.Fatalf("normalized OAuth project scopes = %q", scope)
	}
	if scope := NormalizeOAuthScope("*"); scope != "" {
		t.Fatalf("OAuth full wildcard must be rejected, got %q", scope)
	}

	for _, scope := range []string{
		"project:write",
		"deployment:exec",
		"gateway:delete",
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
		ActionGatewayDelete,
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

func catalogScopeRequiresAdmin(catalog []AccessTokenScopeDefinition, scope string) bool {
	for _, item := range catalog {
		if item.Value == scope {
			return item.RequiresAdminRole
		}
	}
	return false
}
