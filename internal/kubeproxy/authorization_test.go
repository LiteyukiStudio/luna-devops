package kubeproxy

import (
	"testing"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/kubecatalog"
)

func baseAccess() AccessContext {
	return AccessContext{UserID: "u1", BindingID: "b1", ProjectID: "p1", Namespace: "project-a", ProjectRole: authz.ProjectRoleDeveloper, Scopes: ScopeKubeRead + "," + ScopeKubeWrite + "," + ScopeKubeConnect}
}

func TestAuthorizerRejectsApplicationSharedResourcesBeforeUpstream(t *testing.T) {
	access := baseAccess()
	access.ApplicationID = "a1"
	authorizer := CatalogAuthorizer{Catalog: kubecatalog.New()}
	for _, resource := range []string{"events", "serviceaccounts", "resourcequotas", "limitranges"} {
		_, err := authorizer.Authorize(t.Context(), access, RequestInfo{Verb: "list", APIVersion: "v1", Resource: resource, Namespace: access.Namespace, IsResourceRequest: true, IsCollection: true})
		if err == nil {
			t.Fatalf("application binding must not read %s", resource)
		}
	}
}

func TestAuthorizerUsesCentralProjectPolicy(t *testing.T) {
	access := baseAccess()
	access.ProjectRole = authz.ProjectRoleViewer
	authorizer := CatalogAuthorizer{Catalog: kubecatalog.New()}
	_, err := authorizer.Authorize(t.Context(), access, RequestInfo{Verb: "create", APIGroup: "apps", APIVersion: "v1", Resource: "deployments", Namespace: access.Namespace, IsResourceRequest: true, IsCollection: true})
	if err == nil {
		t.Fatal("viewer must not create deployments")
	}
	decision, err := authorizer.Authorize(t.Context(), access, RequestInfo{Verb: "list", APIGroup: "apps", APIVersion: "v1", Resource: "deployments", Namespace: access.Namespace, IsResourceRequest: true, IsCollection: true})
	if err != nil || !decision.Allowed {
		t.Fatalf("viewer should read deployments: decision=%#v err=%v", decision, err)
	}
}

func TestLocalReviewCreationRequiresReadRatherThanWriteScope(t *testing.T) {
	access := baseAccess()
	access.Scopes = ScopeKubeRead
	authorizer := CatalogAuthorizer{Catalog: kubecatalog.New()}
	info := RequestInfo{
		Verb: "create", APIGroup: "authorization.k8s.io", APIVersion: "v1", Resource: "selfsubjectaccessreviews",
		IsResourceRequest: true, IsCollection: true,
	}
	decision, err := authorizer.Authorize(t.Context(), access, info)
	if err != nil || !decision.Allowed {
		t.Fatalf("read-scoped credential must be able to perform a local self review: decision=%#v err=%v", decision, err)
	}
}
