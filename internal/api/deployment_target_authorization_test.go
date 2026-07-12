package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequireInteractiveSessionRejectsBearerToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/projects/prj/applications/app/deployment-targets/dplt/data-export", nil)
	ctx.Request.Header.Set("Authorization", "Bearer token-with-data-export-scope")

	if requireInteractiveSession(ctx) {
		t.Fatal("expected bearer token request to require an interactive session")
	}
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["code"] != "auth.interactive_session_required" {
		t.Fatalf("code = %v, want auth.interactive_session_required", response["code"])
	}
}

func TestAuthorizeDeploymentTargetDataExportRejectsBearerTokenBeforeAuthorization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/projects/prj/applications/app/deployment-targets/dplt/data-export/authorize", nil)
	ctx.Request.Header.Set("Authorization", "Bearer token-with-deployment-wildcard-scope")

	(&Handlers{}).AuthorizeDeploymentTargetDataExport(ctx)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["code"] != "auth.interactive_session_required" {
		t.Fatalf("code = %v, want auth.interactive_session_required", response["code"])
	}
}

func TestRequireInteractiveSessionAllowsCookieAuthenticationToContinue(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/projects/prj/applications/app/deployment-targets/dplt/data-export", nil)
	ctx.Request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "sess_test"})

	if !requireInteractiveSession(ctx) {
		t.Fatal("expected cookie-authenticated request to continue to session validation")
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected response status = %d", recorder.Code)
	}
}

func TestRequireInteractiveSessionRejectsMissingCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/projects/prj/applications/app/deployment-targets/dplt/data-export", nil)

	if requireInteractiveSession(ctx) {
		t.Fatal("expected request without a session cookie to be rejected")
	}
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response["code"] != "auth.session.missing" {
		t.Fatalf("code = %v, want auth.session.missing", response["code"])
	}
}
