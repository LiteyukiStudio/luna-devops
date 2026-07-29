package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/pquerna/otp/totp"
)

func TestOAuthDeviceAuthorizationMFAAndRevocationFlow(t *testing.T) {
	db := newMFAIntegrationDB(t)
	t.Setenv("APP_ENV", "development")
	t.Setenv("PUBLIC_BASE_URL", "https://devops.example.com")
	t.Setenv("SECRET_ENCRYPTION_KEY", "oauth-device-integration-key")

	suffix := randomHex(4)
	user := model.User{
		ID:       "usr_device_" + suffix,
		Email:    "device-" + suffix + "@example.com",
		Name:     "Device User",
		Role:     "platform_admin",
		Language: "zh-CN",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	sessionToken := "sess_device_" + suffix
	if err := db.Create(&model.UserSession{
		ID:        "ses_device_" + suffix,
		UserID:    user.ID,
		TokenHash: hashToken(sessionToken),
		ExpiresAt: time.Now().Add(time.Hour),
		CreatedAt: time.Now(),
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.OAuthApplication{
		ID:                      lunaCLIApplicationID,
		Name:                    "Luna CLI",
		ClientID:                lunaCLIClientID,
		ClientSecretHash:        "",
		RedirectURIs:            "",
		AllowedScopes:           "*",
		AccessTokenLifetimeDays: 30,
	}).Error; err != nil {
		t.Fatal(err)
	}

	setupHandlers := &Handlers{db: db, configs: newConfigCache(db), mode: "development", rateLimiter: newRateLimiter()}
	setupHandlers.secrets = secret.NewStore(db, setupHandlers.audit)
	totpSecret := "JBSWY3DPEHPK3PXP"
	secretRef := setupHandlers.secrets.Store(totpSecret, user.ID, mfaSecretResource(user.ID))
	confirmedAt := time.Now()
	if err := db.Create(&model.UserMFAConfig{
		ID:            "mfa_device_" + suffix,
		UserID:        user.ID,
		TOTPSecretRef: secretRef,
		Enabled:       true,
		ConfirmedAt:   &confirmedAt,
	}).Error; err != nil {
		t.Fatal(err)
	}

	router := NewRouter(db)
	start := performFormRequest(router, http.MethodPost, "/api/v1/oauth/device/authorization", url.Values{
		"client_id": {lunaCLIClientID},
		"scope":     {"user:read deployment:data_export"},
	})
	if start.Code != http.StatusOK {
		t.Fatalf("start device authorization = %d %s", start.Code, start.Body.String())
	}
	var device struct {
		DeviceCode string `json:"device_code"`
		UserCode   string `json:"user_code"`
	}
	if err := json.Unmarshal(start.Body.Bytes(), &device); err != nil {
		t.Fatal(err)
	}
	if device.DeviceCode == "" || device.UserCode == "" {
		t.Fatalf("incomplete device authorization response: %#v", device)
	}
	var pendingAuthorization model.OAuthDeviceAuthorization
	if err := db.First(
		&pendingAuthorization,
		"device_code_hash = ?",
		hashToken(device.DeviceCode),
	).Error; err != nil {
		t.Fatal(err)
	}
	if pendingAuthorization.GrantID != nil || pendingAuthorization.UserID != nil {
		t.Fatalf("pending authorization unexpectedly has an owner or grant: %#v", pendingAuthorization)
	}
	if pendingAuthorization.Scope != "user:read,deployment:data_export" {
		t.Fatalf("pending authorization scope = %q", pendingAuthorization.Scope)
	}

	pending := performFormRequest(router, http.MethodPost, "/api/v1/oauth/token", url.Values{
		"client_id":   {lunaCLIClientID},
		"grant_type":  {oauthDeviceCodeGrantType},
		"device_code": {device.DeviceCode},
	})
	if pending.Code != http.StatusBadRequest || jsonString(t, pending.Body.Bytes(), "error") != "authorization_pending" {
		t.Fatalf("pending token exchange = %d %s", pending.Code, pending.Body.String())
	}

	verify := performCookieJSONRequest(router, http.MethodPost, "/api/v1/oauth/device/verification", sessionToken, `{
		"approved": true,
		"userCode": "`+device.UserCode+`"
	}`)
	if verify.Code != http.StatusOK {
		t.Fatalf("approve device authorization = %d %s", verify.Code, verify.Body.String())
	}
	if err := db.Model(&model.OAuthDeviceAuthorization{}).
		Where("device_code_hash = ?", hashToken(device.DeviceCode)).
		Update("last_polled_at", nil).Error; err != nil {
		t.Fatal(err)
	}

	exchange := performFormRequest(router, http.MethodPost, "/api/v1/oauth/token", url.Values{
		"client_id":   {lunaCLIClientID},
		"grant_type":  {oauthDeviceCodeGrantType},
		"device_code": {device.DeviceCode},
	})
	if exchange.Code != http.StatusOK {
		t.Fatalf("device token exchange = %d %s", exchange.Code, exchange.Body.String())
	}
	var tokens oauthTokenResponse
	if err := json.Unmarshal(exchange.Body.Bytes(), &tokens); err != nil {
		t.Fatal(err)
	}
	if tokens.AccessToken == "" || tokens.RefreshToken == "" {
		t.Fatalf("incomplete OAuth token response: %#v", tokens)
	}

	currentUser := performBearerRequest(router, http.MethodGet, "/api/v1/users/me", tokens.AccessToken, "")
	if currentUser.Code != http.StatusOK || jsonString(t, currentUser.Body.Bytes(), "id") != user.ID {
		t.Fatalf("OAuth bearer current user = %d %s", currentUser.Code, currentUser.Body.String())
	}

	code, err := totp.GenerateCode(totpSecret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	stepUp := performBearerRequest(router, http.MethodPost, "/api/v1/auth/mfa/verify", tokens.AccessToken, `{
		"purpose": "`+stepUpPurposeRuntimeExec+`",
		"code": "`+code+`"
	}`)
	if stepUp.Code != http.StatusOK {
		t.Fatalf("OAuth bearer MFA = %d %s", stepUp.Code, stepUp.Body.String())
	}
	var grant model.OAuthGrant
	if err := db.First(&grant, "application_id = ? and user_id = ? and revoked_at is null", lunaCLIApplicationID, user.ID).Error; err != nil {
		t.Fatal(err)
	}
	var assertion model.StepUpAssertion
	if err := db.First(
		&assertion,
		"user_id = ? and session_id = ? and purpose = ?",
		user.ID,
		oauthAssertionSubject(grant.ID),
		stepUpPurposeRuntimeExec,
	).Error; err != nil {
		t.Fatalf("OAuth MFA assertion was not persisted: %v", err)
	}

	revoke := performFormRequest(router, http.MethodPost, "/api/v1/oauth/revoke", url.Values{
		"client_id": {lunaCLIClientID},
		"token":     {tokens.RefreshToken},
	})
	if revoke.Code != http.StatusOK {
		t.Fatalf("revoke OAuth grant = %d %s", revoke.Code, revoke.Body.String())
	}
	revokedUser := performBearerRequest(router, http.MethodGet, "/api/v1/users/me", tokens.AccessToken, "")
	if revokedUser.Code != http.StatusUnauthorized {
		t.Fatalf("revoked bearer remains usable = %d %s", revokedUser.Code, revokedUser.Body.String())
	}
	var assertionCount int64
	if err := db.Model(&model.StepUpAssertion{}).
		Where("session_id = ?", oauthAssertionSubject(grant.ID)).
		Count(&assertionCount).Error; err != nil || assertionCount != 0 {
		t.Fatalf("OAuth assertions remain after revocation: count=%d err=%v", assertionCount, err)
	}
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
