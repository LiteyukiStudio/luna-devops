package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
)

func TestOAuthDeviceAuthorizationAndRevocationFlow(t *testing.T) {
	db := authIntegrationDB(t)
	t.Setenv("APP_ENV", "development")
	t.Setenv("PUBLIC_BASE_URL", "https://devops.example.com")
	t.Setenv("SECRET_ENCRYPTION_KEY", "oauth-device-integration-key")

	suffix := randomHex(4)
	user := model.User{
		ID:       "usr_device_" + suffix,
		Email:    "device-" + suffix + "@example.com",
		Name:     "Device User",
		Role:     authz.PlatformRoleAdmin,
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
	legacyDeviceCode := "legacy-device-" + suffix
	legacyInvalidatedAt := time.Now()
	if err := db.Create(&model.OAuthDeviceAuthorization{
		ID:              "odev_legacy_" + suffix,
		ApplicationID:   lunaCLIApplicationID,
		DeviceCodeHash:  hashToken(legacyDeviceCode),
		UserCodeHash:    hashToken("legacy-user-" + suffix),
		Status:          "denied",
		IntervalSeconds: 5,
		ExpiresAt:       legacyInvalidatedAt.Add(time.Hour),
		DeniedAt:        &legacyInvalidatedAt,
		ConsumedAt:      &legacyInvalidatedAt,
	}).Error; err != nil {
		t.Fatal(err)
	}

	router := NewRouter(db, mustTestConfig(t))
	legacyScopedStart := performFormRequest(router, http.MethodPost, "/api/v1/oauth/device/authorization", url.Values{
		"client_id": {lunaCLIClientID},
		"scope":     {"user:read"},
	})
	if legacyScopedStart.Code != http.StatusBadRequest || jsonString(t, legacyScopedStart.Body.Bytes(), "error") != "invalid_scope" {
		t.Fatalf("legacy scoped device authorization = %d %s", legacyScopedStart.Code, legacyScopedStart.Body.String())
	}
	invalidatedLegacyExchange := performFormRequest(router, http.MethodPost, "/api/v1/oauth/token", url.Values{
		"client_id":   {lunaCLIClientID},
		"grant_type":  {oauthDeviceCodeGrantType},
		"device_code": {legacyDeviceCode},
	})
	if invalidatedLegacyExchange.Code != http.StatusBadRequest || jsonString(t, invalidatedLegacyExchange.Body.Bytes(), "error") != "invalid_grant" {
		t.Fatalf("invalidated legacy device exchange = %d %s", invalidatedLegacyExchange.Code, invalidatedLegacyExchange.Body.String())
	}
	start := performFormRequest(router, http.MethodPost, "/api/v1/oauth/device/authorization", url.Values{
		"client_id": {lunaCLIClientID},
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
	if tokens.AccessToken == "" || tokens.RefreshToken == "" || tokens.Scope != "" || strings.Contains(exchange.Body.String(), `"scope"`) {
		t.Fatalf("invalid CLI OAuth token response: %#v", tokens)
	}
	var accessToken model.AccessToken
	if err := db.First(&accessToken, "token_hash = ?", hashToken(tokens.AccessToken)).Error; err != nil {
		t.Fatal(err)
	}
	var refreshToken model.OAuthRefreshToken
	if err := db.First(&refreshToken, "token_hash = ?", hashToken(tokens.RefreshToken)).Error; err != nil {
		t.Fatal(err)
	}
	var grant model.OAuthGrant
	if err := db.First(&grant, "id = ?", accessToken.OAuthGrantID).Error; err != nil {
		t.Fatal(err)
	}
	if accessToken.Scope != lunaCLITestFullAccessScope || refreshToken.Scope != lunaCLITestFullAccessScope || grant.Scope != lunaCLITestFullAccessScope {
		t.Fatalf("stored CLI scopes: access=%q refresh=%q grant=%q", accessToken.Scope, refreshToken.Scope, grant.Scope)
	}

	currentUser := performBearerRequest(router, http.MethodGet, "/api/v1/users/me", tokens.AccessToken, "")
	if currentUser.Code != http.StatusOK || jsonString(t, currentUser.Body.Bytes(), "id") != user.ID {
		t.Fatalf("OAuth bearer current user = %d %s", currentUser.Code, currentUser.Body.String())
	}
	configs := performBearerRequest(router, http.MethodGet, "/api/v1/configs", tokens.AccessToken, "")
	if configs.Code != http.StatusOK {
		t.Fatalf("CLI bearer did not inherit administrator permissions = %d %s", configs.Code, configs.Body.String())
	}
	grants := performCookieJSONRequest(router, http.MethodGet, "/api/v1/oauth/grants", sessionToken, "")
	if grants.Code != http.StatusOK || strings.Contains(grants.Body.String(), `"scope":"*"`) || strings.Contains(grants.Body.String(), `"allowedScopes":"*"`) {
		t.Fatalf("authorized applications exposed the internal CLI wildcard = %d %s", grants.Code, grants.Body.String())
	}
	if err := db.Model(&model.User{}).Where("id = ?", user.ID).Update("role", authz.PlatformRoleUser).Error; err != nil {
		t.Fatal(err)
	}
	cliConfigs := performBearerRequest(router, http.MethodGet, "/api/v1/configs", tokens.AccessToken, "")
	webConfigs := performCookieJSONRequest(router, http.MethodGet, "/api/v1/configs", sessionToken, "")
	if cliConfigs.Code != http.StatusForbidden || cliConfigs.Code != webConfigs.Code {
		t.Fatalf("CLI and browser role authorization diverged: CLI=%d Web=%d", cliConfigs.Code, webConfigs.Code)
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
