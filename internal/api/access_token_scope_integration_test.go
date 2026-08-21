package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/database"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/service"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestPlatformAdminAccessTokenScopesAuthorizeDashboardAndDataRetention(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	db := newAccessTokenScopeIntegrationDB(t)
	suffix := randomHex(4)
	user := model.User{
		ID:       "usr_scope_" + suffix,
		Email:    "scope-" + suffix + "@example.com",
		Name:     "Scope Admin",
		Role:     authz.PlatformRoleAdmin,
		Language: "en-US",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	fullToken := "pat_scope_full_" + suffix
	insufficientToken := "pat_scope_limited_" + suffix
	tokens := []model.AccessToken{
		{
			ID:        "pat_scope_full_" + suffix,
			UserID:    user.ID,
			Name:      "Full scope",
			Scope:     "*",
			TokenHash: hashToken(fullToken),
			Source:    "personal",
		},
		{
			ID:        "pat_scope_limited_" + suffix,
			UserID:    user.ID,
			Name:      "Insufficient scope",
			Scope:     "project:read",
			TokenHash: hashToken(insufficientToken),
			Source:    "personal",
		},
	}
	if err := db.Create(&tokens).Error; err != nil {
		t.Fatal(err)
	}

	router := NewRouter(db)
	for _, path := range []string{"/api/v1/dashboard", "/api/v1/data-retention/catalog"} {
		t.Run("full scope "+path, func(t *testing.T) {
			recorder := performBearerRequest(router, http.MethodGet, path, fullToken, "")
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
		})
		t.Run("insufficient scope "+path, func(t *testing.T) {
			recorder := performBearerRequest(router, http.MethodGet, path, insufficientToken, "")
			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
			assertRequiredScopeError(t, recorder, service.RequiredAccessTokenScope(path, http.MethodGet))
		})
	}

	t.Run("full scope Agent observability reaches the availability boundary", func(t *testing.T) {
		recorder := performBearerRequest(router, http.MethodGet, "/api/v1/ai/observability/overview", fullToken, "")
		if recorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
		var response map[string]any
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if response["status"] != "unavailable" || response["observationCode"] != "ai.observability.disabled" {
			t.Fatalf("response = %#v", response)
		}
	})
	t.Run("insufficient scope blocks Agent observability", func(t *testing.T) {
		path := "/api/v1/ai/observability/overview"
		recorder := performBearerRequest(router, http.MethodGet, path, insufficientToken, "")
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
		assertRequiredScopeError(t, recorder, service.RequiredAccessTokenScope(path, http.MethodGet))
	})

	retentionBody := `{"datasets":["platform_events"],"startAt":"2026-01-01T00:00:00Z","endAt":"2026-01-02T00:00:00Z"}`
	t.Run("full scope retention preview", func(t *testing.T) {
		recorder := performBearerRequest(router, http.MethodPost, "/api/v1/data-retention/preview", fullToken, retentionBody)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
	})
	t.Run("insufficient scope retention preview", func(t *testing.T) {
		recorder := performBearerRequest(router, http.MethodPost, "/api/v1/data-retention/preview", insufficientToken, retentionBody)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("full scope retention cleanup", func(t *testing.T) {
		recorder := performBearerRequest(router, http.MethodPost, "/api/v1/data-retention/cleanup", fullToken, retentionBody)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
	})
	t.Run("insufficient scope retention cleanup", func(t *testing.T) {
		recorder := performBearerRequest(router, http.MethodPost, "/api/v1/data-retention/cleanup", insufficientToken, retentionBody)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
	})
}

func assertRequiredScopeError(
	t *testing.T,
	recorder *httptest.ResponseRecorder,
	requiredScope string,
) {
	t.Helper()
	var response struct {
		Code    string `json:"code"`
		Details struct {
			RequiredScope string `json:"requiredScope"`
		} `json:"details"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode scope error: %v", err)
	}
	if response.Code != "auth.token.scope_insufficient" {
		t.Fatalf("error code = %q", response.Code)
	}
	if response.Details.RequiredScope != requiredScope {
		t.Fatalf(
			"required scope = %q, want %q",
			response.Details.RequiredScope,
			requiredScope,
		)
	}
}

func performBearerRequest(router http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+token)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	router.ServeHTTP(recorder, request)
	return recorder
}

func newAccessTokenScopeIntegrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	databaseURL := os.Getenv("AUTH_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is not configured")
	}
	adminDB, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		t.Fatalf("open integration database: %v", err)
	}
	databaseName := fmt.Sprintf("luna_access_token_scope_test_%d", time.Now().UnixNano())
	if !strings.HasPrefix(databaseName, "luna_access_token_scope_test_") {
		t.Fatalf("refuse unsafe integration test database name %q", databaseName)
	}
	if err := adminDB.Exec(`CREATE DATABASE "` + databaseName + `"`).Error; err != nil {
		t.Fatalf("create isolated integration database: %v", err)
	}

	parsedURL, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse integration database URL: %v", err)
	}
	parsedURL.Path = "/" + databaseName
	parsedURL.RawPath = ""
	query := parsedURL.Query()
	query.Del("search_path")
	parsedURL.RawQuery = query.Encode()
	db, err := gorm.Open(postgres.Open(parsedURL.String()), &gorm.Config{})
	if err != nil {
		t.Fatalf("open integration schema: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
		if dropErr := adminDB.Exec(`DROP DATABASE IF EXISTS "` + databaseName + `" WITH (FORCE)`).Error; dropErr != nil {
			t.Errorf("drop isolated integration database: %v", dropErr)
		}
		if sqlDB, dbErr := adminDB.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	if err := database.MigrateContext(context.Background(), db); err != nil {
		t.Fatalf("migrate integration schema: %v", err)
	}
	return db
}
