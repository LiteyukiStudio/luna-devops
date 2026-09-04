package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/api/platformapi"
	transportapi "github.com/LiteyukiStudio/devops/internal/api/transport"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/retention"
	"github.com/gin-gonic/gin"
)

func TestParseDataRetentionRangeRequiresRFC3339OrderedBounds(t *testing.T) {
	input := platformapi.DataRetentionRequest{
		Datasets: []string{"platform_events"},
		StartAt:  "2026-07-01T00:00:00Z",
		EndAt:    "2026-07-02T00:00:00+08:00",
	}
	parsed, err := platformapi.ParseDataRetentionRange(input)
	if err != nil {
		t.Fatalf("parse valid range: %v", err)
	}
	if len(parsed.Datasets) != 1 || parsed.Datasets[0] != "platform_events" {
		t.Fatalf("datasets = %#v", parsed.Datasets)
	}
	if !parsed.StartAt.Equal(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("startAt = %s", parsed.StartAt)
	}

	for _, invalid := range []platformapi.DataRetentionRequest{
		{StartAt: "2026-07-01", EndAt: "2026-07-02T00:00:00Z"},
		{StartAt: "2026-07-01T00:00:00Z", EndAt: "not-a-time"},
		{StartAt: "2026-07-02T00:00:00Z", EndAt: "2026-07-01T00:00:00Z"},
		{StartAt: "2026-07-01T00:00:00Z", EndAt: "2026-07-01T00:00:00Z"},
	} {
		if _, err := platformapi.ParseDataRetentionRange(invalid); err == nil {
			t.Fatalf("expected range to be rejected: %#v", invalid)
		}
	}
}

func TestDataRetentionErrorsUseStableCodes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		err  error
		code string
	}{
		{err: retention.ErrInvalidRange, code: "retention.invalid_range"},
		{err: retention.ErrUnknownDataset, code: "retention.invalid_dataset"},
		{err: retention.ErrNoDatasets, code: "retention.invalid_dataset"},
		{err: errors.New("database unavailable"), code: "retention.cleanup_failed"},
	}
	for _, test := range tests {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		platformapi.WriteDataRetentionError(ctx, test.err)
		if code := jsonString(t, recorder.Body.Bytes(), "code"); code != test.code {
			t.Fatalf("error %v code = %q, want %q", test.err, code, test.code)
		}
	}
}

func TestDataRetentionAuditSummariesDoNotIncludeServiceErrors(t *testing.T) {
	secret := errors.New("raw build log content must not be audited")
	if summary := platformapi.DataRetentionFailureSummary(secret); summary != "cleanup failed" {
		t.Fatalf("failure summary = %q", summary)
	}
	items := []retention.Result{
		{Dataset: retention.DatasetBuildLogs, Matched: 12, Deleted: 10},
		{Dataset: retention.DatasetReleaseLogs, Matched: 3, Deleted: 3},
	}
	if summary := platformapi.DataRetentionResultSummary(items); summary != "datasets=2 matched=15 deleted=13" {
		t.Fatalf("result summary = %q", summary)
	}
}

func TestDataRetentionEndpointsRequirePlatformAdmin(t *testing.T) {
	db := authIntegrationDB(t)
	now := time.Now()
	user := model.User{ID: "usr_retention_guard", Email: "retention-guard@example.com", Name: "Retention Guard", Role: authz.PlatformRoleUser, Language: "en-US"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	sessionToken := "sess_retention_guard"
	if err := db.Create(&model.UserSession{ID: "ses_retention_guard", UserID: user.ID, TokenHash: transportapi.HashToken(sessionToken), ExpiresAt: now.Add(time.Hour)}).Error; err != nil {
		t.Fatal(err)
	}
	handlers := &Handlers{db: db, configs: newConfigCache(db), mode: "development"}
	handlers.domains = newDomainHandlers(handlers)
	router := NewRouter(db, mustTestConfig(t))

	tests := []struct {
		method  string
		path    string
		handler func(ctx *gin.Context)
	}{
		{method: http.MethodGet, path: "/api/v1/data-retention/catalog", handler: handlers.domains.platform.ListDataRetentionCatalog},
		{method: http.MethodPost, path: "/api/v1/data-retention/preview", handler: handlers.domains.platform.PreviewDataRetention},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			recorder, ctx := newAPIIntegrationContext(test.method, test.path, nil, sessionToken)
			test.handler(ctx)
			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
		})
	}
	cleanup := performCookieJSONRequest(router, http.MethodPost, "/api/v1/data-retention/cleanup", sessionToken, `{}`)
	if cleanup.Code != http.StatusForbidden {
		t.Fatalf("cleanup status = %d, body = %s", cleanup.Code, cleanup.Body.String())
	}
}

func TestDataRetentionRoutesAreRegistered(t *testing.T) {
	db := authIntegrationDB(t)
	router := NewRouter(db, mustTestConfig(t))
	routes := make(map[string]bool)
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	for _, expected := range []string{
		"GET /api/v1/data-retention/catalog",
		"POST /api/v1/data-retention/preview",
		"POST /api/v1/data-retention/cleanup",
	} {
		if !routes[expected] {
			t.Fatalf("route %q is not registered", expected)
		}
	}
}
