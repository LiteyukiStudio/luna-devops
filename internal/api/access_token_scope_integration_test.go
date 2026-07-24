package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/database"
	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestPlatformAdminAccessTokenScopesAuthorizeDashboardAndDataRetention(t *testing.T) {
	db := newAccessTokenScopeIntegrationDB(t)
	suffix := randomHex(4)
	user := model.User{
		ID:       "usr_scope_" + suffix,
		Email:    "scope-" + suffix + "@example.com",
		Name:     "Scope Admin",
		Role:     "platform_admin",
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
		})
	}

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
	schema := fmt.Sprintf("access_token_scope_test_%d", time.Now().UnixNano())
	if err := adminDB.Exec(`CREATE SCHEMA "` + schema + `"`).Error; err != nil {
		t.Fatalf("create integration schema: %v", err)
	}

	parsedURL, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse integration database URL: %v", err)
	}
	query := parsedURL.Query()
	query.Set("search_path", schema)
	parsedURL.RawQuery = query.Encode()
	db, err := gorm.Open(postgres.Open(parsedURL.String()), &gorm.Config{})
	if err != nil {
		t.Fatalf("open integration schema: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate integration schema: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
		_ = adminDB.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`).Error
		if sqlDB, dbErr := adminDB.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}
