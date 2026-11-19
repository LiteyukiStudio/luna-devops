package api

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	transportapi "github.com/LiteyukiStudio/devops/internal/api/transport"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
)

type oauthFamilyTestFixture struct {
	db           *gorm.DB
	router       http.Handler
	user         model.User
	application  model.OAuthApplication
	clientSecret string
	redirectURI  string
	sessionToken string
}

type issuedOAuthCodeFamily struct {
	tokens       oauthTokenResponse
	accessToken  model.AccessToken
	refreshToken model.OAuthRefreshToken
}

type pendingOAuthAuthorizationCode struct {
	application  model.OAuthApplication
	clientSecret string
	redirectURI  string
	verifier     string
	code         string
}

func TestConcurrentOAuthCodeExchangesShareGrantAndSeparateFamilies(t *testing.T) {
	fixture := newOAuthFamilyTestFixture(t)
	first := startFixtureAuthorizationCode(t, fixture, "user:read")
	second := startFixtureAuthorizationCode(t, fixture, "user:read")

	start := make(chan struct{})
	responses := make(chan *httptest.ResponseRecorder, 2)
	var workers sync.WaitGroup
	for _, pending := range []pendingOAuthAuthorizationCode{first, second} {
		workers.Add(1)
		go func(p pendingOAuthAuthorizationCode) {
			defer workers.Done()
			<-start
			responses <- exchangeOAuthAuthorizationCode(fixture, p)
		}(pending)
	}
	close(start)
	workers.Wait()
	close(responses)
	for response := range responses {
		if response.Code != http.StatusOK {
			t.Fatalf("concurrent code exchange = %d %s", response.Code, response.Body.String())
		}
	}

	var grantCount int64
	if err := fixture.db.Model(&model.OAuthGrant{}).
		Where("application_id = ? and user_id = ? and revoked_at is null", fixture.application.ID, fixture.user.ID).
		Count(&grantCount).Error; err != nil {
		t.Fatal(err)
	}
	if grantCount != 1 {
		t.Fatalf("active consent grant count = %d, want 1", grantCount)
	}
	var families []string
	if err := fixture.db.Model(&model.AccessToken{}).
		Where("oauth_application_id = ? and user_id = ?", fixture.application.ID, fixture.user.ID).
		Distinct("oauth_family_id").Pluck("oauth_family_id", &families).Error; err != nil {
		t.Fatal(err)
	}
	if len(families) != 2 || families[0] == "" || families[1] == "" || families[0] == families[1] {
		t.Fatalf("issued OAuth families = %#v", families)
	}
}

func TestOAuthRefreshReplayRevokesOnlyCompromisedFamily(t *testing.T) {
	fixture := newOAuthFamilyTestFixture(t)
	compromised := issueOAuthCodeFamily(t, fixture)
	unrelated := issueOAuthCodeFamily(t, fixture)
	rotated := refreshOAuthFamily(t, fixture, compromised.tokens.RefreshToken)

	replay := performFormRequest(fixture.router, http.MethodPost, "/api/v1/oauth/token", url.Values{
		"client_id":     {fixture.application.ClientID},
		"client_secret": {fixture.clientSecret},
		"grant_type":    {"refresh_token"},
		"refresh_token": {compromised.tokens.RefreshToken},
	})
	if replay.Code != http.StatusBadRequest || jsonString(t, replay.Body.Bytes(), "error") != "invalid_grant" {
		t.Fatalf("refresh replay = %d %s", replay.Code, replay.Body.String())
	}
	if current := performBearerRequest(fixture.router, http.MethodGet, "/api/v1/users/me", rotated.AccessToken, ""); current.Code != http.StatusUnauthorized {
		t.Fatalf("compromised family access remains valid = %d %s", current.Code, current.Body.String())
	}
	if current := performBearerRequest(fixture.router, http.MethodGet, "/api/v1/users/me", unrelated.tokens.AccessToken, ""); current.Code != http.StatusOK {
		t.Fatalf("unrelated family was revoked = %d %s", current.Code, current.Body.String())
	}
	var grant model.OAuthGrant
	if err := fixture.db.First(&grant, "id = ?", unrelated.accessToken.OAuthGrantID).Error; err != nil {
		t.Fatal(err)
	}
	if grant.RevokedAt != nil {
		t.Fatal("refresh replay revoked the consent grant")
	}
}

