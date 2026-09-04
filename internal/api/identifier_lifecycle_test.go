package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/api/applicationapi"
	"github.com/gin-gonic/gin"
)

func TestApplicationIdentifierConflictCodes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		status   string
		expected string
	}{
		{name: "active", status: "active", expected: "application.identifier_exists"},
		{name: "deleting", status: "deleting", expected: "application.identifier_delete_in_progress"},
		{name: "delete failed", status: "delete_failed", expected: "application.identifier_delete_failed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/applications", nil)

			applicationapi.WriteApplicationIdentifierConflict(ctx, test.status)

			assertConflictCode(t, recorder, test.expected)
		})
	}
}

func TestDeploymentStageConflictCodes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		status   string
		expected string
	}{
		{name: "active", status: "active", expected: "deployment.stage_exists"},
		{name: "deleting", status: "deleting", expected: "deployment.stage_delete_in_progress"},
		{name: "delete failed", status: "delete_failed", expected: "deployment.stage_delete_failed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/deployment-targets", nil)

			writeDeploymentStageConflict(ctx, test.status)

			assertConflictCode(t, recorder, test.expected)
		})
	}
}

func assertConflictCode(t *testing.T, recorder *httptest.ResponseRecorder, expected string) {
	t.Helper()
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusConflict)
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["code"] != expected {
		t.Fatalf("code = %v, want %q", response["code"], expected)
	}
}
