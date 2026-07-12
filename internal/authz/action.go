package authz

import (
	"net/http"
	"sort"
	"strings"
)

type Action string

const (
	ActionSystemUnmapped Action = "system:unmapped"

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

	ActionApplicationRead   Action = "application:read"
	ActionApplicationCreate Action = "application:create"
	ActionApplicationUpdate Action = "application:update"
	ActionApplicationDelete Action = "application:delete"

	ActionDeploymentRead       Action = "deployment:read"
	ActionDeploymentUpdate     Action = "deployment:update"
	ActionDeploymentRelease    Action = "deployment:release"
	ActionDeploymentRestart    Action = "deployment:restart"
	ActionDeploymentRollback   Action = "deployment:rollback"
	ActionDeploymentDelete     Action = "deployment:delete"
	ActionDeploymentExec       Action = "deployment:exec"
	ActionDeploymentDataExport Action = "deployment:data_export"

	ActionBuildRead    Action = "build:read"
	ActionBuildTrigger Action = "build:trigger"
	ActionBuildCancel  Action = "build:cancel"
	ActionBuildDelete  Action = "build:delete"

	ActionGatewayRead   Action = "gateway:read"
	ActionGatewayManage Action = "gateway:manage"

	ActionSecretReadSummary Action = "secret:read_summary"
	ActionSecretViewValue   Action = "secret:view_value"
	ActionSecretUpdate      Action = "secret:update"

	ActionClusterRead   Action = "cluster:read"
	ActionClusterUse    Action = "cluster:use"
	ActionClusterManage Action = "cluster:manage"

	ActionBillingRead   Action = "billing:read"
	ActionBillingAdjust Action = "billing:write"
	ActionEventRead     Action = "event:read"

	ActionGitRead  Action = "git:read"
	ActionGitWrite Action = "git:write"

	ActionRegistryRead  Action = "registry:read"
	ActionRegistryWrite Action = "registry:write"

	ActionImageRead  Action = "image:read"
	ActionImageWrite Action = "image:write"

	ActionTokenManage Action = "token:manage"
)

const (
	PlatformRoleAdmin = "platform_admin"
	PlatformRoleUser  = "user"

	ProjectRoleOwner     = "owner"
	ProjectRoleAdmin     = "admin"
	ProjectRoleDeveloper = "developer"
	ProjectRoleViewer    = "viewer"
)

var projectActionRoles = map[Action][]string{
	ActionProjectRead:   {ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper, ProjectRoleViewer},
	ActionProjectWrite:  {ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper},
	ActionProjectManage: {ProjectRoleOwner, ProjectRoleAdmin},
	ActionProjectDelete: {ProjectRoleOwner},

	ActionApplicationRead:   {ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper, ProjectRoleViewer},
	ActionApplicationCreate: {ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper},
	ActionApplicationUpdate: {ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper},
	ActionApplicationDelete: {ProjectRoleOwner, ProjectRoleAdmin},

	ActionDeploymentRead:       {ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper, ProjectRoleViewer},
	ActionDeploymentUpdate:     {ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper},
	ActionDeploymentRelease:    {ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper},
	ActionDeploymentRestart:    {ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper},
	ActionDeploymentRollback:   {ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper},
	ActionDeploymentDelete:     {ProjectRoleOwner, ProjectRoleAdmin},
	ActionDeploymentExec:       {ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper},
	ActionDeploymentDataExport: {ProjectRoleOwner, ProjectRoleAdmin},

	ActionBuildRead:    {ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper, ProjectRoleViewer},
	ActionBuildTrigger: {ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper},
	ActionBuildCancel:  {ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper},
	ActionBuildDelete:  {ProjectRoleOwner, ProjectRoleAdmin},

	ActionGatewayRead:   {ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper, ProjectRoleViewer},
	ActionGatewayManage: {ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper},

	ActionSecretReadSummary: {ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper, ProjectRoleViewer},
	ActionSecretViewValue:   {ProjectRoleOwner, ProjectRoleAdmin},
	ActionSecretUpdate:      {ProjectRoleOwner, ProjectRoleAdmin},

	ActionClusterRead:   {ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper, ProjectRoleViewer},
	ActionClusterUse:    {ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper},
	ActionClusterManage: {ProjectRoleOwner, ProjectRoleAdmin},

	ActionBillingRead:   {ProjectRoleOwner, ProjectRoleAdmin},
	ActionBillingAdjust: {ProjectRoleOwner},
}

func IsPlatformAdmin(role string) bool {
	return role == PlatformRoleAdmin
}

func NormalizeProjectRole(role string) string {
	switch role {
	case ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper, ProjectRoleViewer:
		return role
	default:
		return ProjectRoleViewer
	}
}

