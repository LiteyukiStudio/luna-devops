package aiapi

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/gin-gonic/gin"
)

func TestAIRequestMatchesConversationProject(t *testing.T) {
	gin.SetMode(gin.TestMode)
	queryOperation := aitool.OpenAPIOperation{
		Parameters:  []aitool.OpenAPIParameter{{InputName: "projectId", WireName: "projectId", In: "query"}},
		InputSchema: map[string]any{"properties": map[string]any{"projectId": map[string]any{"type": "string"}}},
	}
	for name, target := range map[string]string{
		"matching":   "/api/v1/resources?projectId=prj_bound",
		"mismatched": "/api/v1/resources?projectId=prj_other",
		"missing":    "/api/v1/resources",
	} {
		t.Run(name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("GET", target, nil)
			got := aiRequestMatchesConversationProject(ctx, queryOperation, "prj_bound")
			if got != (name == "matching") {
				t.Fatalf("match = %v", got)
			}
		})
	}

	bodyOperation := aitool.OpenAPIOperation{
		RequestBody: true,
		InputSchema: map[string]any{"properties": map[string]any{"projectId": map[string]any{"type": "string"}}},
	}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/api/v1/resources", strings.NewReader(`{"projectId":"prj_bound","nested":{"projectIds":["prj_bound"]}}`))
	if !aiRequestMatchesConversationProject(ctx, bodyOperation, "prj_bound") {
		t.Fatal("matching body project was rejected")
	}
	restored, err := io.ReadAll(ctx.Request.Body)
	if err != nil || !strings.Contains(string(restored), "prj_bound") {
		t.Fatalf("request body was not restored: %q, %v", restored, err)
	}

	ctx, _ = gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/api/v1/resources", strings.NewReader(`{"projectId":"prj_other"}`))
	if aiRequestMatchesConversationProject(ctx, bodyOperation, "prj_bound") {
		t.Fatal("mismatched body project was accepted")
	}

	ctx, _ = gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/api/v1/resources", strings.NewReader(`{"projectId":"prj_other"}`))
	if !aiRequestMatchesConversationProject(ctx, bodyOperation, "") {
		t.Fatal("unbound conversation should defer project authorization to the business handler")
	}
}

func TestOpenAPIPathToGin(t *testing.T) {
	if got := openAPIPathToGin("/api/v1/projects/{projectId}/volumes/{volumeId}"); got != "/api/v1/projects/:projectId/volumes/:volumeId" {
		t.Fatalf("path = %q", got)
	}
}