func TestOAuthTokenRevocationRevokesOnlyCurrentFamily(t *testing.T) {
	fixture := newOAuthFamilyTestFixture(t)
	revoked := issueOAuthCodeFamily(t, fixture)
	unrelated := issueOAuthCodeFamily(t, fixture)

	revoke := performFormRequest(fixture.router, http.MethodPost, "/api/v1/oauth/revoke", url.Values{
		"client_id":     {fixture.application.ClientID},
		"client_secret": {fixture.clientSecret},
		"token":         {revoked.tokens.AccessToken},
	})
	if revoke.Code != http.StatusOK {
		t.Fatalf("revoke OAuth family = %d %s", revoke.Code, revoke.Body.String())
	}
	if current := performBearerRequest(fixture.router, http.MethodGet, "/api/v1/users/me", revoked.tokens.AccessToken, ""); current.Code != http.StatusUnauthorized {
		t.Fatalf("revoked family access remains valid = %d %s", current.Code, current.Body.String())
	}
	if refresh := performFormRequest(fixture.router, http.MethodPost, "/api/v1/oauth/token", url.Values{
		"client_id":     {fixture.application.ClientID},
		"client_secret": {fixture.clientSecret},
		"grant_type":    {"refresh_token"},
		"refresh_token": {revoked.tokens.RefreshToken},
	}); refresh.Code != http.StatusBadRequest || jsonString(t, refresh.Body.Bytes(), "error") != "invalid_grant" {
		t.Fatalf("revoked family refresh = %d %s", refresh.Code, refresh.Body.String())
	}
	if current := performBearerRequest(fixture.router, http.MethodGet, "/api/v1/users/me", unrelated.tokens.AccessToken, ""); current.Code != http.StatusOK {
		t.Fatalf("unrelated family was revoked = %d %s", current.Code, current.Body.String())
	}
	refreshOAuthFamily(t, fixture, unrelated.tokens.RefreshToken)

	var grant model.OAuthGrant
	if err := fixture.db.First(&grant, "id = ?", unrelated.accessToken.OAuthGrantID).Error; err != nil {
		t.Fatal(err)
	}
	if grant.RevokedAt != nil {
		t.Fatal("token revocation revoked the consent grant")
	}
}