func ProjectRoleAllows(role string, action Action) bool {
	if action == "" {
		return false
	}
	for _, allowedRole := range projectActionRoles[action] {
		if role == allowedRole {
			return true
		}
	}
	return false
}

func ProjectActionForLegacyRoles(roles []string) (Action, bool) {
	key := normalizedRoleSetKey(roles)
	switch key {
	case normalizedRoleSetKey([]string{ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper, ProjectRoleViewer}):
		return ActionProjectRead, true
	case normalizedRoleSetKey([]string{ProjectRoleOwner, ProjectRoleAdmin, ProjectRoleDeveloper}):
		return ActionProjectWrite, true
	case normalizedRoleSetKey([]string{ProjectRoleOwner, ProjectRoleAdmin}):
		return ActionProjectManage, true
	case normalizedRoleSetKey([]string{ProjectRoleOwner}):
		return ActionProjectDelete, true
	default:
		return "", false
	}
}

func ProjectRoleAllowsLegacyRoles(role string, roles []string) bool {
	if action, ok := ProjectActionForLegacyRoles(roles); ok {
		return ProjectRoleAllows(role, action)
	}
	for _, allowedRole := range roles {
		if role == allowedRole {
			return true
		}
	}
	return false
}

func RequiredAccessTokenScope(path, method string) string {
	switch {
	case path == "/api/v1/users/me" && method == http.MethodGet:
		return string(ActionUserRead)
	case path == "/api/v1/users/me" && method != http.MethodGet:
		return string(ActionUserWrite)
	case strings.HasPrefix(path, "/api/v1/users"):
		return string(ActionUserManage)
	case strings.HasPrefix(path, "/api/v1/auth"):
		return string(ActionAuthManage)
	case strings.HasPrefix(path, "/api/v1/configs") && method == http.MethodGet:
		return string(ActionConfigRead)
	case strings.HasPrefix(path, "/api/v1/configs") && method != http.MethodGet:
		return string(ActionConfigWrite)
	case isRuntimeClusterPodTerminalPath(path):
		return string(ActionClusterManage)
	case strings.HasPrefix(path, "/api/v1/runtime/clusters") && method == http.MethodGet:
		return string(ActionClusterRead)
	case strings.HasPrefix(path, "/api/v1/runtime/clusters") && method != http.MethodGet:
		return runtimeClusterWriteScope(path, method)
	case strings.HasPrefix(path, "/api/v1/build/variable-sets") && method == http.MethodGet:
		return string(ActionSecretReadSummary)
	case strings.HasPrefix(path, "/api/v1/build/variable-sets") && method != http.MethodGet:
		return string(ActionSecretUpdate)
	case isReleaseRuntimeExecPath(path):
		return string(ActionDeploymentExec)
	case isProjectRuntimeConfigPath(path) && method == http.MethodGet:
		return string(ActionSecretReadSummary)
	case isProjectRuntimeConfigPath(path) && method != http.MethodGet:
		return string(ActionSecretUpdate)
	case isProjectMemberPath(path) && method == http.MethodGet:
		return string(ActionProjectRead)
	case isProjectMemberPath(path) && method != http.MethodGet:
		return string(ActionProjectManage)
	case isProjectApplicationPath(path) && method == http.MethodGet:
		return string(ActionApplicationRead)
	case isProjectApplicationPath(path) && method == http.MethodPost:
		return string(ActionApplicationCreate)
	case isProjectApplicationPath(path) && method == http.MethodDelete:
		return string(ActionApplicationDelete)
	case isProjectApplicationPath(path) && method != http.MethodGet:
		return string(ActionApplicationUpdate)
	case isDeploymentTargetPath(path):
		return deploymentTargetScope(path, method)
	case isProjectBuildPath(path):
		return projectBuildScope(path, method)
	case isProjectReleasePath(path):
		return projectReleaseScope(path, method)
	case isProjectGatewayRoutePath(path) && method == http.MethodGet:
		return string(ActionGatewayRead)
	case isProjectGatewayRoutePath(path) && method != http.MethodGet:
		return string(ActionGatewayManage)
	case isProjectRepositoryBindingPath(path) && method == http.MethodGet:
		return string(ActionGitRead)
	case isProjectRepositoryBindingPath(path) && method != http.MethodGet:
		return string(ActionGitWrite)
	case strings.HasPrefix(path, "/api/v1/projects") && method == http.MethodGet:
		return string(ActionProjectRead)
	case strings.HasPrefix(path, "/api/v1/projects") && method != http.MethodGet:
		return string(ActionProjectWrite)
	case strings.HasPrefix(path, "/api/v1/access-tokens"):
		return string(ActionTokenManage)
	case strings.HasPrefix(path, "/api/v1/billing") && method == http.MethodGet:
		return string(ActionBillingRead)
	case strings.HasPrefix(path, "/api/v1/billing") && method != http.MethodGet:
		return string(ActionBillingAdjust)
	case strings.HasPrefix(path, "/api/v1/events") && method == http.MethodGet:
		return string(ActionEventRead)
	case strings.HasPrefix(path, "/api/v1/git") && method == http.MethodGet:
		return string(ActionGitRead)
	case strings.HasPrefix(path, "/api/v1/git") && method != http.MethodGet:
		return string(ActionGitWrite)
	case strings.HasPrefix(path, "/api/v1/registries") && method == http.MethodGet:
		return string(ActionRegistryRead)
	case strings.HasPrefix(path, "/api/v1/registries") && method != http.MethodGet:
		return string(ActionRegistryWrite)
	case strings.HasPrefix(path, "/api/v1/container-images") && method == http.MethodGet:
		return string(ActionImageRead)
	case strings.HasPrefix(path, "/api/v1/container-images") && method != http.MethodGet:
		return string(ActionImageWrite)
	default:
		return string(ActionSystemUnmapped)
	}
}

