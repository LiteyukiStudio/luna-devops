package authz

import "strings"

type Action string

const (
	ActionUserRead   Action = "user:read"
	ActionUserWrite  Action = "user:write"
	ActionUserManage Action = "user:manage"

	ActionAuthManage Action = "auth:manage"

	ActionConfigRead  Action = "config:read"
	ActionConfigWrite Action = "config:write"

	ActionProjectRead   Action = "project:read"
	ActionProjectWrite  Action = "project:write"
	ActionProjectManage Action = "project:manage"
	ActionProjectDelete Action = "project:delete"
	// ActionProjectOwnerOnly requires an Owner membership from non-platform
	// administrators and is also used as an internal membership sub-rule.
	ActionProjectOwnerOnly Action = "project:owner_only"
	// ActionProjectPin lets every member maintain their own dashboard preference.
	ActionProjectPin Action = "project:pin"

	ActionApplicationRead   Action = "application:read"
	ActionApplicationCreate Action = "application:create"
	ActionApplicationUpdate Action = "application:update"
	ActionApplicationDelete Action = "application:delete"

	ActionDeploymentRead     Action = "deployment:read"
	ActionDeploymentUpdate   Action = "deployment:update"
	ActionDeploymentRelease  Action = "deployment:release"
	ActionDeploymentRestart  Action = "deployment:restart"
	ActionDeploymentRollback Action = "deployment:rollback"
	ActionDeploymentDelete   Action = "deployment:delete"
	ActionDeploymentExec     Action = "deployment:exec"

	ActionBuildRead    Action = "build:read"
	ActionBuildTrigger Action = "build:trigger"
	ActionBuildCancel  Action = "build:cancel"
	ActionBuildDelete  Action = "build:delete"

	ActionGatewayRead   Action = "gateway:read"
	ActionGatewayManage Action = "gateway:manage"
	// ActionGatewayDelete keeps the restrictive Owner/Admin rule explicit.
	ActionGatewayDelete Action = "gateway:delete"

	ActionSecretReadSummary Action = "secret:read_summary"
	ActionSecretViewValue   Action = "secret:view_value"
	ActionSecretUpdate      Action = "secret:update"

	ActionClusterRead   Action = "cluster:read"
	ActionClusterUse    Action = "cluster:use"
	ActionClusterManage Action = "cluster:manage"

	ActionBillingRead   Action = "billing:read"
	ActionBillingAdjust Action = "billing:write"
	ActionEventRead     Action = "event:read"

	ActionDashboardRead          Action = "dashboard:read"
	ActionAgentObservabilityRead Action = "agent-observability:read"
	ActionDataRetentionRead      Action = "retention:read"
	ActionDataRetentionManage    Action = "retention:manage"

	ActionGitRead  Action = "git:read"
	ActionGitWrite Action = "git:write"

	ActionRegistryRead  Action = "registry:read"
	ActionRegistryWrite Action = "registry:write"
	// ActionRegistryUse requires a role that may attach a registry-derived image
	// to project delivery configuration.
	ActionRegistryUse Action = "registry:use"

	ActionImageRead  Action = "image:read"
	ActionImageWrite Action = "image:write"

	ActionVolumeRead   Action = "volume:read"
	ActionVolumeWrite  Action = "volume:write"
	ActionVolumeImport Action = "volume:import"
	ActionVolumeExport Action = "volume:export"
	ActionVolumeDelete Action = "volume:delete"

	ActionTokenManage Action = "token:manage"
)

func AccessTokenAllows(scopeText, required string) bool {
	if required == "" {
		return true
	}
	scopes := splitCSV(strings.ReplaceAll(scopeText, " ", ","))
	if contains(scopes, "*") || contains(scopes, required) {
		return true
	}
	requiredPrefix, _, _ := strings.Cut(required, ":")
	return contains(scopes, requiredPrefix+":*")
}

func NormalizeAccessTokenScope(scopeText string) string {
	scopes := normalizeList(strings.Split(strings.ReplaceAll(scopeText, " ", ","), ","))
	if len(scopes) == 0 {
		return string(ActionProjectRead)
	}
	for _, scope := range scopes {
		if scope == "*" || !allowedAccessTokenScopes[scope] {
			return ""
		}
	}
	return strings.Join(scopes, ",")
}

func NormalizeOAuthScope(scopeText string) string {
	scopes := normalizeList(strings.Split(strings.ReplaceAll(scopeText, " ", ","), ","))
	if len(scopes) == 0 {
		return ""
	}
	for _, scope := range scopes {
		if scope == "*" || !allowedOAuthScopes[scope] {
			return ""
		}
	}
	return strings.Join(scopes, ",")
}

func UserCanCreateAccessTokenScope(userRole, scopeText string) bool {
	if IsPlatformAdmin(userRole) {
		return true
	}
	for _, scope := range splitCSV(scopeText) {
		if !userCreatableAccessTokenScopes[scope] {
			return false
		}
	}
	return true
}

func UserCanAuthorizeOAuthScope(userRole, scopeText string) bool {
	if IsPlatformAdmin(userRole) {
		return true
	}
	for _, scope := range splitCSV(scopeText) {
		if !userAuthorizableOAuthScopes[scope] {
			return false
		}
	}
	return true
}

func normalizeList(values []string) []string {
	seen := map[string]bool{}
	output := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		output = append(output, value)
	}
	return output
}

func splitCSV(value string) []string {
	return normalizeList(strings.Split(value, ","))
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
