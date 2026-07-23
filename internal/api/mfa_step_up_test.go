package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
)

func TestMFARequiredResponseKeepsPurposeInProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	writeMFARequired(ctx, stepUpPurposeRuntimeTerminal)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d", recorder.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["code"] != "mfa_required" || body["purpose"] != stepUpPurposeRuntimeTerminal {
		t.Fatalf("unexpected response: %#v", body)
	}
	if body["error"] == "" {
		t.Fatalf("expected a stable user-facing error: %#v", body)
	}
}

func TestGenerateTOTPEnrollmentAndValidateWithOneStepSkew(t *testing.T) {
	enrollment, err := generateTOTPEnrollment("admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if enrollment.Secret == "" || !strings.HasPrefix(enrollment.OTPAuthURL, "otpauth://totp/") {
		t.Fatalf("invalid enrollment: %#v", enrollment)
	}
	if !strings.HasPrefix(enrollment.QRCodeDataURL, "data:image/png;base64,") {
		t.Fatalf("invalid QR code data URL: %q", enrollment.QRCodeDataURL)
	}

	now := time.Unix(1_750_000_000, 0)
	code, err := totp.GenerateCodeCustom(enrollment.Secret, now, totp.ValidateOpts{Period: 30, Skew: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !validateTOTPCode(enrollment.Secret, code, now.Add(30*time.Second)) {
		t.Fatal("expected code from the adjacent time step to be accepted")
	}
	wantCounter := now.Unix() / 30
	if counter, valid := matchTOTPCounter(enrollment.Secret, code, now.Add(30*time.Second)); !valid || counter != wantCounter {
		t.Fatalf("matched counter = %d, valid=%v, want %d", counter, valid, wantCounter)
	}
	if validateTOTPCode(enrollment.Secret, code, now.Add(90*time.Second)) {
		t.Fatal("expected code outside the configured skew to be rejected")
	}
}

func TestMFAEnrollmentRequiresPasswordOrFreshOIDCSession(t *testing.T) {
	now := time.Now()
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	localUser := model.User{Password: string(passwordHash)}
	if !mfaEnrollmentReauthenticated(localUser, model.UserSession{}, "correct-password", now) {
		t.Fatal("expected current local password to reauthenticate enrollment")
	}
	if mfaEnrollmentReauthenticated(localUser, model.UserSession{}, "wrong-password", now) {
		t.Fatal("wrong local password must not reauthenticate enrollment")
	}

	passwordlessUser := model.User{}
	freshBoundary := now.Add(-mfaEnrollmentOIDCSessionMaxAge)
	if !mfaEnrollmentReauthenticated(passwordlessUser, model.UserSession{PrimaryAuthenticatedAt: &freshBoundary}, "", now) {
		t.Fatal("session at the documented OIDC freshness boundary should be accepted")
	}
	stalePrimaryAuthentication := now.Add(-mfaEnrollmentOIDCSessionMaxAge - time.Second)
	if mfaEnrollmentReauthenticated(passwordlessUser, model.UserSession{PrimaryAuthenticatedAt: &stalePrimaryAuthentication}, "", now) {
		t.Fatal("stale OIDC session must not reauthenticate enrollment")
	}
	if mfaEnrollmentReauthenticated(passwordlessUser, model.UserSession{PrimaryAuthenticatedAt: &now, ImpersonatorID: "usr_admin"}, "", now) {
		t.Fatal("impersonated session must not reauthenticate enrollment")
	}
	if mfaEnrollmentReauthenticated(passwordlessUser, model.UserSession{CreatedAt: now}, "", now) {
		t.Fatal("legacy session without primary authentication time must fail closed")
	}
}

func TestRecoveryCodesAreUniqueFormattedAndStronglyHashed(t *testing.T) {
	codes, hashes, err := generateRecoveryCodes()
	if err != nil {
		t.Fatal(err)
	}
	if len(codes) != mfaRecoveryCodeCount || len(hashes) != mfaRecoveryCodeCount {
		t.Fatalf("codes=%d hashes=%d", len(codes), len(hashes))
	}
	seen := map[string]bool{}
	for index, code := range codes {
		normalized := normalizeRecoveryCode(code)
		if len(normalized) != 16 || strings.Count(code, "-") != 3 {
			t.Fatalf("invalid recovery code format: %q", code)
		}
		if seen[normalized] {
			t.Fatalf("duplicate recovery code: %q", code)
		}
		seen[normalized] = true
		if hashes[index] == normalized || bcrypt.CompareHashAndPassword([]byte(hashes[index]), []byte(normalized)) != nil {
			t.Fatalf("recovery code %d was not stored as a bcrypt hash", index)
		}
	}
}

func TestStepUpAssertionHonorsIdleAndAbsoluteExpiry(t *testing.T) {
	now := time.Now()
	assertion := model.StepUpAssertion{
		ID:                "mfaas_test",
		IdleExpiresAt:     now.Add(time.Minute),
		AbsoluteExpiresAt: now.Add(2 * time.Minute),
	}
	if !stepUpAssertionActive(assertion, now) {
		t.Fatal("expected assertion to be active")
	}
	assertion.IdleExpiresAt = now
	if stepUpAssertionActive(assertion, now) {
		t.Fatal("expected idle-expired assertion to be inactive")
	}
	assertion.IdleExpiresAt = now.Add(time.Minute)
	assertion.AbsoluteExpiresAt = now
	if stepUpAssertionActive(assertion, now) {
		t.Fatal("expected absolute-expired assertion to be inactive")
	}
}

func TestStepUpIdleRefreshNeverPassesAbsoluteExpiry(t *testing.T) {
	now := time.Now()
	absolute := now.Add(3 * time.Minute)
	if got := refreshedStepUpIdleExpiry(now, 10*time.Minute, absolute); !got.Equal(absolute) {
		t.Fatalf("refresh = %s, want %s", got, absolute)
	}
	if got := refreshedStepUpIdleExpiry(now, time.Minute, absolute); !got.Equal(now.Add(time.Minute)) {
		t.Fatalf("refresh = %s, want %s", got, now.Add(time.Minute))
	}
}

func TestStepUpPurposeAndBearerTokenValidation(t *testing.T) {
	if got := normalizeStepUpPurpose(" RUNTIME_EXEC "); got != stepUpPurposeRuntimeExec {
		t.Fatalf("purpose = %q", got)
	}
	if got := normalizeStepUpPurpose(stepUpPurposeDataRetentionCleanup); got != stepUpPurposeDataRetentionCleanup {
		t.Fatalf("data retention purpose = %q", got)
	}
	if got := normalizeStepUpPurpose("arbitrary_admin_action"); got != "" {
		t.Fatalf("unknown purpose should be rejected, got %q", got)
	}

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/mfa/verify", nil)
	ctx.Request.Header.Set("Authorization", "Bearer pat_example")
	if !requestUsesBearerToken(ctx) {
		t.Fatal("expected PAT request to be detected")
	}
}

func TestStepUpMiddlewareRejectsUnknownPurposeAtRegistration(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected an invalid route purpose to panic during registration")
		}
	}()
	(&Handlers{}).stepUpMiddleware("unknown_sensitive_action")
}