func TestOAuthRefreshRotationLinearizesWithRevocation(t *testing.T) {
	tests := []struct {
		name   string
		revoke func(*gorm.DB, oauthFamilyTestFixture, issuedOAuthCodeFamily) error
	}{
		{
			name: "family",
			revoke: func(db *gorm.DB, _ oauthFamilyTestFixture, issued issuedOAuthCodeFamily) error {
				return db.Transaction(func(tx *gorm.DB) error {
					return revokeOAuthFamily(tx, issued.accessToken.OAuthGrantID, issued.accessToken.OAuthFamilyID, time.Now())
				})
			},
		},
		{
			name: "grant",
			revoke: func(db *gorm.DB, _ oauthFamilyTestFixture, issued issuedOAuthCodeFamily) error {
				return db.Transaction(func(tx *gorm.DB) error {
					return revokeOAuthGrant(tx, issued.accessToken.OAuthGrantID, time.Now())
				})
			},
		},
		{
			name: "application",
			revoke: func(db *gorm.DB, fixture oauthFamilyTestFixture, _ issuedOAuthCodeFamily) error {
				return db.Transaction(func(tx *gorm.DB) error {
					return revokeOAuthApplication(tx, fixture.application.ID, time.Now())
				})
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newOAuthFamilyTestFixture(t)
			issued := issueOAuthCodeFamily(t, fixture)
			accessCreated := make(chan struct{})
			releaseCreate := make(chan struct{})
			var createBarrier sync.Once
			var releaseBarrier sync.Once
			release := func() { releaseBarrier.Do(func() { close(releaseCreate) }) }
			defer release()
			callbackName := "test:oauth_refresh_rotation_" + test.name + "_" + transportapi.RandomHex(4)
			if err := fixture.db.Callback().Create().After("gorm:create").Register(callbackName, func(tx *gorm.DB) {
				if tx.Statement == nil || tx.Statement.Schema == nil || tx.Statement.Schema.Name != "AccessToken" {
					return
				}
				createBarrier.Do(func() {
					close(accessCreated)
					<-releaseCreate
				})
			}); err != nil {
				t.Fatalf("register refresh barrier: %v", err)
			}
			t.Cleanup(func() { _ = fixture.db.Callback().Create().Remove(callbackName) })

			refreshDone := make(chan *httptest.ResponseRecorder, 1)
			go func() {
				refreshDone <- performFormRequest(fixture.router, http.MethodPost, "/api/v1/oauth/token", url.Values{
					"client_id":     {fixture.application.ClientID},
					"client_secret": {fixture.clientSecret},
					"grant_type":    {"refresh_token"},
					"refresh_token": {issued.tokens.RefreshToken},
				})
			}()
			select {
			case <-accessCreated:
			case <-time.After(5 * time.Second):
				t.Fatal("refresh rotation did not reach the access-token insert barrier")
			}

			revokeStarted := make(chan struct{})
			revokeDone := make(chan error, 1)
			go func() {
				close(revokeStarted)
				revokeDone <- test.revoke(fixture.db, fixture, issued)
			}()
			<-revokeStarted
			select {
			case err := <-revokeDone:
				release()
				<-refreshDone
				t.Fatalf("revocation returned before in-flight rotation committed: %v", err)
			case <-time.After(150 * time.Millisecond):
			}

			release()
			var refresh *httptest.ResponseRecorder
			select {
			case refresh = <-refreshDone:
			case <-time.After(5 * time.Second):
				t.Fatal("refresh rotation did not complete")
			}
			if refresh.Code != http.StatusOK {
				t.Fatalf("refresh rotation = %d %s", refresh.Code, refresh.Body.String())
			}
			if err := <-revokeDone; err != nil {
				t.Fatalf("revoke after refresh rotation: %v", err)
			}
			var rotated oauthTokenResponse
			if err := json.Unmarshal(refresh.Body.Bytes(), &rotated); err != nil {
				t.Fatal(err)
			}
			if current := performBearerRequest(fixture.router, http.MethodGet, "/api/v1/users/me", rotated.AccessToken, ""); current.Code != http.StatusUnauthorized {
				t.Fatalf("revocation returned with rotated access token active = %d %s", current.Code, current.Body.String())
			}
			var activeAccess int64
			if err := fixture.db.Model(&model.AccessToken{}).
				Where("oauth_grant_id = ? and oauth_family_id = ? and revoked_at is null", issued.accessToken.OAuthGrantID, issued.accessToken.OAuthFamilyID).
				Count(&activeAccess).Error; err != nil {
				t.Fatal(err)
			}
			var activeRefresh int64
			if err := fixture.db.Model(&model.OAuthRefreshToken{}).
				Where("grant_id = ? and family_id = ? and revoked_at is null", issued.accessToken.OAuthGrantID, issued.accessToken.OAuthFamilyID).
				Count(&activeRefresh).Error; err != nil {
				t.Fatal(err)
			}
			if activeAccess != 0 || activeRefresh != 0 {
				t.Fatalf("active credentials after revoke: access=%d refresh=%d", activeAccess, activeRefresh)
			}
		})
	}
}

