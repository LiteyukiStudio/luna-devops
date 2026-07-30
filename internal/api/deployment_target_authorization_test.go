package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
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

func TestDataExportTicketIsBoundAndConsumedOnce(t *testing.T) {
	handlers := &Handlers{mode: "test"}
	authorization := testDataExportAuthorization()
	ticket, expiresAt, err := handlers.issueDataExportTicket(context.Background(), authorization)
	if err != nil {
		t.Fatalf("issue ticket: %v", err)
	}
	if ticket == "" || !expiresAt.After(time.Now()) {
		t.Fatalf("invalid ticket response: ticket=%q expiresAt=%v", ticket, expiresAt)
	}
	if _, storedAsPlaintext := dataExportMemoryTickets.Load(ticket); storedAsPlaintext {
		t.Fatal("data export ticket was stored in plaintext")
	}
	if _, storedAsHash := dataExportMemoryTickets.Load(hashToken(ticket)); !storedAsHash {
		t.Fatal("data export ticket was not stored by hash")
	}

	value, valid, err := handlers.consumeDataExportTicket(context.Background(), ticket)
	if err != nil || !valid {
		t.Fatalf("consume ticket: valid=%v err=%v", valid, err)
	}
	if !value.matchesResource(authorization.project.ID, authorization.app.ID, authorization.target.ID) {
		t.Fatalf("ticket binding = %#v, want current data export resource", value)
	}
	_, valid, err = handlers.consumeDataExportTicket(context.Background(), ticket)
	if err != nil || valid {
		t.Fatalf("consumed ticket was reusable: valid=%v err=%v", valid, err)
	}
}

func TestDataExportTicketRejectsDifferentBindingAndIsStillConsumed(t *testing.T) {
	handlers := &Handlers{mode: "test"}
	authorization := testDataExportAuthorization()
	ticket, _, err := handlers.issueDataExportTicket(context.Background(), authorization)
	if err != nil {
		t.Fatalf("issue ticket: %v", err)
	}

	value, valid, err := handlers.consumeDataExportTicket(context.Background(), ticket)
	if err != nil || !valid {
		t.Fatalf("consume ticket: valid=%v err=%v", valid, err)
	}
	if value.matchesResource(authorization.project.ID, authorization.app.ID, "dplt_other") {
		t.Fatal("ticket accepted a different target")
	}
	_, valid, err = handlers.consumeDataExportTicket(context.Background(), ticket)
	if err != nil || valid {
		t.Fatalf("binding mismatch did not atomically consume ticket: valid=%v err=%v", valid, err)
	}
}

func TestDataExportTicketRejectsExpiredTicket(t *testing.T) {
	handlers := &Handlers{mode: "test"}
	authorization := testDataExportAuthorization()
	ticket, _, err := handlers.issueDataExportTicket(context.Background(), authorization)
	if err != nil {
		t.Fatalf("issue ticket: %v", err)
	}
	value, found := dataExportMemoryTickets.Load(hashToken(ticket))
	if !found {
		t.Fatal("issued ticket was not stored")
	}
	expired := value.(dataExportTicketValue)
	expired.ExpiresAt = time.Now().Add(-time.Second)
	dataExportMemoryTickets.Store(hashToken(ticket), expired)

	_, valid, err := handlers.consumeDataExportTicket(context.Background(), ticket)
	if err != nil || valid {
		t.Fatalf("expired ticket accepted: valid=%v err=%v", valid, err)
	}
	_, valid, err = handlers.consumeDataExportTicket(context.Background(), ticket)
	if err != nil || valid {
		t.Fatalf("expired ticket was not atomically consumed: valid=%v err=%v", valid, err)
	}
}

func TestDataExportTicketUsesRedisAtomicallyInProduction(t *testing.T) {
	server := miniredis.RunT(t)
	handlers := &Handlers{mode: "production", rateLimiter: newRateLimiter(server.Addr())}
	t.Cleanup(func() { _ = handlers.rateLimiter.redis.Close() })
	authorization := testDataExportAuthorization()
	ticket, _, err := handlers.issueDataExportTicket(context.Background(), authorization)
	if err != nil {
		t.Fatalf("issue Redis ticket: %v", err)
	}
	if len(ticket) < 2 || ticket[:2] != "r_" {
		t.Fatalf("ticket = %q, want Redis-backed prefix", ticket)
	}

	_, valid, err := handlers.consumeDataExportTicket(context.Background(), ticket)
	if err != nil || !valid {
		t.Fatalf("consume Redis ticket: valid=%v err=%v", valid, err)
	}
	_, valid, err = handlers.consumeDataExportTicket(context.Background(), ticket)
	if err != nil || valid {
		t.Fatalf("Redis ticket was reusable: valid=%v err=%v", valid, err)
	}
}

func TestDataExportTicketFailsClosedWithoutRedisInProduction(t *testing.T) {
	handlers := &Handlers{mode: "production"}
	authorization := testDataExportAuthorization()
	if _, _, err := handlers.issueDataExportTicket(context.Background(), authorization); err == nil {
		t.Fatal("expected production ticket issuance without Redis to fail closed")
	}
}