func TestAuthenticationMiddlewaresReuseCurrentUserContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	admin := model.User{ID: "usr_admin", Role: "platform_admin", Language: "zh-CN"}
	handlers := &Handlers{configs: &configCache{values: map[string]string{
		"security.stepUpMfa.enabled": "false",
	}}}
	router := gin.New()
	router.POST(
		"/sensitive",
		func(ctx *gin.Context) {
			ctx.Set(currentUserContextKey, admin)
			ctx.Next()
		},
		handlers.platformAdminMiddleware(),
		handlers.stepUpMiddleware(stepUpPurposeUserAdminUpdate),
		func(ctx *gin.Context) {
			user, ok := handlers.currentUser(ctx)
			if !ok || user.ID != admin.ID {
				t.Fatalf("unexpected current user: %#v, ok=%v", user, ok)
			}
			ctx.Status(http.StatusNoContent)
		},
	)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/sensitive", nil))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestPlatformAdminMiddlewareStopsNonAdminBeforeStepUp(t *testing.T) {
	gin.SetMode(gin.TestMode)
	user := model.User{ID: "usr_member", Role: "user", Language: "zh-CN"}
	handlers := &Handlers{configs: &configCache{values: map[string]string{
		"security.stepUpMfa.enabled": "true",
	}}}
	handlerCalled := false
	router := gin.New()
	router.POST(
		"/admin-sensitive",
		func(ctx *gin.Context) {
			ctx.Set(currentUserContextKey, user)
			ctx.Next()
		},
		handlers.platformAdminMiddleware(),
		handlers.stepUpMiddleware(stepUpPurposeUserAdminUpdate),
		func(ctx *gin.Context) {
			handlerCalled = true
			ctx.Status(http.StatusNoContent)
		},
	)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/admin-sensitive", nil))
	if recorder.Code != http.StatusForbidden || handlerCalled {
		t.Fatalf("status = %d, handlerCalled = %v", recorder.Code, handlerCalled)
	}
}

