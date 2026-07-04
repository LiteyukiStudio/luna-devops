package service

import "github.com/LiteyukiStudio/devops/internal/authz"

func RequiredAccessTokenScope(path, method string) string {
	return authz.RequiredAccessTokenScope(path, method)
}

func AccessTokenAllows(scopeText, required string) bool {
	return authz.AccessTokenAllows(scopeText, required)
}

func NormalizeAccessTokenScope(scopeText string) string {
	return authz.NormalizeAccessTokenScope(scopeText)
}

func UserCanCreateAccessTokenScope(userRole, scopeText string) bool {
	return authz.UserCanCreateAccessTokenScope(userRole, scopeText)
}

func AccessTokenScopeCatalog(userRole string) []authz.AccessTokenScopeDefinition {
	return authz.AccessTokenScopeCatalog(userRole)
}