func TestAuthorizeDeploymentTargetDataExportWithBrowserCookie(t *testing.T) {
	fixture := newDataExportAuthorizationFixture(t, true)
	sessionToken, session := fixture.createSession(t)
	fixture.createAssertion(t, session.ID)

	recorder := fixture.authorize(t, "", sessionToken)
	response := requireDataExportTicketResponse(t, recorder)

	value, valid, err := fixture.handlers.consumeDataExportTicket(context.Background(), response.Ticket)
	if err != nil || !valid {
		t.Fatalf("consume browser ticket: valid=%v err=%v", valid, err)
	}
	authorization, ok := fixture.handlers.dataExportAuthorizationFromTicket(context.Background(), value)
	if !ok {
		t.Fatal("browser ticket could not authorize a headerless download")
	}
	if authorization.binding.SubjectID != session.ID || authorization.user.ID != fixture.user.ID {
		t.Fatalf("browser ticket authorization = %#v", authorization.binding)
	}
}

func TestAuthorizeDeploymentTargetDataExportWithLunaCLIOAuth(t *testing.T) {
	fixture := newDataExportAuthorizationFixture(t, false)
	plainToken, grant := fixture.createOAuthToken(t)
	fixture.createAssertion(t, oauthAssertionSubject(grant.ID))

	recorder := fixture.authorize(t, plainToken, "")
	response := requireDataExportTicketResponse(t, recorder)

	// The export request does not need a bearer header. The consumed ticket
	// carries the grant-bound step-up subject and resource authorization.
	value, valid, err := fixture.handlers.consumeDataExportTicket(context.Background(), response.Ticket)
	if err != nil || !valid {
		t.Fatalf("consume OAuth ticket: valid=%v err=%v", valid, err)
	}
	authorization, ok := fixture.handlers.dataExportAuthorizationFromTicket(context.Background(), value)
	if !ok {
		t.Fatal("OAuth ticket could not authorize a headerless download")
	}
	if authorization.binding.SubjectID != oauthAssertionSubject(grant.ID) ||
		!authorization.binding.AssertionRequired {
		t.Fatalf("OAuth ticket authorization = %#v", authorization.binding)
	}
}

func TestAuthorizeDeploymentTargetDataExportRejectsPersonalAccessToken(t *testing.T) {
	fixture := newDataExportAuthorizationFixture(t, false)
	plainToken := "pat_" + randomHex(24)
	if err := fixture.db.Create(&model.AccessToken{
		ID:        "pat_" + randomHex(8),
		UserID:    fixture.user.ID,
		Name:      "data export PAT",
		Scope:     "deployment:data_export",
		TokenHash: hashToken(plainToken),
		Source:    "personal",
	}).Error; err != nil {
		t.Fatal(err)
	}

	recorder := fixture.authorize(t, plainToken, "")
	requireDataExportErrorCode(t, recorder, http.StatusForbidden, "mfa.session_required")
}

func TestAuthorizeDeploymentTargetDataExportOAuthRequiresAssertion(t *testing.T) {
	fixture := newDataExportAuthorizationFixture(t, false)
	plainToken, _ := fixture.createOAuthToken(t)

	recorder := fixture.authorize(t, plainToken, "")
	requireDataExportErrorCode(t, recorder, http.StatusForbidden, "mfa_required")
}

func testDataExportAuthorization() deploymentTargetDataExportAuthorization {
	return deploymentTargetDataExportAuthorization{
		user:    model.User{ID: "usr_export"},
		project: model.Project{ID: "prj_export"},
		app:     model.Application{ID: "app_export"},
		target:  model.DeploymentTarget{ID: "dplt_export"},
		binding: dataExportAuthorizationBinding{
			UserID:    "usr_export",
			SubjectID: "ses_export",
			Deadline:  time.Now().Add(time.Hour),
		},
	}
}

type dataExportAuthorizationFixture struct {
	db       *gorm.DB
	handlers *Handlers
	user     model.User
	project  model.Project
	app      model.Application
	target   model.DeploymentTarget
}