func runtimeClusterWriteScope(path, method string) string {
	if method == http.MethodDelete && strings.HasSuffix(path, "/resources") {
		return string(ActionClusterManage)
	}
	if strings.HasSuffix(path, "/test") {
		return string(ActionClusterUse)
	}
	return string(ActionClusterManage)
}

func deploymentTargetScope(path, method string) string {
	switch {
	case strings.HasSuffix(path, "/restart"):
		return string(ActionDeploymentRestart)
	case strings.HasSuffix(path, "/data-export") || strings.HasSuffix(path, "/data-export/authorize"):
		return string(ActionDeploymentDataExport)
	case strings.Contains(path, "/metrics/stream"):
		return string(ActionDeploymentRead)
	case method == http.MethodGet:
		return string(ActionDeploymentRead)
	case method == http.MethodDelete:
		return string(ActionDeploymentDelete)
	default:
		return string(ActionDeploymentUpdate)
	}
}

func projectBuildScope(path, method string) string {
	switch {
	case strings.HasSuffix(path, "/trigger") || strings.HasSuffix(path, "/retry"):
		return string(ActionBuildTrigger)
	case strings.HasSuffix(path, "/cancel"):
		return string(ActionBuildCancel)
	case method == http.MethodDelete:
		return string(ActionBuildDelete)
	default:
		return string(ActionBuildRead)
	}
}

func projectReleaseScope(path, method string) string {
	switch {
	case strings.HasSuffix(path, "/rollback"):
		return string(ActionDeploymentRollback)
	case isReleaseRuntimeExecPath(path):
		return string(ActionDeploymentExec)
	case method == http.MethodPost:
		return string(ActionDeploymentRelease)
	default:
		return string(ActionDeploymentRead)
	}
}

func AccessTokenAllows(scopeText, required string) bool {
	if required == "" {
		return true
	}
	if required == string(ActionSystemUnmapped) {
		return false
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

func isReleaseRuntimeExecPath(path string) bool {
	switch path {
	case "/api/v1/projects/:projectId/releases/:releaseId/exec",
		"/api/v1/projects/:projectId/releases/:releaseId/terminal",
		"/api/v1/projects/:projectId/releases/:releaseId/terminal/authorize":
		return true
	default:
		return false
	}
}

func isRuntimeClusterPodTerminalPath(path string) bool {
	return path == "/api/v1/runtime/clusters/:clusterId/pods/terminal"
}

func isProjectRuntimeConfigPath(path string) bool {
	return strings.Contains(path, "/runtime-config-sets")
}

func isProjectMemberPath(path string) bool {
	return strings.Contains(path, "/members") || strings.Contains(path, "/member-candidates")
}

func isProjectApplicationPath(path string) bool {
	return strings.Contains(path, "/applications") && !strings.Contains(path, "/deployment-targets")
}

func isDeploymentTargetPath(path string) bool {
	return strings.Contains(path, "/deployment-targets")
}

func isProjectBuildPath(path string) bool {
	return strings.Contains(path, "/build-runs") || strings.Contains(path, "/build-jobs")
}

func isProjectReleasePath(path string) bool {
	return strings.Contains(path, "/releases")
}

func isProjectGatewayRoutePath(path string) bool {
	return strings.Contains(path, "/gateway-routes")
}

func isProjectRepositoryBindingPath(path string) bool {
	return strings.Contains(path, "/repository-bindings")
}

func normalizedRoleSetKey(roles []string) string {
	normalized := normalizeList(roles)
	sort.Strings(normalized)
	return strings.Join(normalized, ",")
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