func TestOAuthGrantRevocationInvalidatesApprovedPendingAuthorizationCodes(t *testing.T) {
	fixture := newOAuthFamilyTestFixture(t)
	issued := issueOAuthCodeFamily(t, fixture)

	verifier := strings.Repeat("p", 64)
	digest := sha256.Sum256([]byte(verifier))
	plainCode := "lyo_code_pending_" + transportapi.RandomHex(8)
	pendingCode := model.OAuthAuthorizationCode{
		ID: "ocod_pending_" + transportapi.RandomHex(4), ApplicationID: fixture.application.ID, UserID: fixture.user.ID,
		CodeHash: transportapi.HashToken(plainCode), RedirectURI: fixture.redirectURI, Scope: "user:read",
		CodeChallenge: base64.RawURLEncoding.EncodeToString(digest[:]), CodeChallengeMethod: "S256",
		ExpiresAt: time.Now().Add(time.Minute), CreatedAt: time.Now(),
	}
	if err := fixture.db.Create(&pendingCode).Error; err != nil {
		t.Fatal(err)
	}

	revoke := performCookieJSONRequest(
		fixture.router,
		http.MethodDelete,
		"/api/v1/oauth/grants/"+issued.accessToken.OAuthGrantID,
		fixture.sessionToken,
		"",
	)
	if revoke.Code != http.StatusNoContent {
		t.Fatalf("revoke grant with pending consent = %d %s", revoke.Code, revoke.Body.String())
	}
	var storedCode model.OAuthAuthorizationCode
	if err := fixture.db.First(&storedCode, "id = ?", pendingCode.ID).Error; err != nil {
		t.Fatal(err)
	}
	if storedCode.ConsumedAt == nil {
		t.Fatal("grant revoke left approved authorization code pending")
	}
	if _, err := exchangeOAuthAuthorizationCodeValue(
		fixture.db,
		oauthClientAuthentication{
			applicationID:    fixture.application.ID,
			clientID:         fixture.application.ClientID,
			clientSecretHash: fixture.application.ClientSecretHash,
		},
		plainCode,
		pendingCode.RedirectURI,
		verifier,
		time.Now(),
	); err == nil {
		t.Fatal("authorization code exchanged after its consent grant was revoked")
	}
}

func TestOAuthFreshConsentAfterGrantRevocationCreatesNewGrant(t *testing.T) {
	fixture := newOAuthFamilyTestFixture(t)
	first := issueOAuthCodeFamily(t, fixture)
	revoke := performCookieJSONRequest(
		fixture.router,
		http.MethodDelete,
		"/api/v1/oauth/grants/"+first.accessToken.OAuthGrantID,
		fixture.sessionToken,
		"",
	)
	if revoke.Code != http.StatusNoContent {
		t.Fatalf("revoke initial grant = %d %s", revoke.Code, revoke.Body.String())
	}
	time.Sleep(5 * time.Millisecond)
	second := issueOAuthCodeFamily(t, fixture)
	if second.accessToken.OAuthGrantID == first.accessToken.OAuthGrantID {
		t.Fatalf("fresh consent reused revoked grant %q", first.accessToken.OAuthGrantID)
	}
	if second.accessToken.OAuthFamilyID == "" {
		t.Fatal("fresh consent did not issue a token family")
	}
}

