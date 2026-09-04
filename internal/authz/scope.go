package authz

import "strings"

type AccessTokenScopeDefinition struct {
	Value             string `json:"value"`
	Group             string `json:"group"`
	Recommended       bool   `json:"recommended"`
	CreatableByUser   bool   `json:"creatableByUser"`
	RequiresAdminRole bool   `json:"requiresAdminRole"`
}

type scopeDefinition struct {
	AccessTokenScopeDefinition
	AccessTokenAllowed      bool
	OAuthAuthorizableByUser bool
}

var scopeCatalog = []scopeDefinition{
	scopeDef(ActionProjectRead, "project", true, true, true),
	scopeDef(ActionProjectWrite, "project", false, false, true),
	scopeDef(ActionProjectManage, "project", false, false, true),
	scopeDef(ActionProjectDelete, "project", false, false, true),

	scopeDef(ActionApplicationRead, "application", true, true, true),
	scopeDef(ActionApplicationCreate, "application", false, false, true),
	scopeDef(ActionApplicationUpdate, "application", false, false, true),
	scopeDef(ActionApplicationDelete, "application", false, false, true),

	scopeDef(ActionDeploymentRead, "deployment", true, true, true),
	scopeDef(ActionDeploymentUpdate, "deployment", false, false, true),
	scopeDef(ActionDeploymentRelease, "deployment", true, true, true),
	scopeDef(ActionDeploymentRestart, "deployment", false, false, true),
	scopeDef(ActionDeploymentRollback, "deployment", false, false, true),
	scopeDef(ActionDeploymentDelete, "deployment", false, false, true),
	scopeDef(ActionDeploymentExec, "deployment", false, false, true),

	scopeDef(ActionBuildRead, "build", true, true, true),
	scopeDef(ActionBuildTrigger, "build", true, true, true),
	scopeDef(ActionBuildCancel, "build", false, false, true),
	scopeDef(ActionBuildDelete, "build", false, false, true),

	scopeDef(ActionGatewayRead, "gateway", true, true, true),
	scopeDef(ActionGatewayManage, "gateway", false, false, true),
	scopeDef(ActionGatewayDelete, "gateway", false, false, true),

	scopeDef(ActionSecretReadSummary, "secret", true, true, true),
	scopeDef(ActionSecretViewValue, "secret", false, false, true),
	scopeDef(ActionSecretUpdate, "secret", false, false, true),

	scopeDef(ActionClusterRead, "cluster", true, true, true),
	scopeDef(ActionClusterUse, "cluster", false, false, true),
	scopeDef(ActionClusterManage, "cluster", false, false, true),

	scopeDef(ActionGitRead, "git", true, true, true),
	scopeDef(ActionGitWrite, "git", false, false, true),

	scopeDef(ActionRegistryRead, "registry", true, true, true),
	scopeDef(ActionRegistryWrite, "registry", false, false, true),

	scopeDef(ActionImageRead, "image", true, true, true),
	scopeDef(ActionImageWrite, "image", false, false, true),

	scopeDef(ActionVolumeRead, "volume", true, true, true),
	scopeDef(ActionVolumeWrite, "volume", false, false, true),
	oauthOnlyScopeDef(ActionVolumeImport, "volume", true),
	oauthOnlyScopeDef(ActionVolumeExport, "volume", true),
	oauthOnlyScopeDef(ActionVolumeDelete, "volume", true),

	scopeDef(ActionBillingRead, "billing", true, true, true),
	scopeDef(ActionBillingAdjust, "billing", false, false, true),
	scopeDef(ActionEventRead, "event", true, true, true),

	scopeDef(ActionDashboardRead, "dashboard", true, true, true),
	scopeDef(ActionAgentObservabilityRead, "agent-observability", true, false, false),
	scopeDef(ActionDataRetentionRead, "retention", false, false, false),
	scopeDef(ActionDataRetentionManage, "retention", false, false, false),

	scopeDef(ActionUserRead, "user", true, true, true),
	scopeDef(ActionUserWrite, "user", false, false, true),
	scopeDef(ActionUserManage, "user", false, false, false),

	scopeDef(ActionConfigRead, "system", false, false, false),
	scopeDef(ActionConfigWrite, "system", false, false, false),
	scopeDef(ActionAuthManage, "system", false, false, false),
	scopeDef(ActionTokenManage, "system", false, false, true),
}

var allowedAccessTokenScopes = buildAllowedAccessTokenScopes()
var userCreatableAccessTokenScopes = buildUserCreatableAccessTokenScopes()
var allowedOAuthScopes = buildAllowedOAuthScopes()
var userAuthorizableOAuthScopes = buildUserAuthorizableOAuthScopes()

func AccessTokenScopeCatalog(userRole string) []AccessTokenScopeDefinition {
	output := make([]AccessTokenScopeDefinition, 0, len(scopeCatalog))
	for _, definition := range scopeCatalog {
		if !definition.AccessTokenAllowed {
			continue
		}
		scope := definition.AccessTokenScopeDefinition
		scope.RequiresAdminRole = !IsPlatformAdmin(userRole) && !scope.CreatableByUser
		output = append(output, scope)
	}
	return output
}

func scopeDef(action Action, group string, recommended, creatableByUser, oauthAuthorizableByUser bool) scopeDefinition {
	return scopeDefinition{
		AccessTokenScopeDefinition: AccessTokenScopeDefinition{
			Value:           string(action),
			Group:           group,
			Recommended:     recommended,
			CreatableByUser: creatableByUser,
		},
		AccessTokenAllowed:      true,
		OAuthAuthorizableByUser: oauthAuthorizableByUser,
	}
}

func oauthOnlyScopeDef(action Action, group string, authorizableByUser bool) scopeDefinition {
	return scopeDefinition{
		AccessTokenScopeDefinition: AccessTokenScopeDefinition{
			Value: string(action),
			Group: group,
		},
		OAuthAuthorizableByUser: authorizableByUser,
	}
}

func buildAllowedAccessTokenScopes() map[string]bool {
	scopes := make(map[string]bool, len(scopeCatalog)+12)
	prefixes := map[string]bool{}
	for _, scope := range scopeCatalog {
		if !scope.AccessTokenAllowed {
			continue
		}
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
	scopes := make(map[string]bool, len(scopeCatalog))
	for _, scope := range scopeCatalog {
		if scope.AccessTokenAllowed && scope.CreatableByUser {
			scopes[scope.Value] = true
		}
	}
	return scopes
}

func buildAllowedOAuthScopes() map[string]bool {
	scopes := make(map[string]bool, len(scopeCatalog))
	for _, scope := range scopeCatalog {
		scopes[scope.Value] = true
	}
	return scopes
}

func buildUserAuthorizableOAuthScopes() map[string]bool {
	scopes := make(map[string]bool, len(scopeCatalog))
	for _, scope := range scopeCatalog {
		if scope.OAuthAuthorizableByUser {
			scopes[scope.Value] = true
		}
	}
	return scopes
}
