package deploymentapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
)

func TestEnsureRuntimeWebConsoleEnabledRejectsDisabledTarget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	disabled := false

	if ensureRuntimeWebConsoleEnabled(ctx, model.Project{WebConsoleEnabled: true}, model.DeploymentTarget{WebConsoleEnabled: &disabled}) {
		t.Fatal("expected a disabled deployment target to reject Web Console access")
	}
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["code"] != "runtime.web_console_disabled" {
		t.Fatalf("code = %v, want runtime.web_console_disabled", response["code"])
	}
}

func TestEnsureRuntimeWebConsoleEnabledRejectsDisabledProjectEvenWithEnabledTarget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	enabled := true

	if ensureRuntimeWebConsoleEnabled(ctx, model.Project{WebConsoleEnabled: false}, model.DeploymentTarget{WebConsoleEnabled: &enabled}) {
		t.Fatal("expected the project Web Console ceiling to reject an enabled deployment override")
	}
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
}

func TestRuntimeTerminalAuthorizationResponseIsNotCached(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	markRuntimeTerminalAuthorizationResponse(ctx)

	if value := recorder.Header().Get("Cache-Control"); value != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", value)
	}
}