func TestOAuthPendingAuthorizationCodeExchangeRevalidatesAuthority(t *testing.T) {
	tests := []struct {
		name       string
		scope      string
		mutate     func(*gorm.DB, pendingOAuthAuthorizationCode, model.User) error
		wantStatus int
	}{
		{
			name:  "application scope shrunk",
			scope: "volume:export",
			mutate: func(db *gorm.DB, pending pendingOAuthAuthorizationCode, _ model.User) error {
				return db.Transaction(func(tx *gorm.DB) error {
					if err := tx.Model(&model.OAuthApplication{}).Where("id = ?", pending.application.ID).Update("allowed_scopes", "user:read").Error; err != nil {
						return err
					}
					return revokeOAuthApplication(tx, pending.application.ID, time.Now())
				})
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:  "application revoked",
			scope: "user:read",
			mutate: func(db *gorm.DB, pending pendingOAuthAuthorizationCode, _ model.User) error {
				return db.Transaction(func(tx *gorm.DB) error {
					now := time.Now()
					if err := tx.Model(&model.OAuthApplication{}).Where("id = ?", pending.application.ID).Update("revoked_at", now).Error; err != nil {
						return err
					}
					return revokeOAuthApplication(tx, pending.application.ID, now)
				})
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:  "user disabled",
			scope: "user:read",
			mutate: func(db *gorm.DB, _ pendingOAuthAuthorizationCode, user model.User) error {
				return db.Model(&model.User{}).Where("id = ?", user.ID).Update("disabled", true).Error
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:  "user role downgraded",
			scope: "user:manage",
			mutate: func(db *gorm.DB, _ pendingOAuthAuthorizationCode, user model.User) error {
				return db.Model(&model.User{}).Where("id = ?", user.ID).Update("role", authz.PlatformRoleUser).Error
			},
			wantStatus: http.StatusBadRequest,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newOAuthFamilyTestFixture(t)
			pending := startOAuthAuthorizationCode(t, fixture, test.scope)
			if err := test.mutate(fixture.db, pending, fixture.user); err != nil {
				t.Fatal(err)
			}
			exchange := exchangeOAuthAuthorizationCode(fixture, pending)
			if exchange.Code != test.wantStatus {
				t.Fatalf("authorization-code exchange after authority change = %d %s, want %d", exchange.Code, exchange.Body.String(), test.wantStatus)
			}
			var activeGrants int64
			if err := fixture.db.Model(&model.OAuthGrant{}).
				Where("application_id = ? and user_id = ? and revoked_at is null", pending.application.ID, fixture.user.ID).
				Count(&activeGrants).Error; err != nil {
				t.Fatal(err)
			}
			if activeGrants != 0 {
				t.Fatalf("authority change authorization-code exchange created %d active grants", activeGrants)
			}
		})
	}
}

func TestUserOAuthGrantRevocationRevokesAllFamilies(t *testing.T) {
	fixture := newOAuthFamilyTestFixture(t)
	first := issueOAuthCodeFamily(t, fixture)
	second := issueOAuthCodeFamily(t, fixture)

	revoke := performCookieJSONRequest(
		fixture.router,
		http.MethodDelete,
		"/api/v1/oauth/grants/"+first.accessToken.OAuthGrantID,
		fixture.sessionToken,
		"",
	)
	if revoke.Code != http.StatusNoContent {
		t.Fatalf("revoke OAuth grant = %d %s", revoke.Code, revoke.Body.String())
	}
	for _, token := range []string{first.tokens.AccessToken, second.tokens.AccessToken} {
		current := performBearerRequest(fixture.router, http.MethodGet, "/api/v1/users/me", token, "")
		if current.Code != http.StatusUnauthorized {
			t.Fatalf("grant-wide revoke left family active = %d %s", current.Code, current.Body.String())
		}
	}
	var grant model.OAuthGrant
	if err := fixture.db.First(&grant, "id = ?", first.accessToken.OAuthGrantID).Error; err != nil {
		t.Fatal(err)
	}
	if grant.RevokedAt == nil {
		t.Fatal("user authorization revoke left consent grant active")
	}
}

func TestOAuthTokenRevocationFailureReturnsServerErrorAndRollsBack(t *testing.T) {
	fixture := newOAuthFamilyTestFixture(t)
	issued := issueOAuthCodeFamily(t, fixture)
	callbackName := "test:oauth_family_revoke_failure"
	if err := fixture.db.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Name == "AccessToken" {
			tx.AddError(errors.New("forced OAuth family revoke failure"))
		}
	}); err != nil {
		t.Fatalf("register update callback: %v", err)
	}
	t.Cleanup(func() { _ = fixture.db.Callback().Update().Remove(callbackName) })

	revoke := performFormRequest(fixture.router, http.MethodPost, "/api/v1/oauth/revoke", url.Values{
		"client_id":     {fixture.application.ClientID},
		"client_secret": {fixture.clientSecret},
		"token":         {issued.tokens.RefreshToken},
	})
	if revoke.Code != http.StatusInternalServerError || jsonString(t, revoke.Body.Bytes(), "error") != "server_error" {
		t.Fatalf("failed token revocation = %d %s", revoke.Code, revoke.Body.String())
	}
	var accessToken model.AccessToken
	if err := fixture.db.First(&accessToken, "id = ?", issued.accessToken.ID).Error; err != nil {
		t.Fatal(err)
	}
	if accessToken.RevokedAt != nil {
		t.Fatal("failed family revocation committed a partial access-token revoke")
	}
}

