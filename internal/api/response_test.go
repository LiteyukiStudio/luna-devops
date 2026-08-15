package api

import (
	"encoding/json"
	"fmt"
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
	t.Setenv("APP_ENV", "development")
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	writeErrorKeyWithDetails(
		ctx,
		http.StatusForbidden,
		"en-US",
		"auth.token.scope_insufficient",
		gin.H{"requiredScope": "volume:export"},
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
	if response.Details.RequiredScope != "volume:export" {
		t.Fatalf("required scope = %q", response.Details.RequiredScope)
	}
}

func TestProductionErrorResponseContainsOnlySafeFields(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	router := gin.New()
	router.Use(requestIDMiddleware())
	router.GET("/failure", func(ctx *gin.Context) {
		writeErrorCode(
			ctx,
			http.StatusInternalServerError,
			"provider.request_failed",
			"pq: relation secrets does not exist at /srv/luna/internal/provider/client.go",
		)
	})

	request := httptest.NewRequest(http.MethodGet, "/failure?detail=true", nil)
	request.Header.Set("Accept-Language", "en-US")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if len(response) != 3 {
		t.Fatalf("production response fields = %#v", response)
	}
	if response["code"] != "provider.request_failed" {
		t.Fatalf("code = %#v", response["code"])
	}
	if response["error"] != "The service is temporarily unavailable. Please try again later." {
		t.Fatalf("public error = %#v", response["error"])
	}
	if requestID, ok := response["requestId"].(string); !ok || !strings.HasPrefix(requestID, "req_") {
		t.Fatalf("requestId = %#v", response["requestId"])
	}
	if strings.Contains(recorder.Body.String(), "pq:") || strings.Contains(recorder.Body.String(), "/srv/luna") {
		t.Fatalf("production response leaked diagnostic detail: %s", recorder.Body.String())
	}
}

func TestDevelopmentErrorResponseKeepsDiagnosticDetail(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/failure", nil)

	writeErrorCode(ctx, http.StatusInternalServerError, "provider.request_failed", "provider connection refused")

	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if response["error"] != "provider connection refused" || response["detail"] != "provider connection refused" {
		t.Fatalf("development response lost diagnostic detail: %#v", response)
	}
}

func TestProductionUnknownMiddlewareErrorUsesStableFallback(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	router := gin.New()
	router.Use(requestIDMiddleware(), errorResponseMiddleware())
	router.GET("/failure", func(ctx *gin.Context) {
		_ = ctx.Error(fmt.Errorf("token=secret provider url=http://internal-provider.local"))
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/failure", nil))

	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if response["code"] != "internal_error" {
		t.Fatalf("code = %#v", response["code"])
	}
	if strings.Contains(recorder.Body.String(), "secret") || strings.Contains(recorder.Body.String(), "internal-provider") {
		t.Fatalf("middleware response leaked raw error: %s", recorder.Body.String())
	}
}

func TestProductionKeyDetailsAreOmitted(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/failure", nil)

	writeErrorKeyWithDetails(
		ctx,
		http.StatusForbidden,
		"en-US",
		"auth.token.scope_insufficient",
		gin.H{"requiredScope": "volume:export"},
	)

	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if _, exists := response["details"]; exists {
		t.Fatalf("production response included details: %#v", response)
	}
	if _, exists := response["requestId"]; !exists {
		t.Fatalf("production response omitted requestId: %#v", response)
	}
}

func TestDirectErrorMessagesAreLocalized(t *testing.T) {
	tests := []struct {
		language string
		key      string
		want     string
	}{
		{language: "zh-CN", key: "mfa_required", want: "需要完成敏感操作二次验证"},
		{language: "en-US", key: "mfa_required", want: "Additional verification is required for this sensitive action."},
		{language: "zh-CN", key: "service_binding_in_use", want: "该应用或部署配置仍被服务关系引用，请先删除相关关系"},
		{language: "en-US", key: "service_binding_in_use", want: "This application or deployment target is still referenced by a service relation. Remove the relation first."},
	}
	for _, test := range tests {
		t.Run(test.language+"/"+test.key, func(t *testing.T) {
			if got := messageFor(test.language, test.key); got != test.want {
				t.Fatalf("messageFor(%q, %q) = %q, want %q", test.language, test.key, got, test.want)
			}
		})
	}
}

func TestProductionTerminalDisconnectMessageIsSafe(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/terminal", nil)
	ctx.Request.Header.Set("Accept-Language", "en-US")

	message := string(terminalDisconnectedMessage(
		ctx,
		"dial tcp https://internal-provider.local: token=secret connection refused",
	))

	if !strings.Contains(message, "The terminal connection was closed.") ||
		!strings.Contains(message, "code="+terminalDisconnectedErrorCode) ||
		!strings.Contains(message, "requestId=req_") {
		t.Fatalf("production terminal error omitted safe context: %q", message)
	}
	if strings.Contains(message, "internal-provider") || strings.Contains(message, "token=secret") {
		t.Fatalf("production terminal error leaked diagnostic detail: %q", message)
	}
}

func TestDevelopmentTerminalDisconnectMessageKeepsDetail(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/terminal", nil)
	detail := "provider connection refused"

	message := string(terminalDisconnectedMessage(ctx, detail))

	if !strings.Contains(message, detail) {
		t.Fatalf("development terminal error lost diagnostic detail: %q", message)
	}
}

func TestServiceBindingConflictOmitsAffectedSourcesInProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/service-binding-target", nil)

	writeServiceBindingInUse(ctx, []serviceBindingUsage{{
		BindingID: "svb_1", SourceApplicationName: "private-application",
	}})

	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode production conflict: %v", err)
	}
	if _, exists := body["affectedSources"]; exists {
		t.Fatalf("production conflict exposed affected sources: %#v", body)
	}
	if body["code"] != "service_binding_in_use" {
		t.Fatalf("unexpected conflict response: %#v", body)
	}
}

func TestServiceBindingConflictKeepsAffectedSourcesInDevelopment(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/service-binding-target", nil)

	writeServiceBindingInUse(ctx, []serviceBindingUsage{{
		BindingID: "svb_1", SourceApplicationName: "debug-application",
	}})

	var body struct {
		AffectedSources []serviceBindingUsage `json:"affectedSources"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode development conflict: %v", err)
	}
	if len(body.AffectedSources) != 1 || body.AffectedSources[0].SourceApplicationName != "debug-application" {
		t.Fatalf("development conflict lost affected sources: %#v", body)
	}
}
