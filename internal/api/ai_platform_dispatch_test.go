package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/aiagent"
	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/gin-gonic/gin"
)

func TestBuildAIPlatformRequestBindsPathQueryAndBody(t *testing.T) {
	operation := aitool.OpenAPIOperation{
		OperationID: "updateApplication", Method: http.MethodPut,
		Path: "/api/v1/projects/{projectId}/applications/{applicationId}",
		Parameters: []aitool.OpenAPIParameter{
			{InputName: "projectId", WireName: "projectId", In: "path", Required: true},
			{InputName: "applicationId", WireName: "applicationId", In: "path", Required: true},
			{InputName: "dryRun", WireName: "dryRun", In: "query"},
		},
		RequestBody: true, RequestRequired: true, RequestType: "application/json",
	}
	target, body, _, err := buildAIPlatformRequest(operation, map[string]any{
		"projectId": "prj_1", "applicationId": "app/1", "dryRun": true,
		"body": map[string]any{"name": "Example"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if target != "/api/v1/projects/prj_1/applications/app%2F1?dryRun=true" {
		t.Fatalf("target = %q", target)
	}
	encoded, _ := io.ReadAll(body)
	var payload map[string]any
	if json.Unmarshal(encoded, &payload) != nil || payload["name"] != "Example" {
		t.Fatalf("body = %s", encoded)
	}
}

func TestBuildAIPlatformRequestRejectsUnknownOrMissingArguments(t *testing.T) {
	operation := aitool.OpenAPIOperation{
		OperationID: "getProject", Method: http.MethodGet,
		Path:       "/api/v1/projects/{projectId}",
		Parameters: []aitool.OpenAPIParameter{{InputName: "projectId", WireName: "projectId", In: "path", Required: true}},
	}
	for _, arguments := range []map[string]any{
		{},
		{"projectId": "prj_1", "unexpected": "value"},
	} {
		if _, _, _, err := buildAIPlatformRequest(operation, arguments); err == nil {
			t.Fatalf("arguments should be rejected: %#v", arguments)
		}
	}
}

func TestAppendAIQueryValuePreservesRepeatedValues(t *testing.T) {
	operation := aitool.OpenAPIOperation{
		OperationID: "listExample", Method: http.MethodGet, Path: "/api/v1/example",
		Parameters: []aitool.OpenAPIParameter{{InputName: "status", WireName: "status", In: "query"}},
	}
	target, _, _, err := buildAIPlatformRequest(operation, map[string]any{"status": []any{"ready", "failed"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(target, "status=ready") || !strings.Contains(target, "status=failed") {
		t.Fatalf("target = %q", target)
	}
}

func TestBuildAIProjectListRequestPreservesExplicitScope(t *testing.T) {
	operation, ok := aitool.PlatformOperation("listProjects")
	if !ok {
		t.Fatal("missing operation listProjects")
	}
	target, _, _, err := buildAIPlatformRequest(operation, map[string]any{
		"scope": "all", "page": float64(2), "pageSize": float64(50),
	})
	if err != nil {
		t.Fatal(err)
	}
	if target != "/api/v1/projects?page=2&pageSize=50&scope=all" {
		t.Fatalf("target = %q", target)
	}
}

func TestDispatchAIPlatformOperationPropagatesDelegatedSessionRunAndMFA(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/probe", func(ctx *gin.Context) {
		actor, ok := ctx.Request.Context().Value(aiPlatformActorContextKey{}).(aiPlatformActor)
		if !ok {
			ctx.Status(http.StatusUnauthorized)
			return
		}
		ctx.JSON(http.StatusOK, gin.H{
			"userId": actor.UserID, "sessionId": actor.SessionID,
			"mfaPurpose": actor.MFAPurpose, "mfaAssertion": actor.MFAAssertion,
			"runId": ctx.GetHeader("X-Luna-AI-Run-ID"),
		})
	})
	handlers := &Handlers{platformRouter: router}
	recorder := httptest.NewRecorder()
	parent, _ := gin.CreateTestContext(recorder)
	parent.Request = httptest.NewRequest(http.MethodPost, "/internal/v1/ai/tools/probe/execute", nil)
	claims := aiagent.DelegationClaims{
		UserID: "usr_1", SessionID: "ses_1", RunID: "airun_1", ToolCallID: "aitool_1",
		ArgumentsHash: "sha256:test", MFAPurpose: stepUpPurposeRuntimeExec, MFAAssertion: "mfaas_1",
	}
	result, err := handlers.dispatchAIPlatformOperation(parent, claims, aitool.OpenAPIOperation{
		OperationID: "probe", Method: http.MethodPost, Path: "/probe",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	payload, ok := result.Body.(map[string]any)
	if !ok || payload["userId"] != claims.UserID || payload["sessionId"] != claims.SessionID ||
		payload["runId"] != claims.RunID || payload["mfaPurpose"] != claims.MFAPurpose ||
		payload["mfaAssertion"] != claims.MFAAssertion {
		t.Fatalf("delegated actor context was not preserved: %#v", result.Body)
	}
}
