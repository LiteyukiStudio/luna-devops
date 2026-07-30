package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestErrorResponseIncludesGeneratedRequestID(t *testing.T) {
	router := gin.New()
	router.Use(requestIDMiddleware())
	router.GET("/failure", func(ctx *gin.Context) {
		writeErrorCode(ctx, http.StatusServiceUnavailable, "ai.tool_storage_unavailable", "database detail")
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/failure", nil))

	var response struct {
		Code      string `json:"code"`
		RequestID string `json:"requestId"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if response.Code != "ai.tool_storage_unavailable" {
		t.Fatalf("code = %q", response.Code)
	}
	if !strings.HasPrefix(response.RequestID, "req_") {
		t.Fatalf("request id = %q", response.RequestID)
	}
	if header := recorder.Header().Get("X-Request-ID"); header != response.RequestID {
		t.Fatalf("X-Request-ID = %q, body requestId = %q", header, response.RequestID)
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
