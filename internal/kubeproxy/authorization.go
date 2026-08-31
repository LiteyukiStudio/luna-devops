package kubeproxy

import (
	"context"
	"fmt"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/kubecatalog"
)

const (
	ScopeKubeRead    = "kube:read"
	ScopeKubeWrite   = "kube:write"
	ScopeKubeConnect = "kube:connect"
)

type CatalogAuthorizer struct {
	Catalog *kubecatalog.Catalog
}

func (authorizer CatalogAuthorizer) Authorize(_ context.Context, access AccessContext, info RequestInfo) (Decision, error) {
	if access.UserID == "" || access.ProjectID == "" || access.BindingID == "" || access.Namespace == "" {
		return Decision{}, Unauthorized(fmt.Errorf("incomplete access context"))
	}
	if info.IsDiscovery {
		if !KubeScopeAllows(access.Scopes, ScopeKubeRead) {
			return Decision{}, Forbidden(CodeForbidden, fmt.Errorf("kube read scope is required"))
		}
		return Decision{Allowed: true, RequiredScopes: []string{ScopeKubeRead}}, nil
	}
	if !info.IsResourceRequest || authorizer.Catalog == nil {
		return Decision{}, NotFound(info.GVR(), info.Name)
	}
	rule, ok := authorizer.Catalog.Lookup(info.GVR())
	if !ok || kubecatalog.IsFixedDenied(info.GVR(), info.Subresource) {
		return Decision{}, Forbidden(CodeForbidden, fmt.Errorf("resource is outside the kubectl catalog"))
	}
	if rule.Namespaced && info.Namespace != access.Namespace {
		return Decision{}, Forbidden(CodeForbidden, fmt.Errorf("namespace is outside the binding"))
	}
	if !rule.Namespaced && info.Namespace != "" {
		return Decision{}, NotFound(info.GVR(), info.Name)
	}
	applicationBinding := strings.TrimSpace(access.ApplicationID) != ""
	if applicationBinding && rule.BindingScope == kubecatalog.BindingScopeProject {
		return Decision{}, Forbidden(CodeForbidden, fmt.Errorf("resource is available only to project bindings"))
	}
	if !applicationBinding && rule.BindingScope == kubecatalog.BindingScopeApplication {
		return Decision{}, Forbidden(CodeForbidden, fmt.Errorf("resource is available only to application bindings"))
	}
	permission, ok := rule.PermissionFor(info.Subresource, info.Verb)
	if !ok {
		return Decision{}, Forbidden(CodeForbidden, fmt.Errorf("resource verb or subresource is not allowed"))
	}
	requiredScopes := requiredScopes(info, rule)
	for _, scope := range requiredScopes {
		if !KubeScopeAllows(access.Scopes, scope) {
			return Decision{}, Forbidden(CodeForbidden, fmt.Errorf("required kube scope is missing"))
		}
	}
	for _, action := range permission.Actions {
		if authz.IsPlatformAdmin(access.PlatformRole) {
			continue
		}
		if !authz.ProjectRoleAllows(access.ProjectRole, action) {
			return Decision{}, Forbidden(CodeForbidden, fmt.Errorf("project role does not allow action %s", action))
		}
	}
	return Decision{Allowed: true, Rule: rule, Actions: permission.Actions, RequiredScopes: requiredScopes}, nil
}

func requiredScopes(info RequestInfo, rule kubecatalog.ResourceRule) []string {
	if rule.Category == kubecatalog.CategoryReview {
		return []string{ScopeKubeRead}
	}
	if info.Subresource == "ephemeralcontainers" {
		return []string{ScopeKubeWrite, ScopeKubeConnect}
	}
	switch info.Verb {
	case "get", "list", "watch":
		return []string{ScopeKubeRead}
	case "connect":
		return []string{ScopeKubeConnect}
	default:
		return []string{ScopeKubeWrite}
	}
}

func KubeScopeAllows(scopeText, required string) bool {
	values := strings.FieldsFunc(scopeText, func(r rune) bool { return r == ',' || r == ' ' })
	for _, value := range values {
		if strings.TrimSpace(value) == required {
			return true
		}
	}
	return false
}
