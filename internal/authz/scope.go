package authz

import "strings"

type AccessTokenScopeDefinition struct {
	Value             string `json:"value"`
	Group             string `json:"group"`
	Recommended       bool   `json:"recommended"`
	CreatableByUser   bool   `json:"creatableByUser"`
	RequiresAdminRole bool   `json:"requiresAdminRole"`
}

var accessTokenScopeCatalog = []AccessTokenScopeDefinition{
	scopeDef(ActionProjectRead, "project", true, true),
	scopeDef(ActionProjectWrite, "project", false, false),
	scopeDef(ActionProjectManage, "project", false, false),
	scopeDef(ActionProjectDelete, "project", false, false),

	scopeDef(ActionApplicationRead, "application", true, true),
	scopeDef(ActionApplicationCreate, "application", false, false),
	scopeDef(ActionApplicationUpdate, "application", false, false),
	scopeDef(ActionApplicationDelete, "application", false, false),

	scopeDef(ActionDeploymentRead, "deployment", true, true),
	scopeDef(ActionDeploymentUpdate, "deployment", false, false),
	scopeDef(ActionDeploymentRelease, "deployment", true, true),
	scopeDef(ActionDeploymentRestart, "deployment", false, false),
	scopeDef(ActionDeploymentRollback, "deployment", false, false),
	scopeDef(ActionDeploymentDelete, "deployment", false, false),
	scopeDef(ActionDeploymentExec, "deployment", false, false),

	scopeDef(ActionBuildRead, "build", true, true),
	scopeDef(ActionBuildTrigger, "build", true, true),
	scopeDef(ActionBuildCancel, "build", false, false),
	scopeDef(ActionBuildDelete, "build", false, false),

	scopeDef(ActionGatewayRead, "gateway", true, true),
	scopeDef(ActionGatewayManage, "gateway", false, false),

	scopeDef(ActionSecretReadSummary, "secret", true, true),
	scopeDef(ActionSecretViewValue, "secret", false, false),
	scopeDef(ActionSecretUpdate, "secret", false, false),

	scopeDef(ActionClusterRead, "cluster", true, true),
	scopeDef(ActionClusterUse, "cluster", false, false),
	scopeDef(ActionClusterManage, "cluster", false, false),

	scopeDef(ActionGitRead, "git", true, true),
	scopeDef(ActionGitWrite, "git", false, false),

	scopeDef(ActionRegistryRead, "registry", true, true),
	scopeDef(ActionRegistryWrite, "registry", false, false),

	scopeDef(ActionImageRead, "image", true, true),
	scopeDef(ActionImageWrite, "image", false, false),

	scopeDef(ActionBillingRead, "billing", true, true),
	scopeDef(ActionBillingAdjust, "billing", false, false),

	scopeDef(ActionUserRead, "user", true, true),
	scopeDef(ActionUserWrite, "user", false, false),
	scopeDef(ActionUserManage, "user", false, false),

	scopeDef(ActionConfigRead, "system", false, false),
	scopeDef(ActionConfigWrite, "system", false, false),
	scopeDef(ActionAuthManage, "system", false, false),
	scopeDef(ActionTokenManage, "system", false, false),
}

var allowedAccessTokenScopes = buildAllowedAccessTokenScopes()
var userCreatableAccessTokenScopes = buildUserCreatableAccessTokenScopes()

func AccessTokenScopeCatalog(userRole string) []AccessTokenScopeDefinition {
	output := make([]AccessTokenScopeDefinition, 0, len(accessTokenScopeCatalog))
	for _, scope := range accessTokenScopeCatalog {
		scope.RequiresAdminRole = !IsPlatformAdmin(userRole) && !scope.CreatableByUser
		output = append(output, scope)
	}
	return output
}

func scopeDef(action Action, group string, recommended, creatableByUser bool) AccessTokenScopeDefinition {
	return AccessTokenScopeDefinition{
		Value:           string(action),
		Group:           group,
		Recommended:     recommended,
		CreatableByUser: creatableByUser,
	}
}

func buildAllowedAccessTokenScopes() map[string]bool {
	scopes := make(map[string]bool, len(accessTokenScopeCatalog)+12)
	prefixes := map[string]bool{}
	for _, scope := range accessTokenScopeCatalog {
		scopes[scope.Value] = true
		prefix, _, found := strings.Cut(scope.Value, ":")
		if found {
			prefixes[prefix] = true
		}
	}
	for prefix := range prefixes {
		scopes[prefix+":*"] = true
	}
	return scopes
}

func buildUserCreatableAccessTokenScopes() map[string]bool {
	scopes := make(map[string]bool, len(accessTokenScopeCatalog))
	for _, scope := range accessTokenScopeCatalog {
		if scope.CreatableByUser {
			scopes[scope.Value] = true
		}
	}
	return scopes
}
