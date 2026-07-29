package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestInternalErrorCodeUsesStableRouteTemplate(t *testing.T) {
	router := gin.New()
	router.GET("/api/v1/projects/:projectId", func(ctx *gin.Context) {
		if code := internalErrorCode(ctx); code != "internal_error.get_api_v1_projects_projectid" {
			t.Fatalf("code = %q", code)
		}
		ctx.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/projects/prj_123", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestInternalErrorCodeFallsBackWithoutRegisteredRoute(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/unknown", nil)
	if code := internalErrorCode(ctx); code != "internal_error" {
		t.Fatalf("code = %q", code)
	}
}

func TestWriteErrorKeyWithDetailsKeepsStableMachineReadableContext(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	writeErrorKeyWithDetails(
		ctx,
		http.StatusForbidden,
		"en-US",
		"auth.token.scope_insufficient",
		gin.H{"requiredScope": "deployment:data_export"},
	)

	var response struct {
		Code    string `json:"code"`
		Details struct {
			RequiredScope string `json:"requiredScope"`
		} `json:"details"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if response.Code != "auth.token.scope_insufficient" {
		t.Fatalf("code = %q", response.Code)
	}
	if response.Details.RequiredScope != "deployment:data_export" {
		t.Fatalf("required scope = %q", response.Details.RequiredScope)
	}
}
