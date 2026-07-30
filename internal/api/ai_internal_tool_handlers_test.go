package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAIToolRegistryRejectsArbitraryOperationsAndPaths(t *testing.T) {
	if _, exists := aiToolPolicies["httpRequest"]; exists {
		t.Fatal("arbitrary HTTP must never enter the AI tool catalog")
	}
	if _, exists := aiToolPolicies["executeSQL"]; exists {
		t.Fatal("arbitrary SQL must never enter the AI tool catalog")
	}
	for operationID, policy := range aiToolPolicies {
		if operationID != policy.OperationID {
			t.Fatalf("mismatched tool policy %s = %#v", operationID, policy)
		}
		if operationID == "createProject" {
			if policy.Risk != "write" || policy.ApprovalRequired || policy.MFAPurpose != "" {
				t.Fatalf("unexpected low-risk project creation policy = %#v", policy)
			}
			continue
		}
		if policy.Risk != "read" || policy.ApprovalRequired || policy.MFAPurpose != "" {
			t.Fatalf("unsafe registered tool policy %s = %#v", operationID, policy)
		}
	}
}

func TestAIToolRegistryIncludesP2DiagnosticCatalog(t *testing.T) {
	expected := map[string]string{
		"listGatewayRoutes":          "gateway:read",
		"listGatewayCertificates":    "gateway:read",
		"listProjectHookRuns":        "project:read",
		"listNotificationDeliveries": "event:read",
		"listRuntimeEvents":          "event:read",
	}
	for operationID, scope := range expected {
		policy, ok := aiToolPolicies[operationID]
		if !ok || len(policy.Scopes) != 1 || policy.Scopes[0] != scope || policy.Risk != "read" {
			t.Errorf("P2 policy %s = %#v", operationID, policy)
		}
	}
}

func TestAIAgentCallbackCredentialFailsClosed(t *testing.T) {
	t.Setenv("AI_INTERNAL_SECRET", "")
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/internal/v1/ai/delegations/exchange", nil)
	ctx.Request.Header.Set("Authorization", "Bearer attacker")
	if requireAIAgentService(ctx) {
		t.Fatal("empty server credential must fail closed")
	}
	if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), "ai.agent_service_not_configured") {
		t.Fatalf("response = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestAIArgumentsHashIsStableAndSensitiveToChanges(t *testing.T) {
	first := hashAIArguments(map[string]any{"projectId": "prj_1", "limit": float64(20)})
	second := hashAIArguments(map[string]any{"limit": float64(20), "projectId": "prj_1"})
	changed := hashAIArguments(map[string]any{"projectId": "prj_2", "limit": float64(20)})
	if first != second || first == changed || !strings.HasPrefix(first, "sha256:") {
		t.Fatalf("hashes = %q %q %q", first, second, changed)
	}
}