func TestOAuthAuthorizationCodeCreatesConsentAndFamilyAtExchange(t *testing.T) {
	fixture := newOAuthFamilyTestFixture(t)
	clientID := "oauth-code-client-" + transportapi.RandomHex(4)
	clientSecret := "oauth-code-secret"
	redirectURI := "https://client.example.com/callback"
	application := model.OAuthApplication{
		ID: "oapp_code_" + transportapi.RandomHex(4), Name: "Code Client", ClientID: clientID, ClientSecretHash: transportapi.HashToken(clientSecret),
		RedirectURIs: encodeStringList([]string{redirectURI}), AllowedScopes: "user:read", AccessTokenLifetimeDays: 30,
	}
	if err := fixture.db.Create(&application).Error; err != nil {
		t.Fatal(err)
	}
	verifier := strings.Repeat("v", 64)
	digest := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(digest[:])
	input := oauthAuthorizationDecisionInput{
		Approved: true, ClientID: clientID, RedirectURI: redirectURI, Scope: "user:read", State: "state",
		CodeChallenge: challenge, CodeChallengeMethod: "S256",
	}
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	decision := performCookieJSONRequest(fixture.router, http.MethodPost, "/api/v1/oauth/authorize", fixture.sessionToken, string(body))
	if decision.Code != http.StatusOK {
		t.Fatalf("authorization decision = %d %s", decision.Code, decision.Body.String())
	}
	redirectURL, err := url.Parse(jsonString(t, decision.Body.Bytes(), "redirectUrl"))
	if err != nil {
		t.Fatal(err)
	}
	plainCode := redirectURL.Query().Get("code")
	if plainCode == "" {
		t.Fatal("authorization response has no code")
	}
	var code model.OAuthAuthorizationCode
	if err := fixture.db.First(&code, "code_hash = ?", transportapi.HashToken(plainCode)).Error; err != nil {
		t.Fatal(err)
	}
	if code.GrantID != nil {
		t.Fatalf("authorization approval created grant %q", *code.GrantID)
	}
	var grantCount int64
	if err := fixture.db.Model(&model.OAuthGrant{}).Where("application_id = ? and user_id = ?", application.ID, fixture.user.ID).Count(&grantCount).Error; err != nil {
		t.Fatal(err)
	}
	if grantCount != 0 {
		t.Fatalf("grant count before exchange = %d", grantCount)
	}

	exchange := performFormRequest(fixture.router, http.MethodPost, "/api/v1/oauth/token", url.Values{
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"grant_type":    {"authorization_code"},
		"code":          {plainCode},
		"redirect_uri":  {redirectURI},
		"code_verifier": {verifier},
	})
	if exchange.Code != http.StatusOK {
		t.Fatalf("authorization code exchange = %d %s", exchange.Code, exchange.Body.String())
	}
	var tokens oauthTokenResponse
	if err := json.Unmarshal(exchange.Body.Bytes(), &tokens); err != nil {
		t.Fatal(err)
	}
	if tokens.Scope != "user:read" {
		t.Fatalf("third-party token response scope = %q", tokens.Scope)
	}
	var accessToken model.AccessToken
	if err := fixture.db.First(&accessToken, "token_hash = ?", transportapi.HashToken(tokens.AccessToken)).Error; err != nil {
		t.Fatal(err)
	}
	if accessToken.OAuthGrantID == "" || accessToken.OAuthFamilyID == "" {
		t.Fatalf("issued authorization-code token has incomplete family: %#v", accessToken)
	}
	if err := fixture.db.First(&code, "id = ?", code.ID).Error; err != nil {
		t.Fatal(err)
	}
	if code.GrantID == nil || *code.GrantID != accessToken.OAuthGrantID || code.ConsumedAt == nil {
		t.Fatalf("exchanged authorization code = %#v", code)
	}
}

