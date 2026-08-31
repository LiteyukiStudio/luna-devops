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
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func TestRuntimeTerminalBearerTransportRequiresOneTimeTicket(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/runtime/clusters/rcl_test/pods/terminal", nil)
	ctx.Request.Header.Set("Authorization", "Bearer lyo_test")

	if requireRuntimeTerminalTicketForBearer(ctx, "") {
		t.Fatal("bearer WebSocket transport without a one-time ticket was accepted")
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusForbidden || response["code"] != "runtime_terminal.ticket_required" {
		t.Fatalf("missing bearer terminal ticket = status %d, body %#v", recorder.Code, response)
	}

	withTicketRecorder := httptest.NewRecorder()
	withTicket, _ := gin.CreateTestContext(withTicketRecorder)
	withTicket.Request = httptest.NewRequest(http.MethodGet, "/api/v1/runtime/clusters/rcl_test/pods/terminal?ticket=ticket_test", nil)
	withTicket.Request.Header.Set("Authorization", "Bearer lyo_test")
	if !requireRuntimeTerminalTicketForBearer(withTicket, "ticket_test") {
		t.Fatal("bearer WebSocket transport with a one-time ticket was rejected")
	}

	browserRecorder := httptest.NewRecorder()
	browser, _ := gin.CreateTestContext(browserRecorder)
	browser.Request = httptest.NewRequest(http.MethodGet, "/api/v1/runtime/clusters/rcl_test/pods/terminal", nil)
	if !requireRuntimeTerminalTicketForBearer(browser, "") {
		t.Fatal("browser session-cookie WebSocket flow should remain available without a ticket")
	}
}

func TestRuntimeTerminalAuthorizationStateRequiresLiveIdentityAndAuthorization(t *testing.T) {
	now := time.Now()
	binding := runtimeTerminalAuthorizationBinding{UserID: "usr_test", SubjectID: "ses_test", Deadline: now.Add(time.Hour)}
	active := runtimeTerminalAuthorizationState{
		Session:              model.UserSession{ID: binding.SubjectID, UserID: binding.UserID, ExpiresAt: now.Add(time.Hour)},
		User:                 model.User{ID: binding.UserID},
		AuthorizationAllowed: true,
	}
	tests := []struct {
		name   string
		mutate func(*runtimeTerminalAuthorizationState, *runtimeTerminalAuthorizationBinding)
		want   bool
	}{
		{name: "active", want: true},
		{name: "session removed", mutate: func(state *runtimeTerminalAuthorizationState, _ *runtimeTerminalAuthorizationBinding) {
			state.Session = model.UserSession{}
		}},
		{name: "session expired", mutate: func(state *runtimeTerminalAuthorizationState, _ *runtimeTerminalAuthorizationBinding) {
			state.Session.ExpiresAt = now
		}},
		{name: "user disabled", mutate: func(state *runtimeTerminalAuthorizationState, _ *runtimeTerminalAuthorizationBinding) {
			state.User.Disabled = true
		}},
		{name: "authorization removed", mutate: func(state *runtimeTerminalAuthorizationState, _ *runtimeTerminalAuthorizationBinding) {
			state.AuthorizationAllowed = false
		}},
		{name: "ticket deadline reached", mutate: func(_ *runtimeTerminalAuthorizationState, binding *runtimeTerminalAuthorizationBinding) {
			binding.Deadline = now
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := active
			currentBinding := binding
			if test.mutate != nil {
				test.mutate(&state, &currentBinding)
			}
			if got := state.active(currentBinding, now); got != test.want {
				t.Fatalf("active() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestRuntimeTerminalAuthorizationStateSupportsLunaCLIOAuth(t *testing.T) {
	now := time.Now()
	grantID := "oagr_test"
	tokenID := "tok_test"
	token := model.AccessToken{
		ID: tokenID, UserID: "usr_test", Source: "oauth", OAuthApplicationID: lunaCLIApplicationID,
		OAuthGrantID: grantID, OAuthFamilyID: "ofam_test",
	}
	binding := continuousAuthorizationBindingForAccessToken(token.UserID, token)
	binding.Deadline = now.Add(time.Hour)
	state := runtimeTerminalAuthorizationState{
		AccessToken:      token,
		OAuthGrant:       model.OAuthGrant{ID: grantID, ApplicationID: lunaCLIApplicationID, UserID: binding.UserID},
		OAuthApplication: model.OAuthApplication{ID: lunaCLIApplicationID},
		User:             model.User{ID: binding.UserID}, AuthorizationAllowed: true,
	}
	if !state.active(binding, now) {
		t.Fatal("active Luna CLI OAuth grant should authorize the terminal")
	}
	state.OAuthGrant.RevokedAt = &now
	if state.active(binding, now) {
		t.Fatal("revoked Luna CLI OAuth grant must revoke terminal authorization")
	}
}

func TestOAuthFamilyRevocationInvalidatesTerminalAndDownloadIdentity(t *testing.T) {
	db := authIntegrationDB(t)
	now := time.Now()
	plainToken := "lyo_family_bound_identity"
	user := model.User{ID: "usr_family_bound_identity", Email: "family-bound@example.com", Name: "Family Bound", Role: authz.PlatformRoleAdmin, Language: "en-US"}
	application := model.OAuthApplication{
		ID: lunaCLIApplicationID, Name: "Luna CLI", ClientID: lunaCLIClientID, RedirectURIs: "", AllowedScopes: "user:read", AccessTokenLifetimeDays: 30,
	}
	grant := model.OAuthGrant{ID: "ogrt_family_bound_identity", ApplicationID: application.ID, UserID: user.ID, Scope: "user:read"}
	accessToken := model.AccessToken{
		ID: "tok_family_bound_identity", UserID: user.ID, Name: application.Name, Scope: "user:read", TokenHash: hashToken(plainToken),
		Source: "oauth", OAuthApplicationID: application.ID, OAuthGrantID: grant.ID, OAuthFamilyID: "ofam_family_bound_identity",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&application).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&grant).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&accessToken).Error; err != nil {
		t.Fatal(err)
	}

	handlers := &Handlers{db: db}
	binding := continuousAuthorizationBindingForAccessToken(user.ID, accessToken)
	binding.Deadline = now.Add(time.Hour)
	if !handlers.continuousAuthorizationActive(t.Context(), binding, func(context.Context, model.User) bool { return true }) {
		t.Fatal("live OAuth family should authorize the existing terminal")
	}
	router := gin.New()
	router.GET("/api/v1/users/me", func(ctx *gin.Context) {
		current, ok := handlers.currentUserFromAccessToken(ctx)
		if !ok {
			return
		}
		subject, ok := handlers.currentInteractiveSubject(ctx, current)
		if !ok {
			ctx.Status(http.StatusForbidden)
			return
		}
		ctx.String(http.StatusOK, subject)
	})
	liveIdentity := performBearerRequest(router, http.MethodGet, "/api/v1/users/me", plainToken, "")
	if liveIdentity.Code != http.StatusOK || liveIdentity.Body.String() != binding.SubjectID {
		t.Fatalf("live download identity = %d %q", liveIdentity.Code, liveIdentity.Body.String())
	}

	if err := db.Transaction(func(tx *gorm.DB) error {
		return revokeOAuthFamily(tx, grant.ID, accessToken.OAuthFamilyID, time.Now())
	}); err != nil {
		t.Fatal(err)
	}
	if handlers.continuousAuthorizationActive(t.Context(), binding, func(context.Context, model.User) bool { return true }) {
		t.Fatal("revoked OAuth family must stop the existing terminal")
	}
	revokedIdentity := performBearerRequest(router, http.MethodGet, "/api/v1/users/me", plainToken, "")
	if revokedIdentity.Code != http.StatusUnauthorized {
		t.Fatalf("revoked OAuth family download identity = %d %s", revokedIdentity.Code, revokedIdentity.Body.String())
	}
}

func TestRuntimeClusterPodTerminalTicketIsOneTimeAndResourceBound(t *testing.T) {
	handlers := &Handlers{mode: "test"}
	now := time.Now()
	binding := runtimeTerminalAuthorizationBinding{UserID: "usr_runtime_pod_ticket", SubjectID: "ses_runtime_pod_ticket", Deadline: now.Add(time.Hour)}
	reference := runtimeClusterPodTerminalAuthorizationReference{
		ClusterID: "rcl_runtime_pod_ticket", ClusterKubeconfig: "sec_runtime_pod_ticket", Namespace: "ns-runtime-pod-ticket",
		Name: "pod-runtime-pod-ticket", ProjectID: "prj_runtime_pod_ticket", ApplicationID: "app_runtime_pod_ticket",
		DeploymentTargetID: "dplt_runtime_pod_ticket", ReleaseID: "rel_runtime_pod_ticket",
	}
	ticket, expiresAt, err := handlers.issueRuntimeTerminalTicket(context.Background(), binding, "runtime_pod", reference)
	if err != nil {
		t.Fatal(err)
	}
	if expiresAt.Before(now.Add(runtimeTerminalTicketTTL-time.Second)) || expiresAt.After(now.Add(runtimeTerminalTicketTTL+time.Second)) {
		t.Fatalf("ticket expiry %s is outside the expected short TTL", expiresAt)
	}
	if _, found := runtimeTerminalMemoryTickets.Load(ticket); found {
		t.Fatal("raw terminal ticket must not be stored")
	}
	if _, found := runtimeTerminalMemoryTickets.Load(hashToken(ticket)); !found {
		t.Fatal("hashed terminal ticket was not stored")
	}
	value, ok, err := handlers.consumeRuntimeTerminalTicket(context.Background(), ticket)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || value.UserID != binding.UserID || value.Authorization.SubjectID != binding.SubjectID {
		t.Fatalf("consumed ticket is not bound to the expected user/session: %#v", value)
	}
	if !value.matches("runtime_pod", reference) {
		t.Fatal("ticket did not match its runtime Pod resource")
	}
	otherReference := reference
	otherReference.Name = "another-pod"
	if value.matches("runtime_pod", otherReference) {
		t.Fatal("ticket must not match another runtime Pod")
	}
	if _, ok, err := handlers.consumeRuntimeTerminalTicket(context.Background(), ticket); err != nil || ok {
		t.Fatalf("consuming a terminal ticket twice = (%v, %v), want (false, nil)", ok, err)
	}
}

func TestRuntimeTerminalAuthorizationRevokesDeletedSession(t *testing.T) {
	db := authIntegrationDB(t)
	now := time.Now()
	user := model.User{ID: "usr_terminal_monitor", Email: "terminal-monitor@example.com", Name: "Terminal Monitor", Role: authz.PlatformRoleAdmin, Language: "en-US"}
	session := model.UserSession{ID: "ses_terminal_monitor", UserID: user.ID, TokenHash: hashToken("terminal-monitor"), ExpiresAt: now.Add(time.Hour)}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatal(err)
	}
	binding := runtimeTerminalAuthorizationBinding{UserID: user.ID, SubjectID: session.ID, Deadline: session.ExpiresAt}
	handlers := &Handlers{db: db}
	if !handlers.continuousAuthorizationActive(context.Background(), binding, func(context.Context, model.User) bool { return true }) {
		t.Fatal("live session should authorize the terminal")
	}
	if err := db.Delete(&session).Error; err != nil {
		t.Fatal(err)
	}
	if handlers.continuousAuthorizationActive(context.Background(), binding, func(context.Context, model.User) bool { return true }) {
		t.Fatal("deleted session must revoke terminal authorization")
	}
}
