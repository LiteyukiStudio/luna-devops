package service

import (
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/openapiscope"
)

func RequiredAccessTokenScopes(path, method string) ([]string, error) {
	return openapiscope.RequiredScopes(path, method)
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