func newDataExportAuthorizationFixture(t *testing.T, stepUpEnabled bool) dataExportAuthorizationFixture {
	t.Helper()
	db := newMFAIntegrationDB(t)
	if err := db.AutoMigrate(
		&model.Project{},
		&model.ProjectMember{},
		&model.Application{},
		&model.DeploymentTarget{},
	); err != nil {
		t.Fatalf("migrate data export fixture: %v", err)
	}
	suffix := randomHex(8)
	user := model.User{
		ID:       "usr_export_" + suffix,
		Email:    "data-export-" + suffix + "@example.test",
		Name:     "Data Export User",
		Role:     authz.PlatformRoleUser,
		Language: "zh-CN",
	}
	project := model.Project{
		ID:                "prj_export_" + suffix,
		Identifier:        "export-" + suffix,
		Name:              "Data Export Project",
		NamespaceStrategy: "project",
		DeleteStatus:      "active",
	}
	app := model.Application{
		ID:           "app_export_" + suffix,
		ProjectID:    project.ID,
		Identifier:   "export-" + suffix,
		Name:         "Data Export App",
		DeleteStatus: "active",
	}
	target := model.DeploymentTarget{
		ID:                   "dplt_export_" + suffix,
		ProjectID:            project.ID,
		ApplicationID:        app.ID,
		Name:                 "Data Export Target",
		DeleteStatus:         "active",
		DataRetentionEnabled: true,
	}
	enabled := "false"
	if stepUpEnabled {
		enabled = "true"
	}
	for _, value := range []any{
		&user,
		&project,
		&model.ProjectMember{
			ID:        "pm_export_" + suffix,
			ProjectID: project.ID,
			UserID:    user.ID,
			Role:      authz.ProjectRoleOwner,
		},
		&app,
		&target,
		&model.AppConfig{Key: "security.stepUpMfa.enabled", Value: enabled},
	} {
		if err := db.Create(value).Error; err != nil {
			t.Fatalf("create data export fixture %T: %v", value, err)
		}
	}
	return dataExportAuthorizationFixture{
		db: db,
		handlers: &Handlers{
			db:          db,
			configs:     newConfigCache(db),
			mode:        "development",
			rateLimiter: newRateLimiter(),
		},
		user: user, project: project, app: app, target: target,
	}
}

func (fixture dataExportAuthorizationFixture) createSession(t *testing.T) (string, model.UserSession) {
	t.Helper()
	plainToken := "session_" + randomHex(24)
	session := model.UserSession{
		ID:        "ses_export_" + randomHex(8),
		UserID:    fixture.user.ID,
		TokenHash: hashToken(plainToken),
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := fixture.db.Create(&session).Error; err != nil {
		t.Fatal(err)
	}
	return plainToken, session
}

func (fixture dataExportAuthorizationFixture) createOAuthToken(t *testing.T) (string, model.OAuthGrant) {
	t.Helper()
	application := model.OAuthApplication{
		ID:               lunaCLIApplicationID,
		Name:             "Luna CLI",
		ClientID:         lunaCLIClientID,
		ClientSecretHash: "",
		RedirectURIs:     "",
		AllowedScopes:    "deployment:data_export",
	}
	grant := model.OAuthGrant{
		ID:            "oag_export_" + randomHex(8),
		ApplicationID: application.ID,
		UserID:        fixture.user.ID,
		Scope:         "deployment:data_export",
	}
	plainToken := "oauth_" + randomHex(24)
	expiresAt := time.Now().Add(time.Hour)
	token := model.AccessToken{
		ID:                 "atk_export_" + randomHex(8),
		UserID:             fixture.user.ID,
		Name:               "Luna CLI data export",
		Scope:              "deployment:data_export",
		TokenHash:          hashToken(plainToken),
		Source:             "oauth",
		OAuthApplicationID: application.ID,
		OAuthGrantID:       grant.ID,
		ExpiresAt:          &expiresAt,
	}
	for _, value := range []any{&application, &grant, &token} {
		if err := fixture.db.Create(value).Error; err != nil {
			t.Fatalf("create OAuth fixture %T: %v", value, err)
		}
	}
	return plainToken, grant
}

func (fixture dataExportAuthorizationFixture) createAssertion(t *testing.T, subject string) {
	t.Helper()
	now := time.Now()
	assertion := model.StepUpAssertion{
		ID:                "mfaas_export_" + randomHex(8),
		UserID:            fixture.user.ID,
		SessionID:         subject,
		Purpose:           stepUpPurposeDataExport,
		VerifiedAt:        now,
		LastActivityAt:    now,
		IdleExpiresAt:     now.Add(10 * time.Minute),
		AbsoluteExpiresAt: now.Add(time.Hour),
	}
	if err := fixture.db.Create(&assertion).Error; err != nil {
		t.Fatal(err)
	}
}

func (fixture dataExportAuthorizationFixture) authorize(t *testing.T, bearerToken, sessionToken string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	path := "/api/v1/projects/:projectId/applications/:applicationId/deployment-targets/:targetId/data-export/authorize"
	router.POST(path, fixture.handlers.AuthorizeDeploymentTargetDataExport)
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/projects/"+fixture.project.ID+
			"/applications/"+fixture.app.ID+
			"/deployment-targets/"+fixture.target.ID+
			"/data-export/authorize",
		nil,
	)
	if bearerToken != "" {
		request.Header.Set("Authorization", "Bearer "+bearerToken)
	}
	if sessionToken != "" {
		request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionToken})
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func requireDataExportTicketResponse(t *testing.T, recorder *httptest.ResponseRecorder) dataExportTicketResponse {
	t.Helper()
	if recorder.Code != http.StatusOK {
		t.Fatalf("authorize status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	var response dataExportTicketResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode ticket response: %v", err)
	}
	if response.Ticket == "" || !response.ExpiresAt.After(time.Now()) {
		t.Fatalf("invalid ticket response: %#v", response)
	}
	return response
}

func requireDataExportErrorCode(t *testing.T, recorder *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if recorder.Code != status {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, status, recorder.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if response["code"] != code {
		t.Fatalf("code = %v, want %s; body=%s", response["code"], code, recorder.Body.String())
	}
}
