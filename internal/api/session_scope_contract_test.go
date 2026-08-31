package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestWildcardAccessTokenFailsClosedWithoutScopeContract(t *testing.T) {
	missing, err := missingRequiredAccessTokenScope("*", "/api/v1/contract-is-not-declared", http.MethodGet)
	if err == nil {
		t.Fatalf("unknown route authorized with missing scope %q", missing)
	}
	if missing != "" {
		t.Fatalf("contract failure must not enter ordinary scope matching: %q", missing)
	}
}

func TestWildcardAccessTokenAllowsDeclaredScopeContract(t *testing.T) {
	missing, err := missingRequiredAccessTokenScope("*", "/api/v1/dashboard", http.MethodGet)
	if err != nil {
		t.Fatal(err)
	}
	if missing != "" {
		t.Fatalf("wildcard token unexpectedly missed declared scope %q", missing)
	}
}

func TestDestructiveAndInteractiveScopesUseLeastPrivilegeContracts(t *testing.T) {
	tests := []struct {
		name              string
		path              string
		method            string
		insufficientScope string
		requiredScope     string
	}{
		{
			name:              "project deletion does not accept project write",
			path:              "/api/v1/projects/:projectId",
			method:            http.MethodDelete,
			insufficientScope: "project:write",
			requiredScope:     "project:delete",
		},
		{
			name:              "runtime terminal does not accept cluster read",
			path:              "/api/v1/runtime/clusters/:clusterId/pods/terminal",
			method:            http.MethodGet,
			insufficientScope: "cluster:read",
			requiredScope:     "cluster:manage",
		},
		{
			name:              "gateway deletion does not accept gateway manage",
			path:              "/api/v1/projects/:projectId/gateway-routes/:routeId",
			method:            http.MethodDelete,
			insufficientScope: "gateway:manage",
			requiredScope:     "gateway:delete",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			missing, err := missingRequiredAccessTokenScope(test.insufficientScope, test.path, test.method)
			if err != nil {
				t.Fatal(err)
			}
			if missing != test.requiredScope {
				t.Fatalf("insufficient scope missed %q, want %q", missing, test.requiredScope)
			}
			missing, err = missingRequiredAccessTokenScope(test.requiredScope, test.path, test.method)
			if err != nil {
				t.Fatal(err)
			}
			if missing != "" {
				t.Fatalf("required scope unexpectedly missed %q", missing)
			}
		})
	}
}

func TestScopeContractFailureReturnsStableSafeServiceUnavailable(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	setRuntimeMode(ctx, "production")
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/contract-is-not-declared", nil)

	writeScopeContractUnavailableError(ctx, "OpenAPI parser diagnostic")

	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusServiceUnavailable || response["code"] != "auth.token.scope_contract_unavailable" {
		t.Fatalf("scope contract failure = status %d, body %#v", recorder.Code, response)
	}
	if _, exists := response["developerDetail"]; exists {
		t.Fatalf("production response exposed contract diagnostic: %#v", response)
	}
}