func newOAuthFamilyTestFixture(t *testing.T) oauthFamilyTestFixture {
	t.Helper()
	db := authIntegrationDB(t)
	t.Setenv("APP_ENV", "development")
	t.Setenv("PUBLIC_BASE_URL", "https://devops.example.com")
	t.Setenv("SECRET_ENCRYPTION_KEY", "oauth-family-integration-key")
	suffix := transportapi.RandomHex(4)
	user := model.User{
		ID: "usr_oauth_family_" + suffix, Email: "oauth-family-" + suffix + "@example.com", Name: "OAuth Family", Role: authz.PlatformRoleAdmin, Language: "zh-CN",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	sessionToken := "sess_oauth_family_" + suffix
	if err := db.Create(&model.UserSession{
		ID: "ses_oauth_family_" + suffix, UserID: user.ID, TokenHash: transportapi.HashToken(sessionToken), ExpiresAt: time.Now().Add(time.Hour),
	}).Error; err != nil {
		t.Fatal(err)
	}
	clientSecret := "oauth-family-client-secret-" + suffix
	redirectURI := "https://client.example.com/callback"
	application := model.OAuthApplication{
		ID:                      "oapp_family_" + suffix,
		Name:                    "Family Test App",
		ClientID:                "family-client-" + suffix,
		RedirectURIs:            encodeStringList([]string{redirectURI}),
		AllowedScopes:           "user:read,project:read",
		AccessTokenLifetimeDays: 30,
		ClientSecretHash:        transportapi.HashToken(clientSecret),
	}
	if err := db.Create(&application).Error; err != nil {
		t.Fatal(err)
	}
	return oauthFamilyTestFixture{
		db:           db,
		router:       NewRouter(db, mustTestConfig(t)),
		user:         user,
		application:  application,
		clientSecret: clientSecret,
		redirectURI:  redirectURI,
		sessionToken: sessionToken,
	}
}

func startFixtureAuthorizationCode(t *testing.T, fixture oauthFamilyTestFixture, scope string) pendingOAuthAuthorizationCode {
	t.Helper()
	verifier := strings.Repeat("f", 64)
	digest := sha256.Sum256([]byte(verifier))
	input := oauthAuthorizationDecisionInput{
		Approved:            true,
		ClientID:            fixture.application.ClientID,
		RedirectURI:         fixture.redirectURI,
		Scope:               scope,
		CodeChallenge:       base64.RawURLEncoding.EncodeToString(digest[:]),
		CodeChallengeMethod: "S256",
	}
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	decision := performCookieJSONRequest(fixture.router, http.MethodPost, "/api/v1/oauth/authorize", fixture.sessionToken, string(body))
	if decision.Code != http.StatusOK {
		t.Fatalf("authorization-code consent = %d %s", decision.Code, decision.Body.String())
	}
	redirectURL, err := url.Parse(jsonString(t, decision.Body.Bytes(), "redirectUrl"))
	if err != nil {
		t.Fatal(err)
	}
	code := redirectURL.Query().Get("code")
	if code == "" {
		t.Fatal("authorization-code consent returned no code")
	}
	return pendingOAuthAuthorizationCode{
		application:  fixture.application,
		clientSecret: fixture.clientSecret,
		redirectURI:  fixture.redirectURI,
		verifier:     verifier,
		code:         code,
	}
}

func issueOAuthCodeFamily(t *testing.T, fixture oauthFamilyTestFixture) issuedOAuthCodeFamily {
	t.Helper()
	pending := startFixtureAuthorizationCode(t, fixture, "user:read")
	exchange := exchangeOAuthAuthorizationCode(fixture, pending)
	if exchange.Code != http.StatusOK {
		t.Fatalf("code token exchange = %d %s", exchange.Code, exchange.Body.String())
	}
	var tokens oauthTokenResponse
	if err := json.Unmarshal(exchange.Body.Bytes(), &tokens); err != nil {
		t.Fatal(err)
	}
	var accessToken model.AccessToken
	if err := fixture.db.First(&accessToken, "token_hash = ?", transportapi.HashToken(tokens.AccessToken)).Error; err != nil {
		t.Fatal(err)
	}
	var refreshToken model.OAuthRefreshToken
	if err := fixture.db.First(&refreshToken, "token_hash = ?", transportapi.HashToken(tokens.RefreshToken)).Error; err != nil {
		t.Fatal(err)
	}
	return issuedOAuthCodeFamily{tokens: tokens, accessToken: accessToken, refreshToken: refreshToken}
}

func startOAuthAuthorizationCode(t *testing.T, fixture oauthFamilyTestFixture, scope string) pendingOAuthAuthorizationCode {
	t.Helper()
	suffix := transportapi.RandomHex(4)
	pending := pendingOAuthAuthorizationCode{
		application: model.OAuthApplication{
			ID: "oapp_pending_code_" + suffix, Name: "Pending Code Client", ClientID: "pending-code-client-" + suffix,
			RedirectURIs: encodeStringList([]string{"https://client.example.com/callback"}), AllowedScopes: scope, AccessTokenLifetimeDays: 30,
		},
		clientSecret: "pending-code-secret-" + suffix,
		redirectURI:  "https://client.example.com/callback",
		verifier:     strings.Repeat("v", 64),
	}
	pending.application.ClientSecretHash = transportapi.HashToken(pending.clientSecret)
	if err := fixture.db.Create(&pending.application).Error; err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(pending.verifier))
	input := oauthAuthorizationDecisionInput{
		Approved: true, ClientID: pending.application.ClientID, RedirectURI: pending.redirectURI, Scope: scope,
		CodeChallenge: base64.RawURLEncoding.EncodeToString(digest[:]), CodeChallengeMethod: "S256",
	}
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	decision := performCookieJSONRequest(fixture.router, http.MethodPost, "/api/v1/oauth/authorize", fixture.sessionToken, string(body))
	if decision.Code != http.StatusOK {
		t.Fatalf("authorization-code consent = %d %s", decision.Code, decision.Body.String())
	}
	redirectURL, err := url.Parse(jsonString(t, decision.Body.Bytes(), "redirectUrl"))
	if err != nil {
		t.Fatal(err)
	}
	pending.code = redirectURL.Query().Get("code")
	if pending.code == "" {
		t.Fatal("authorization-code consent returned no code")
	}
	return pending
}

