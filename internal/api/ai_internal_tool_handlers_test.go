package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/gin-gonic/gin"
)

func TestAIToolRegistryRejectsArbitraryOperationsAndPaths(t *testing.T) {
	if _, exists := aiToolPolicies["httpRequest"]; exists {
		t.Fatal("arbitrary HTTP must never enter the AI tool catalog")
	}
	if _, exists := aiToolPolicies["executeSQL"]; exists {
		t.Fatal("arbitrary SQL must never enter the AI tool catalog")
	}
	if len(aiToolPolicies) < 150 {
		t.Fatalf("Agent platform catalog is unexpectedly small: %d", len(aiToolPolicies))
	}
	for operationID, policy := range aiToolPolicies {
		if operationID != policy.OperationID {
			t.Fatalf("mismatched tool policy %s = %#v", operationID, policy)
		}
		if (policy.Risk == "sensitive" || policy.Risk == "destructive") && !policy.ApprovalRequired {
			t.Fatalf("high-risk operation does not require approval %s = %#v", operationID, policy)
		}
		if policy.Risk == "read" && policy.ApprovalRequired {
			t.Fatalf("read operation unexpectedly requires approval %s = %#v", operationID, policy)
		}
	}
	if policy := aiToolPolicies["createProject"]; policy.Risk != "write" || policy.ApprovalRequired {
		t.Fatalf("unexpected low-risk project creation policy = %#v", policy)
	}
}

func TestAIOpenAPIToolsDelegateAuthorizationToPlatformHandlers(t *testing.T) {
	for operationID, policy := range aiToolPolicies {
		if policy.Risk != "read" || policy.ProjectAction == "" {
			continue
		}
		if !authz.ProjectRoleAllows(authz.ProjectRoleViewer, policy.ProjectAction) {
			t.Errorf("viewer denied read operation %s via action %s", operationID, policy.ProjectAction)
		}
	}
	for _, operationID := range []string{"listApplications", "createApplication", "deleteApplication"} {
		if aiToolPolicies[operationID].ProjectAction != "" {
			t.Fatalf("OpenAPI operation %s must reuse its platform Handler authorization: %#v", operationID, aiToolPolicies[operationID])
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

func TestAIToolRegistryIncludesPublicWebReadTools(t *testing.T) {
	for _, operationID := range []string{"webSearch", "fetchWebPage"} {
		policy, ok := aiToolPolicies[operationID]
		if !ok {
			t.Fatalf("missing %s policy", operationID)
		}
		if policy.Risk != "read" || policy.ApprovalRequired || len(policy.Scopes) != 1 || policy.Scopes[0] != "web:read" {
			t.Fatalf("unexpected %s policy = %#v", operationID, policy)
		}
	}
}

func TestAIToolRegistryRequiresFreshApprovalAndMFAForEveryRuntimeSessionCommand(t *testing.T) {
	for _, operationID := range []string{
		"createReleaseRuntimeCommandSession",
		"executeReleaseRuntimeCommandSession",
	} {
		policy, ok := aiToolPolicies[operationID]
		if !ok {
			t.Fatalf("missing runtime command session operation %s", operationID)
		}
		if policy.Risk != "sensitive" || !policy.ApprovalRequired || policy.MFAPurpose != stepUpPurposeRuntimeExec {
			t.Fatalf("runtime command session policy %s = %#v", operationID, policy)
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
	canonical := `{"body":{"applicationIdentifier":"postgres","applicationName":"PostgreSQL","values":{"password":"generated"}},"projectId":"prj_1","templateId":"postgresql"}`
	arguments, err := decodeAICanonicalArguments(canonical)
	if err != nil || arguments["projectId"] != "prj_1" {
		t.Fatalf("decode canonical arguments = %#v, %v", arguments, err)
	}
	first := hashAICanonicalArguments(canonical)
	second := hashAICanonicalArguments(canonical)
	changed := hashAICanonicalArguments(strings.Replace(canonical, "prj_1", "prj_2", 1))
	if first != second || first == changed || !strings.HasPrefix(first, "sha256:") {
		t.Fatalf("canonical hashes = %q %q %q", first, second, changed)
	}
}
