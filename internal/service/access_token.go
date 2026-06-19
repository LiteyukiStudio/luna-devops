package service

import (
	"net/http"
	"strings"
)

func RequiredAccessTokenScope(path, method string) string {
	switch {
	case path == "/api/v1/users/me" && method == http.MethodGet:
		return "user:read"
	case path == "/api/v1/users/me" && method != http.MethodGet:
		return "user:write"
	case strings.HasPrefix(path, "/api/v1/users"):
		return "user:manage"
	case strings.HasPrefix(path, "/api/v1/auth"):
		return "auth:manage"
	case strings.HasPrefix(path, "/api/v1/configs") && method == http.MethodGet:
		return "config:read"
	case strings.HasPrefix(path, "/api/v1/configs") && method != http.MethodGet:
		return "config:write"
	case strings.HasPrefix(path, "/api/v1/projects") && method == http.MethodGet:
		return "project:read"
	case strings.HasPrefix(path, "/api/v1/projects") && method != http.MethodGet:
		return "project:write"
	case strings.HasPrefix(path, "/api/v1/access-tokens"):
		return "token:manage"
	case strings.HasPrefix(path, "/api/v1/billing") && method == http.MethodGet:
		return "billing:read"
	case strings.HasPrefix(path, "/api/v1/billing") && method != http.MethodGet:
		return "billing:write"
	case strings.HasPrefix(path, "/api/v1/git") && method == http.MethodGet:
		return "git:read"
	case strings.HasPrefix(path, "/api/v1/git") && method != http.MethodGet:
		return "git:write"
	case strings.HasPrefix(path, "/api/v1/registries") && method == http.MethodGet:
		return "registry:read"
	case strings.HasPrefix(path, "/api/v1/registries") && method != http.MethodGet:
		return "registry:write"
	case strings.HasPrefix(path, "/api/v1/container-images") && method == http.MethodGet:
		return "image:read"
	case strings.HasPrefix(path, "/api/v1/container-images") && method != http.MethodGet:
		return "image:write"
	default:
		return "system:unmapped"
	}
}

func AccessTokenAllows(scopeText, required string) bool {
	if required == "" {
		return true
	}
	if required == "system:unmapped" {
		return false
	}
	scopes := splitCSV(strings.ReplaceAll(scopeText, " ", ","))
	if containsString(scopes, "*") || containsString(scopes, required) {
		return true
	}
	requiredPrefix, _, _ := strings.Cut(required, ":")
	return containsString(scopes, requiredPrefix+":*")
}

func NormalizeAccessTokenScope(scopeText string) string {
	scopes := normalizeList(strings.Split(strings.ReplaceAll(scopeText, " ", ","), ","), false)
	if len(scopes) == 0 {
		return "project:read"
	}
	allowed := map[string]bool{
		"project:read":   true,
		"project:write":  true,
		"project:*":      true,
		"application:*":  true,
		"git:read":       true,
		"git:write":      true,
		"git:*":          true,
		"registry:read":  true,
		"registry:write": true,
		"registry:*":     true,
		"image:read":     true,
		"image:write":    true,
		"image:*":        true,
		"user:read":      true,
		"user:write":     true,
		"user:manage":    true,
		"user:*":         true,
		"config:read":    true,
		"config:write":   true,
		"config:*":       true,
		"auth:manage":    true,
		"token:manage":   true,
		"billing:read":   true,
		"billing:write":  true,
		"billing:*":      true,
	}
	for _, scope := range scopes {
		if scope == "*" || !allowed[scope] {
			return ""
		}
	}
	return strings.Join(scopes, ",")
}

func UserCanCreateAccessTokenScope(userRole, scopeText string) bool {
	if userRole == "platform_admin" {
		return true
	}
	for _, scope := range splitCSV(scopeText) {
		switch scope {
		case "project:read", "git:read", "registry:read", "image:read", "user:read", "billing:read":
			continue
		default:
			return false
		}
	}
	return true
}