func exchangeOAuthAuthorizationCode(fixture oauthFamilyTestFixture, pending pendingOAuthAuthorizationCode) *httptest.ResponseRecorder {
	return performFormRequest(fixture.router, http.MethodPost, "/api/v1/oauth/token", url.Values{
		"client_id":     {pending.application.ClientID},
		"client_secret": {pending.clientSecret},
		"grant_type":    {"authorization_code"},
		"code":          {pending.code},
		"redirect_uri":  {pending.redirectURI},
		"code_verifier": {pending.verifier},
	})
}

func refreshOAuthFamily(t *testing.T, fixture oauthFamilyTestFixture, refreshToken string) oauthTokenResponse {
	t.Helper()
	response := performFormRequest(fixture.router, http.MethodPost, "/api/v1/oauth/token", url.Values{
		"client_id":     {fixture.application.ClientID},
		"client_secret": {fixture.clientSecret},
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	})
	if response.Code != http.StatusOK {
		t.Fatalf("refresh OAuth family = %d %s", response.Code, response.Body.String())
	}
	var tokens oauthTokenResponse
	if err := json.Unmarshal(response.Body.Bytes(), &tokens); err != nil {
		t.Fatal(err)
	}
	return tokens
}

func performFormRequest(router http.Handler, method, path string, form url.Values) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	router.ServeHTTP(recorder, request)
	return recorder
}

func performCookieJSONRequest(router http.Handler, method, path, sessionToken, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://devops.example.com")
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionToken})
	router.ServeHTTP(recorder, request)
	return recorder
}