func TestRequireStepUpReusesMiddlewareAssertion(t *testing.T) {
	user := model.User{ID: "usr_admin", Role: "platform_admin"}
	handlers := &Handlers{configs: &configCache{values: map[string]string{
		"security.stepUpMfa.enabled": "true",
	}}}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set(stepUpPurposeContextKey, stepUpPurposeUserAdminUpdate)
	if !handlers.requireStepUp(ctx, user, stepUpPurposeUserAdminUpdate) {
		t.Fatal("expected middleware assertion to be reused by the handler guard")
	}
}

func TestMFASessionEndpointsRejectPersonalAccessTokensBeforeDatabaseAccess(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/mfa/verify", nil)
	ctx.Request.Header.Set("Authorization", "Bearer pat_example")

	_, _, ok := (&Handlers{}).currentMFAUserSession(ctx)
	if ok {
		t.Fatal("PAT must not be able to establish an MFA browser session")
	}
	var body map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusForbidden || body["code"] != "mfa.session_required" {
		t.Fatalf("unexpected response: status=%d body=%#v", recorder.Code, body)
	}
}

func TestStepUpPolicyConfigValidation(t *testing.T) {
	if !containsStepUpConfig(map[string]any{"security.stepUpMfa.enabled": true}) {
		t.Fatal("expected step-up config update to be detected")
	}
	if containsStepUpConfig(map[string]any{"site.title": "Luna DevOps"}) {
		t.Fatal("unrelated config must not trigger MFA policy validation")
	}
	current := map[string]string{
		"security.stepUpMfa.enabled":                "false",
		"security.stepUpMfa.idleTimeoutMinutes":     "10",
		"security.stepUpMfa.absoluteTimeoutMinutes": "60",
	}
	if stepUpConfigValuesChanged(map[string]string{
		"site.title":                                "Updated",
		"security.stepUpMfa.enabled":                "false",
		"security.stepUpMfa.idleTimeoutMinutes":     "10",
		"security.stepUpMfa.absoluteTimeoutMinutes": "60",
	}, current) {
		t.Fatal("unchanged step-up values in a full form payload must not trigger policy verification")
	}
	if !stepUpConfigValuesChanged(map[string]string{"security.stepUpMfa.idleTimeoutMinutes": "11"}, current) {
		t.Fatal("changed step-up timeout must trigger policy verification")
	}
	if _, err := configMinuteValue("0", 10, 1, 120); err == nil {
		t.Fatal("expected zero idle timeout to be rejected")
	}
	if _, err := configMinuteValue("121", 10, 1, 120); err == nil {
		t.Fatal("expected excessive idle timeout to be rejected")
	}
	if !isBooleanConfigValue("enabled") || isBooleanConfigValue("sometimes") {
		t.Fatal("unexpected boolean config validation")
	}
}
