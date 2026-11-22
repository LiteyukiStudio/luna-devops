package api

import (
	"context"
	"github.com/LiteyukiStudio/devops/internal/api/projectapi"
	transportapi "github.com/LiteyukiStudio/devops/internal/api/transport"
	"net/http"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func TestRuntimeTerminalAuthorizationStateRequiresLiveIdentityAndAuthorization(t *testing.T) {
	now := time.Now()
	binding := runtimeTerminalAuthorizationBinding{UserID: "usr_test", SubjectID: "ses_test", Deadline: now.Add(time.Hour)}
	active := projectapi.ContinuousAuthorizationState{
		Session:              model.UserSession{ID: binding.SubjectID, UserID: binding.UserID, ExpiresAt: now.Add(time.Hour)},
		User:                 model.User{ID: binding.UserID},
		AuthorizationAllowed: true,
	}
	tests := []struct {
		name   string
		mutate func(*projectapi.ContinuousAuthorizationState, *runtimeTerminalAuthorizationBinding)
		want   bool
	}{
		{name: "active", want: true},
		{name: "session removed", mutate: func(state *projectapi.ContinuousAuthorizationState, _ *runtimeTerminalAuthorizationBinding) {
			state.Session = model.UserSession{}
		}},
		{name: "session expired", mutate: func(state *projectapi.ContinuousAuthorizationState, _ *runtimeTerminalAuthorizationBinding) {
			state.Session.ExpiresAt = now
		}},
		{name: "user disabled", mutate: func(state *projectapi.ContinuousAuthorizationState, _ *runtimeTerminalAuthorizationBinding) {
			state.User.Disabled = true
		}},
		{name: "authorization removed", mutate: func(state *projectapi.ContinuousAuthorizationState, _ *runtimeTerminalAuthorizationBinding) {
			state.AuthorizationAllowed = false
		}},
		{name: "ticket deadline reached", mutate: func(_ *projectapi.ContinuousAuthorizationState, binding *runtimeTerminalAuthorizationBinding) {
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
			if got := projectapi.ContinuousAuthorizationStateActive(state, currentBinding, now); got != test.want {
				t.Fatalf("active() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestContinuousAuthorizationStateSupportsOAuthStreamBinding(t *testing.T) {
	now := time.Now()
	appID := "oapp_stream_test"
	grantID := "oagr_test"
	tokenID := "tok_test"
	token := model.AccessToken{
		ID: tokenID, UserID: "usr_test", Source: "oauth", OAuthApplicationID: appID,
		OAuthGrantID: grantID, OAuthFamilyID: "ofam_test",
	}
	binding := continuousAuthorizationBindingForAccessToken(token.UserID, token)
	binding.Deadline = now.Add(time.Hour)
	state := projectapi.ContinuousAuthorizationState{
		AccessToken:      token,
		OAuthGrant:       model.OAuthGrant{ID: grantID, ApplicationID: appID, UserID: binding.UserID},
		OAuthApplication: model.OAuthApplication{ID: appID},
		User:             model.User{ID: binding.UserID}, AuthorizationAllowed: true,
	}
	if !projectapi.ContinuousAuthorizationStateActive(state, binding, now) {
		t.Fatal("active OAuth grant should keep continuous stream authorization active")
	}
	state.OAuthGrant.RevokedAt = &now
	if projectapi.ContinuousAuthorizationStateActive(state, binding, now) {
		t.Fatal("revoked OAuth grant must revoke continuous stream authorization")
	}
}

func TestOAuthFamilyRevocationInvalidatesStreamAndInteractiveTerminalRejectsBearer(t *testing.T) {
	db := authIntegrationDB(t)
	now := time.Now()
	plainToken := "lyo_family_bound_identity"
	user := model.User{ID: "usr_family_bound_identity", Email: "family-bound@example.com", Name: "Family Bound", Role: authz.PlatformRoleAdmin, Language: "en-US"}
	application := model.OAuthApplication{
		ID: "oapp_family_bound", Name: "OAuth Client", ClientID: "family-bound-client",
		ClientSecretHash: transportapi.HashToken("family-bound-secret"),
		RedirectURIs:     "[]", AllowedScopes: "user:read", AccessTokenLifetimeDays: 30,
	}
	grant := model.OAuthGrant{ID: "ogrt_family_bound_identity", ApplicationID: application.ID, UserID: user.ID, Scope: "user:read"}
	accessToken := model.AccessToken{
		ID: "tok_family_bound_identity", UserID: user.ID, Name: application.Name, Scope: "user:read", TokenHash: transportapi.HashToken(plainToken),
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
	handlers.domains = newDomainHandlers(handlers)
	binding := continuousAuthorizationBindingForAccessToken(user.ID, accessToken)
	binding.Deadline = now.Add(time.Hour)
	if !handlers.continuousAuthorizationActive(t.Context(), binding, func(context.Context, model.User) bool { return true }) {
		t.Fatal("live OAuth family should authorize continuous stream")
	}
	router := gin.New()
	router.GET("/api/v1/users/me", func(ctx *gin.Context) {
		current, ok := handlers.currentUserFromAccessToken(ctx)
		if !ok {
			return
		}
		if _, ok := handlers.domains.runtime.CurrentInteractiveSubject(ctx, current); ok {
			t.Fatal("bearer token must not be accepted as an interactive terminal subject")
		}
		ctx.Status(http.StatusForbidden)
	})
	bearerInteractive := performBearerRequest(router, http.MethodGet, "/api/v1/users/me", plainToken, "")
	if bearerInteractive.Code != http.StatusForbidden {
		t.Fatalf("bearer interactive terminal check = %d %q, want 403", bearerInteractive.Code, bearerInteractive.Body.String())
	}

	if err := db.Transaction(func(tx *gorm.DB) error {
		return revokeOAuthFamily(tx, grant.ID, accessToken.OAuthFamilyID, time.Now())
	}); err != nil {
		t.Fatal(err)
	}
	if handlers.continuousAuthorizationActive(t.Context(), binding, func(context.Context, model.User) bool { return true }) {
		t.Fatal("revoked OAuth family must stop continuous authorization")
	}
	revokedIdentity := performBearerRequest(router, http.MethodGet, "/api/v1/users/me", plainToken, "")
	if revokedIdentity.Code != http.StatusUnauthorized {
		t.Fatalf("revoked OAuth family bearer check = %d %s", revokedIdentity.Code, revokedIdentity.Body.String())
	}
}

func TestRuntimeTerminalAuthorizationRevokesDeletedSession(t *testing.T) {
	db := authIntegrationDB(t)
	now := time.Now()
	user := model.User{ID: "usr_terminal_monitor", Email: "terminal-monitor@example.com", Name: "Terminal Monitor", Role: authz.PlatformRoleAdmin, Language: "en-US"}
	session := model.UserSession{ID: "ses_terminal_monitor", UserID: user.ID, TokenHash: transportapi.HashToken("terminal-monitor"), ExpiresAt: now.Add(time.Hour)}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatal(err)
	}
	binding := runtimeTerminalAuthorizationBinding{UserID: user.ID, SubjectID: session.ID, Deadline: session.ExpiresAt}
	handlers := &Handlers{db: db}
	handlers.domains = newDomainHandlers(handlers)
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
