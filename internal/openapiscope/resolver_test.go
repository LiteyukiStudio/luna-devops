package openapiscope

import (
	"errors"
	"reflect"
	"testing"
)

func TestResolverMatchesGinAndOpenAPIPathTemplates(t *testing.T) {
	resolver, err := New([]byte(`
openapi: 3.1.0
paths:
  /api/v1/projects/{projectId}/applications/{applicationId}:
    patch:
      operationId: updateApplication
      x-luna-cli:
        requiredScopes: [application:update, application:update]
`))
	if err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{
		"/api/v1/projects/{projectId}/applications/{applicationId}",
		"/api/v1/projects/:projectId/applications/:applicationId",
	} {
		scopes, err := resolver.RequiredScopes(path, "PATCH")
		if err != nil {
			t.Fatalf("resolve %s: %v", path, err)
		}
		if !reflect.DeepEqual(scopes, []string{"application:update"}) {
			t.Fatalf("scopes for %s = %#v", path, scopes)
		}
	}
}

func TestResolverFailsClosedWithoutRequiredScopes(t *testing.T) {
	resolver, err := New([]byte(`
openapi: 3.1.0
paths:
  /api/v1/projects:
    get:
      operationId: listProjects
`))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := resolver.RequiredScopes("/api/v1/projects", "GET"); !errors.Is(err, ErrRequiredScopesNotDeclared) {
		t.Fatalf("missing requiredScopes error = %v", err)
	}
	if _, err := resolver.RequiredScopes("/api/v1/unknown", "GET"); !errors.Is(err, ErrRequiredScopesNotDeclared) {
		t.Fatalf("unknown route error = %v", err)
	}
}
