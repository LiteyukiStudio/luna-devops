package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
)

func TestRuntimeWebConsoleEnabled(t *testing.T) {
	enabled := true
	disabled := false
	tests := []struct {
		name           string
		projectEnabled bool
		targetOverride *bool
		want           bool
	}{
		{name: "inherits enabled project default", projectEnabled: true, want: true},
		{name: "inherits disabled project default", projectEnabled: false, want: false},
		{name: "target disables enabled project", projectEnabled: true, targetOverride: &disabled, want: false},
		{name: "target cannot enable disabled project", projectEnabled: false, targetOverride: &enabled, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := runtimeWebConsoleEnabled(
				model.Project{WebConsoleEnabled: test.projectEnabled},
				model.DeploymentTarget{WebConsoleEnabled: test.targetOverride},
			)
			if got != test.want {
				t.Fatalf("runtimeWebConsoleEnabled() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestNormalizeWebConsoleOverrideOnlyKeepsFurtherDisable(t *testing.T) {
	enabled := true
	disabled := false
	if got := normalizeWebConsoleOverride(nil); got != nil {
		t.Fatalf("inherit override = %v, want nil", got)
	}
	if got := normalizeWebConsoleOverride(&enabled); got != nil {
		t.Fatalf("enabled override = %v, want inherit", *got)
	}
	if got := normalizeWebConsoleOverride(&disabled); got == nil || *got {
		t.Fatalf("disabled override = %v, want false", got)
	}
}

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
