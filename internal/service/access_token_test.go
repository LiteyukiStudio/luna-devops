package service

import (
	"net/http"
	"testing"
)

func TestRequiredAccessTokenScopeRequiresDeploymentExecForReleaseRuntimeExec(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		method string
	}{
		{
			name:   "terminal websocket",
			path:   "/api/v1/projects/:projectId/releases/:releaseId/terminal",
			method: http.MethodGet,
		},
		{
			name:   "exec command",
			path:   "/api/v1/projects/:projectId/releases/:releaseId/exec",
			method: http.MethodPost,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			required := RequiredAccessTokenScope(test.path, test.method)
			if required != "deployment:exec" {
				t.Fatalf("RequiredAccessTokenScope(%q, %q) = %q, want deployment:exec", test.path, test.method, required)
			}
			if AccessTokenAllows("project:read", required) {
				t.Fatal("expected project:read token to be denied")
			}
			if !AccessTokenAllows("deployment:exec", required) {
				t.Fatal("expected deployment:exec token to be allowed")
			}
		})
	}
}

func TestRequiredAccessTokenScopeKeepsReleaseRuntimeLogsDeploymentReadOnly(t *testing.T) {
	required := RequiredAccessTokenScope("/api/v1/projects/:projectId/releases/:releaseId/runtime-logs", http.MethodGet)

	if required != "deployment:read" {
		t.Fatalf("runtime logs required scope = %q, want deployment:read", required)
	}
	if !AccessTokenAllows("deployment:read", required) {
		t.Fatal("expected deployment:read token to read runtime logs")
	}
}
