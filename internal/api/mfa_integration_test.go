package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/gin-gonic/gin"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestMFAEnrollmentVerificationRecoveryAndDisableFlow(t *testing.T) {
	db := newMFAIntegrationDB(t)
	t.Setenv("APP_ENV", "development")
	t.Setenv("SECRET_ENCRYPTION_KEY", "mfa-integration-test-key")

	testSuffix := randomHex(4)
	password := "mfa-integration-password"
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	user := model.User{ID: "usr_mfa_" + testSuffix, Email: "mfa-" + testSuffix + "@example.com", Name: "MFA User", AuthType: "local", Role: "platform_admin", Language: "zh-CN", Password: string(passwordHash)}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	sessionToken := "sess_mfa_integration_" + testSuffix
	session := model.UserSession{ID: "ses_mfa_" + testSuffix, UserID: user.ID, TokenHash: hashToken(sessionToken), ExpiresAt: time.Now().Add(time.Hour), CreatedAt: time.Now()}
	if err := db.Create(&session).Error; err != nil {
		t.Fatal(err)
	}
	handlers := &Handlers{db: db, configs: newConfigCache(db), mode: "development", rateLimiter: newRateLimiter()}
	handlers.secrets = secret.NewStore(db, handlers.audit)

	policyBlockedRecorder, policyBlockedContext := newMFAIntegrationContext(http.MethodPut, "/api/v1/configs", map[string]any{"values": map[string]any{"security.stepUpMfa.enabled": true}}, sessionToken)
	handlers.UpdateConfigs(policyBlockedContext)
	if policyBlockedRecorder.Code != http.StatusConflict {
		t.Fatalf("policy enabled without an MFA admin = %d %s", policyBlockedRecorder.Code, policyBlockedRecorder.Body.String())
	}

	statusRecorder, statusContext := newMFAIntegrationContext(http.MethodGet, "/api/v1/auth/mfa/status", nil, sessionToken)
	handlers.GetMFAStatus(statusContext)
	if statusRecorder.Code != http.StatusOK || jsonBool(t, statusRecorder.Body.Bytes(), "enabled") {
		t.Fatalf("initial status = %d %s", statusRecorder.Code, statusRecorder.Body.String())
	}

	wrongPasswordRecorder, wrongPasswordContext := newMFAIntegrationContext(http.MethodPost, "/api/v1/auth/mfa/totp/enroll", map[string]string{"currentPassword": "wrong"}, sessionToken)
	handlers.EnrollMFA(wrongPasswordContext)
	if wrongPasswordRecorder.Code != http.StatusUnauthorized || jsonString(t, wrongPasswordRecorder.Body.Bytes(), "code") != "mfa.reauth_required" {
		t.Fatalf("wrong-password enroll = %d %s", wrongPasswordRecorder.Code, wrongPasswordRecorder.Body.String())
	}

	enrollRecorder, enrollContext := newMFAIntegrationContext(http.MethodPost, "/api/v1/auth/mfa/totp/enroll", map[string]string{"currentPassword": password}, sessionToken)
	handlers.EnrollMFA(enrollContext)
	if enrollRecorder.Code != http.StatusCreated {
		t.Fatalf("enroll = %d %s", enrollRecorder.Code, enrollRecorder.Body.String())
	}
	var enrollment struct {
		Secret        string `json:"secret"`
		OTPAuthURL    string `json:"otpauthUrl"`
		QRCodeDataURL string `json:"qrCodeDataUrl"`
	}
	if err := json.Unmarshal(enrollRecorder.Body.Bytes(), &enrollment); err != nil {
		t.Fatal(err)
	}
	if enrollment.Secret == "" || enrollment.OTPAuthURL == "" || enrollment.QRCodeDataURL == "" {
		t.Fatalf("incomplete enrollment response: %#v", enrollment)
	}
	var storedSecret model.SecretValue
	if err := db.First(&storedSecret, "resource = ?", mfaSecretResource(user.ID)).Error; err != nil {
		t.Fatal(err)
	}
	if storedSecret.CipherRef == "" || bytes.Contains([]byte(storedSecret.CipherRef), []byte(enrollment.Secret)) {
		t.Fatal("TOTP secret was not encrypted at rest")
	}

	confirmationCode, err := totp.GenerateCode(enrollment.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	confirmRecorder, confirmContext := newMFAIntegrationContext(http.MethodPost, "/api/v1/auth/mfa/totp/confirm", map[string]string{"code": confirmationCode}, sessionToken)
	handlers.ConfirmMFA(confirmContext)
	if confirmRecorder.Code != http.StatusOK {
		t.Fatalf("confirm = %d %s", confirmRecorder.Code, confirmRecorder.Body.String())
	}
	var initialRecovery struct {
		RecoveryCodes []string `json:"recoveryCodes"`
	}
	if err := json.Unmarshal(confirmRecorder.Body.Bytes(), &initialRecovery); err != nil {
		t.Fatal(err)
	}
	if len(initialRecovery.RecoveryCodes) != mfaRecoveryCodeCount {
		t.Fatalf("recovery code count = %d", len(initialRecovery.RecoveryCodes))
	}
	if !handlers.hasMFAEnabledPlatformAdmin() {
		t.Fatal("confirmed platform administrator was not recognized as MFA-enabled")
	}
	reusedConfirmationRecorder, reusedConfirmationContext := newMFAIntegrationContext(http.MethodPost, "/api/v1/auth/mfa/verify", map[string]string{"purpose": stepUpPurposeRuntimeExec, "code": confirmationCode}, sessionToken)
	handlers.VerifyMFA(reusedConfirmationContext)
	if reusedConfirmationRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("confirmation code reuse = %d %s", reusedConfirmationRecorder.Code, reusedConfirmationRecorder.Body.String())
	}

	verificationCode, err := totp.GenerateCode(enrollment.Secret, time.Now().Add(30*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	securityVerifyRecorder, securityVerifyContext := newMFAIntegrationContext(http.MethodPost, "/api/v1/auth/mfa/verify", map[string]string{"purpose": stepUpPurposeSecuritySettingsUpdate, "code": verificationCode}, sessionToken)
	handlers.VerifyMFA(securityVerifyContext)
	if securityVerifyRecorder.Code != http.StatusOK {
		t.Fatalf("security settings verify = %d %s", securityVerifyRecorder.Code, securityVerifyRecorder.Body.String())
	}
	policyEnableRecorder, policyEnableContext := newMFAIntegrationContext(http.MethodPut, "/api/v1/configs", map[string]any{"values": map[string]any{"security.stepUpMfa.enabled": true}}, sessionToken)
	handlers.UpdateConfigs(policyEnableContext)
	if policyEnableRecorder.Code != http.StatusOK || !handlers.stepUpMFAEnabled() {
		t.Fatalf("policy enable = %d %s", policyEnableRecorder.Code, policyEnableRecorder.Body.String())
	}

	reusedVerifyRecorder, reusedVerifyContext := newMFAIntegrationContext(http.MethodPost, "/api/v1/auth/mfa/verify", map[string]string{"purpose": stepUpPurposeMFAManage, "code": verificationCode}, sessionToken)
	handlers.VerifyMFA(reusedVerifyContext)
	if reusedVerifyRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("verification code reuse = %d %s", reusedVerifyRecorder.Code, reusedVerifyRecorder.Body.String())
	}

	verifyRecorder, verifyContext := newMFAIntegrationContext(http.MethodPost, "/api/v1/auth/mfa/verify", map[string]string{"purpose": stepUpPurposeMFAManage, "recoveryCode": initialRecovery.RecoveryCodes[0]}, sessionToken)
	handlers.VerifyMFA(verifyContext)
	if verifyRecorder.Code != http.StatusOK {
		t.Fatalf("verify = %d %s", verifyRecorder.Code, verifyRecorder.Body.String())
	}

	regenerateRecorder, regenerateContext := newMFAIntegrationContext(http.MethodPost, "/api/v1/auth/mfa/recovery-codes", nil, sessionToken)
	handlers.RegenerateMFARecoveryCodes(regenerateContext)
	if regenerateRecorder.Code != http.StatusOK {
		t.Fatalf("regenerate = %d %s", regenerateRecorder.Code, regenerateRecorder.Body.String())
	}
	var regenerated struct {
		RecoveryCodes []string `json:"recoveryCodes"`
	}
	if err := json.Unmarshal(regenerateRecorder.Body.Bytes(), &regenerated); err != nil {
		t.Fatal(err)
	}
	if len(regenerated.RecoveryCodes) != mfaRecoveryCodeCount {
		t.Fatalf("regenerated recovery code count = %d", len(regenerated.RecoveryCodes))
	}

	recoveryCode := regenerated.RecoveryCodes[0]
	recoveryRecorder, recoveryContext := newMFAIntegrationContext(http.MethodPost, "/api/v1/auth/mfa/verify", map[string]string{"purpose": stepUpPurposeDataExport, "recoveryCode": recoveryCode}, sessionToken)
	handlers.VerifyMFA(recoveryContext)
	if recoveryRecorder.Code != http.StatusOK {
		t.Fatalf("recovery verify = %d %s", recoveryRecorder.Code, recoveryRecorder.Body.String())
	}
	assertionRecorder, assertionContext := newMFAIntegrationContext(http.MethodGet, "/sensitive", nil, sessionToken)
	if !handlers.requireMFAAssertion(assertionContext, user, stepUpPurposeDataExport) || assertionRecorder.Code != http.StatusOK {
		t.Fatalf("assertion refresh = %d %s", assertionRecorder.Code, assertionRecorder.Body.String())
	}

	reuseRecorder, reuseContext := newMFAIntegrationContext(http.MethodPost, "/api/v1/auth/mfa/verify", map[string]string{"purpose": stepUpPurposeRuntimeExec, "recoveryCode": recoveryCode}, sessionToken)
	handlers.VerifyMFA(reuseContext)
	if reuseRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("reused recovery code = %d %s", reuseRecorder.Code, reuseRecorder.Body.String())
	}
	var usedCount int64
	if err := db.Model(&model.MFARecoveryCode{}).Where("user_id = ? and used_at is not null", user.ID).Count(&usedCount).Error; err != nil || usedCount != 1 {
		t.Fatalf("used recovery codes = %d, err=%v", usedCount, err)
	}
	blockedDisableRecorder, blockedDisableContext := newMFAIntegrationContext(http.MethodDelete, "/api/v1/auth/mfa", nil, sessionToken)
	handlers.DisableMFA(blockedDisableContext)
	if blockedDisableRecorder.Code != http.StatusConflict {
		t.Fatalf("last MFA admin disable while policy enabled = %d %s", blockedDisableRecorder.Code, blockedDisableRecorder.Body.String())
	}

	policyDisableRecorder, policyDisableContext := newMFAIntegrationContext(http.MethodPut, "/api/v1/configs", map[string]any{"values": map[string]any{"security.stepUpMfa.enabled": false}}, sessionToken)
	handlers.UpdateConfigs(policyDisableContext)
	if policyDisableRecorder.Code != http.StatusOK || handlers.stepUpMFAEnabled() {
		t.Fatalf("policy disable = %d %s", policyDisableRecorder.Code, policyDisableRecorder.Body.String())
	}

	disableRecorder, disableContext := newMFAIntegrationContext(http.MethodDelete, "/api/v1/auth/mfa", nil, sessionToken)
	handlers.DisableMFA(disableContext)
	disableContext.Writer.WriteHeaderNow()
	if disableRecorder.Code != http.StatusNoContent {
		t.Fatalf("disable = %d %s", disableRecorder.Code, disableRecorder.Body.String())
	}
	var configCount, assertionCount int64
	_ = db.Model(&model.UserMFAConfig{}).Where("user_id = ?", user.ID).Count(&configCount).Error
	_ = db.Model(&model.StepUpAssertion{}).Where("user_id = ?", user.ID).Count(&assertionCount).Error
	if configCount != 0 || assertionCount != 0 {
		t.Fatalf("MFA state remained after disable: configs=%d assertions=%d", configCount, assertionCount)
	}

	var auditLogs []model.AuditLog
	if err := db.Where("user_id = ?", user.ID).Find(&auditLogs).Error; err != nil {
		t.Fatal(err)
	}
	requiredAuditActions := map[string]bool{
		"mfa.enroll":                    false,
		"mfa.confirm":                   false,
		"mfa.verify":                    false,
		"mfa.recovery_codes_regenerate": false,
		"mfa.recovery_code_used":        false,
		"mfa.policy_update":             false,
		"mfa.disable":                   false,
	}
	for _, entry := range auditLogs {
		if _, required := requiredAuditActions[entry.Action]; required {
			requiredAuditActions[entry.Action] = true
		}
		for _, sensitiveValue := range []string{enrollment.Secret, confirmationCode, recoveryCode} {
			if sensitiveValue != "" && strings.Contains(entry.Message, sensitiveValue) {
				t.Fatalf("audit log %s leaked an MFA credential", entry.Action)
			}
		}
	}
	for action, found := range requiredAuditActions {
		if !found {
			t.Fatalf("missing MFA audit action %q; logs=%#v", action, auditLogs)
		}
	}
}

func TestAdminResetUserMFAFlowAndLastAdminProtection(t *testing.T) {
	db := newMFAIntegrationDB(t)
	limitMFAIntegrationConnections(t, db, 1)
	t.Setenv("APP_ENV", "development")
	t.Setenv("SECRET_ENCRYPTION_KEY", "mfa-admin-reset-test-key")
	now := time.Now()
	suffix := randomHex(4)
	actor := model.User{ID: "usr_actor_" + suffix, Email: "actor-" + suffix + "@example.com", Name: "Actor", AuthType: "local", Role: "platform_admin", Language: "en-US"}
	target := model.User{ID: "usr_target_" + suffix, Email: "target-" + suffix + "@example.com", Name: "Target", AuthType: "local", Role: "user", Language: "en-US"}
	lastAdmin := model.User{ID: "usr_last_admin_" + suffix, Email: "last-admin-" + suffix + "@example.com", Name: "Last Admin", AuthType: "local", Role: "platform_admin", Language: "en-US"}
	if err := db.Create(&[]model.User{actor, target, lastAdmin}).Error; err != nil {
		t.Fatal(err)
	}
	sessionToken := "sess_admin_reset_" + suffix
	actorSession := model.UserSession{ID: "ses_actor_" + suffix, UserID: actor.ID, TokenHash: hashToken(sessionToken), ExpiresAt: now.Add(time.Hour), CreatedAt: now}
	if err := db.Create(&actorSession).Error; err != nil {
		t.Fatal(err)
	}
	targetSessionToken := "sess_target_reset_" + suffix
	targetSession := model.UserSession{ID: "ses_target_" + suffix, UserID: target.ID, TokenHash: hashToken(targetSessionToken), ExpiresAt: now.Add(time.Hour), CreatedAt: now}
	if err := db.Create(&targetSession).Error; err != nil {
		t.Fatal(err)
	}
	actorAssertion := model.StepUpAssertion{ID: "mfaas_actor_" + suffix, UserID: actor.ID, SessionID: actorSession.ID, Purpose: stepUpPurposeUserAdminUpdate, VerifiedAt: now, LastActivityAt: now, IdleExpiresAt: now.Add(10 * time.Minute), AbsoluteExpiresAt: now.Add(time.Hour)}
	if err := db.Create(&actorAssertion).Error; err != nil {
		t.Fatal(err)
	}
	handlers := &Handlers{db: db, configs: newConfigCache(db), mode: "development", rateLimiter: newRateLimiter()}
	handlers.secrets = secret.NewStore(db, handlers.audit)

	targetSecretRef := handlers.secrets.Store("TARGETSECRET", actor.ID, mfaSecretResource(target.ID))
	confirmedAt := now
	if err := db.Create(&model.UserMFAConfig{ID: "mfa_target_" + suffix, UserID: target.ID, TOTPSecretRef: targetSecretRef, Enabled: true, ConfirmedAt: &confirmedAt}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.MFARecoveryCode{ID: "mfr_target_" + suffix, UserID: target.ID, CodeHash: "hash"}).Error; err != nil {
		t.Fatal(err)
	}
	targetAssertion := model.StepUpAssertion{ID: "mfaas_target_" + suffix, UserID: target.ID, SessionID: targetSession.ID, Purpose: stepUpPurposeRuntimeExec, VerifiedAt: now, LastActivityAt: now, IdleExpiresAt: now.Add(time.Minute), AbsoluteExpiresAt: now.Add(time.Hour)}
	if err := db.Create(&targetAssertion).Error; err != nil {
		t.Fatal(err)
	}
	nonAdminRecorder, nonAdminContext := newMFAIntegrationContext(http.MethodDelete, "/api/v1/users/"+actor.ID+"/mfa", nil, targetSessionToken)
	nonAdminContext.Params = gin.Params{{Key: "userId", Value: actor.ID}}
	handlers.AdminResetUserMFA(nonAdminContext)
	if nonAdminRecorder.Code != http.StatusForbidden {
		t.Fatalf("non-admin reset = %d %s", nonAdminRecorder.Code, nonAdminRecorder.Body.String())
	}
	var deniedAudit model.AuditLog
	if err := db.First(&deniedAudit, "user_id = ? and action = ? and resource = ? and success = ?", target.ID, "mfa.admin_reset", actor.ID, false).Error; err != nil {
		t.Fatalf("missing denied admin reset audit: %v", err)
	}

	resetRecorder, resetContext := newMFAIntegrationContext(http.MethodDelete, "/api/v1/users/"+target.ID+"/mfa", nil, sessionToken)
	resetContext.Params = gin.Params{{Key: "userId", Value: target.ID}}
	handlers.AdminResetUserMFA(resetContext)
	resetContext.Writer.WriteHeaderNow()
	if resetRecorder.Code != http.StatusNoContent {
		t.Fatalf("admin reset = %d %s", resetRecorder.Code, resetRecorder.Body.String())
	}
	for table, value := range map[string]any{
		"user_mfa_configs":   &model.UserMFAConfig{},
		"mfa_recovery_codes": &model.MFARecoveryCode{},
		"step_up_assertions": &model.StepUpAssertion{},
		"secret_values":      &model.SecretValue{},
	} {
		var count int64
		query := db.Table(table)
		if table == "secret_values" {
			query = query.Where("resource = ?", mfaSecretResource(target.ID))
		} else {
			query = query.Where("user_id = ?", target.ID)
		}
		if err := query.Model(value).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("%s remained after admin reset: count=%d err=%v", table, count, err)
		}
	}
	var resetAudit model.AuditLog
	if err := db.First(&resetAudit, "user_id = ? and action = ? and resource = ? and success = ?", actor.ID, "mfa.admin_reset", target.ID, true).Error; err != nil {
		t.Fatalf("missing successful admin reset audit: %v", err)
	}

	lastAdminSecretRef := handlers.secrets.Store("LASTADMINSECRET", actor.ID, mfaSecretResource(lastAdmin.ID))
	if err := db.Create(&model.UserMFAConfig{ID: "mfa_last_admin_" + suffix, UserID: lastAdmin.ID, TOTPSecretRef: lastAdminSecretRef, Enabled: true, ConfirmedAt: &confirmedAt}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Save(&model.AppConfig{Key: "security.stepUpMfa.enabled", Value: "true"}).Error; err != nil {
		t.Fatal(err)
	}
	handlers.configs.reload(db)
	blockedRecorder, blockedContext := newMFAIntegrationContext(http.MethodDelete, "/api/v1/users/"+lastAdmin.ID+"/mfa", nil, sessionToken)
	blockedContext.Params = gin.Params{{Key: "userId", Value: lastAdmin.ID}}
	handlers.AdminResetUserMFA(blockedContext)
	if blockedRecorder.Code != http.StatusConflict || jsonString(t, blockedRecorder.Body.Bytes(), "code") != "mfa.last_admin_required" {
		t.Fatalf("last-admin reset = %d %s", blockedRecorder.Code, blockedRecorder.Body.String())
	}
	var lastAdminConfigCount int64
	if err := db.Model(&model.UserMFAConfig{}).Where("user_id = ?", lastAdmin.ID).Count(&lastAdminConfigCount).Error; err != nil || lastAdminConfigCount != 1 {
		t.Fatalf("last admin MFA was removed: count=%d err=%v", lastAdminConfigCount, err)
	}
}

func TestOIDCMFAEnrollmentRequiresFreshBrowserSession(t *testing.T) {
	db := newMFAIntegrationDB(t)
	t.Setenv("APP_ENV", "development")
	t.Setenv("SECRET_ENCRYPTION_KEY", "mfa-oidc-reauth-test-key")
	now := time.Now()
	suffix := randomHex(4)
	user := model.User{ID: "usr_oidc_mfa_" + suffix, Email: "oidc-mfa-" + suffix + "@example.com", Name: "OIDC MFA", AuthType: "oidc", Role: "user", Language: "en-US"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	sessionToken := "sess_oidc_mfa_" + suffix
	stalePrimaryAuthentication := now.Add(-mfaEnrollmentOIDCSessionMaxAge - time.Second)
	session := model.UserSession{ID: "ses_oidc_mfa_" + suffix, UserID: user.ID, TokenHash: hashToken(sessionToken), ExpiresAt: now.Add(time.Hour), PrimaryAuthenticatedAt: &stalePrimaryAuthentication}
	if err := db.Create(&session).Error; err != nil {
		t.Fatal(err)
	}
	handlers := &Handlers{db: db, configs: newConfigCache(db), mode: "development", rateLimiter: newRateLimiter()}
	handlers.secrets = secret.NewStore(db, handlers.audit)

	staleRecorder, staleContext := newMFAIntegrationContext(http.MethodPost, "/api/v1/auth/mfa/totp/enroll", map[string]any{}, sessionToken)
	handlers.EnrollMFA(staleContext)
	if staleRecorder.Code != http.StatusUnauthorized || jsonString(t, staleRecorder.Body.Bytes(), "code") != "mfa.reauth_required" {
		t.Fatalf("stale OIDC enroll = %d %s", staleRecorder.Code, staleRecorder.Body.String())
	}
	if err := db.Model(&session).Update("primary_authenticated_at", time.Now()).Error; err != nil {
		t.Fatal(err)
	}
	freshRecorder, freshContext := newMFAIntegrationContext(http.MethodPost, "/api/v1/auth/mfa/totp/enroll", map[string]any{}, sessionToken)
	handlers.EnrollMFA(freshContext)
	if freshRecorder.Code != http.StatusCreated {
		t.Fatalf("fresh OIDC enroll = %d %s", freshRecorder.Code, freshRecorder.Body.String())
	}
}

func TestConcurrentPlatformAdminMFADisableKeepsOneEnabledAdmin(t *testing.T) {
	db := newMFAIntegrationDB(t)
	t.Setenv("APP_ENV", "development")
	t.Setenv("SECRET_ENCRYPTION_KEY", "mfa-concurrent-disable-test-key")
	now := time.Now()
	suffix := randomHex(4)
	users := []model.User{
		{ID: "usr_disable_a_" + suffix, Email: "disable-a-" + suffix + "@example.com", Name: "Disable A", AuthType: "local", Role: "platform_admin", Language: "en-US"},
		{ID: "usr_disable_b_" + suffix, Email: "disable-b-" + suffix + "@example.com", Name: "Disable B", AuthType: "local", Role: "platform_admin", Language: "en-US"},
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatal(err)
	}
	handlers := &Handlers{db: db, configs: newConfigCache(db), mode: "development", rateLimiter: newRateLimiter()}
	handlers.secrets = secret.NewStore(db, handlers.audit)
	if err := db.Save(&model.AppConfig{Key: "security.stepUpMfa.enabled", Value: "true"}).Error; err != nil {
		t.Fatal(err)
	}
	handlers.configs.reload(db)
	tokens := make([]string, len(users))
	for index, user := range users {
		tokens[index] = "sess_disable_" + fmt.Sprint(index) + "_" + suffix
		session := model.UserSession{ID: "ses_disable_" + fmt.Sprint(index) + "_" + suffix, UserID: user.ID, TokenHash: hashToken(tokens[index]), ExpiresAt: now.Add(time.Hour), PrimaryAuthenticatedAt: &now}
		if err := db.Create(&session).Error; err != nil {
			t.Fatal(err)
		}
		secretRef := handlers.secrets.Store("DISABLESECRET"+fmt.Sprint(index), user.ID, mfaSecretResource(user.ID))
		confirmedAt := now
		if err := db.Create(&model.UserMFAConfig{ID: "mfa_disable_" + fmt.Sprint(index) + "_" + suffix, UserID: user.ID, TOTPSecretRef: secretRef, Enabled: true, ConfirmedAt: &confirmedAt}).Error; err != nil {
			t.Fatal(err)
		}
		assertion := model.StepUpAssertion{ID: "mfaas_disable_" + fmt.Sprint(index) + "_" + suffix, UserID: user.ID, SessionID: session.ID, Purpose: stepUpPurposeMFAManage, VerifiedAt: now, LastActivityAt: now, IdleExpiresAt: now.Add(10 * time.Minute), AbsoluteExpiresAt: now.Add(time.Hour)}
		if err := db.Create(&assertion).Error; err != nil {
			t.Fatal(err)
		}
	}

	start := make(chan struct{})
	statuses := make(chan int, len(users))
	var workers sync.WaitGroup
	for index := range users {
		index := index
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			recorder, ctx := newMFAIntegrationContext(http.MethodDelete, "/api/v1/auth/mfa", nil, tokens[index])
			handlers.DisableMFA(ctx)
			ctx.Writer.WriteHeaderNow()
			statuses <- recorder.Code
		}()
	}
	close(start)
	workers.Wait()
	close(statuses)
	successes, conflicts := 0, 0
	for status := range statuses {
		switch status {
		case http.StatusNoContent:
			successes++
		case http.StatusConflict:
			conflicts++
		default:
			t.Fatalf("unexpected concurrent disable status %d", status)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent disable results: success=%d conflict=%d", successes, conflicts)
	}
	var enabledAdmins int64
	if err := db.Model(&model.UserMFAConfig{}).Where("enabled = ?", true).Count(&enabledAdmins).Error; err != nil || enabledAdmins != 1 {
		t.Fatalf("enabled MFA administrators = %d, err=%v", enabledAdmins, err)
	}
}

func TestConcurrentStepUpEnableAndLastAdminDisableRemainRecoverable(t *testing.T) {
	db := newMFAIntegrationDB(t)
	limitMFAIntegrationConnections(t, db, 1)
	t.Setenv("APP_ENV", "development")
	t.Setenv("SECRET_ENCRYPTION_KEY", "mfa-policy-disable-race-test-key")
	now := time.Now()
	suffix := randomHex(4)
	user := model.User{
		ID:       "usr_policy_race_" + suffix,
		Email:    "policy-race-" + suffix + "@example.com",
		Name:     "Policy Race Admin",
		AuthType: "local",
		Role:     "platform_admin",
		Language: "en-US",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	sessionToken := "sess_policy_race_" + suffix
	session := model.UserSession{
		ID:                     "ses_policy_race_" + suffix,
		UserID:                 user.ID,
		TokenHash:              hashToken(sessionToken),
		ExpiresAt:              now.Add(time.Hour),
		PrimaryAuthenticatedAt: &now,
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatal(err)
	}
	handlers := &Handlers{db: db, configs: newConfigCache(db), mode: "development", rateLimiter: newRateLimiter()}
	handlers.secrets = secret.NewStore(db, handlers.audit)
	secretRef := handlers.secrets.Store("POLICYRACESECRET", user.ID, mfaSecretResource(user.ID))
	confirmedAt := now
	if err := db.Create(&model.UserMFAConfig{
		ID:            "mfa_policy_race_" + suffix,
		UserID:        user.ID,
		TOTPSecretRef: secretRef,
		Enabled:       true,
		ConfirmedAt:   &confirmedAt,
	}).Error; err != nil {
		t.Fatal(err)
	}
	for index, purpose := range []string{stepUpPurposeMFAManage, stepUpPurposeSecuritySettingsUpdate} {
		assertion := model.StepUpAssertion{
			ID:                "mfaas_policy_race_" + fmt.Sprint(index) + "_" + suffix,
			UserID:            user.ID,
			SessionID:         session.ID,
			Purpose:           purpose,
			VerifiedAt:        now,
			LastActivityAt:    now,
			IdleExpiresAt:     now.Add(10 * time.Minute),
			AbsoluteExpiresAt: now.Add(time.Hour),
		}
		if err := db.Create(&assertion).Error; err != nil {
			t.Fatal(err)
		}
	}

	start := make(chan struct{})
	statuses := make(chan int, 2)
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		<-start
		recorder, ctx := newMFAIntegrationContext(http.MethodPut, "/api/v1/configs", map[string]any{
			"values": map[string]any{"security.stepUpMfa.enabled": true},
		}, sessionToken)
		handlers.UpdateConfigs(ctx)
		statuses <- recorder.Code
	}()
	go func() {
		defer workers.Done()
		<-start
		recorder, ctx := newMFAIntegrationContext(http.MethodDelete, "/api/v1/auth/mfa", nil, sessionToken)
		handlers.DisableMFA(ctx)
		ctx.Writer.WriteHeaderNow()
		statuses <- recorder.Code
	}()
	close(start)
	workers.Wait()
	close(statuses)
	for status := range statuses {
		if status != http.StatusOK && status != http.StatusNoContent && status != http.StatusConflict && status != http.StatusForbidden {
			t.Fatalf("unexpected policy/disable race status %d", status)
		}
	}

	var enabledMFAConfigs int64
	if err := db.Model(&model.UserMFAConfig{}).Where("enabled = ?", true).Count(&enabledMFAConfigs).Error; err != nil {
		t.Fatal(err)
	}
	var policy model.AppConfig
	policyEnabled := false
	if err := db.First(&policy, "key = ?", "security.stepUpMfa.enabled").Error; err == nil {
		policyEnabled = configBool(policy.Value)
	} else if err != gorm.ErrRecordNotFound {
		t.Fatal(err)
	}
	if policyEnabled && enabledMFAConfigs == 0 {
		t.Fatal("step-up MFA policy was enabled after the final MFA-enabled administrator was removed")
	}
}

func TestLastMFAEnabledAdminCannotBeDisabledOrDemoted(t *testing.T) {
	db := newMFAIntegrationDB(t)
	limitMFAIntegrationConnections(t, db, 1)
	t.Setenv("APP_ENV", "development")
	t.Setenv("SECRET_ENCRYPTION_KEY", "mfa-admin-role-protection-test-key")
	now := time.Now()
	suffix := randomHex(4)
	user := model.User{
		ID:       "usr_admin_role_guard_" + suffix,
		Email:    "admin-role-guard-" + suffix + "@example.com",
		Name:     "Admin Role Guard",
		AuthType: "local",
		Role:     "platform_admin",
		Language: "en-US",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	sessionToken := "sess_admin_role_guard_" + suffix
	session := model.UserSession{
		ID:                     "ses_admin_role_guard_" + suffix,
		UserID:                 user.ID,
		TokenHash:              hashToken(sessionToken),
		ExpiresAt:              now.Add(time.Hour),
		PrimaryAuthenticatedAt: &now,
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatal(err)
	}
	handlers := &Handlers{db: db, configs: newConfigCache(db), mode: "development", rateLimiter: newRateLimiter()}
	handlers.secrets = secret.NewStore(db, handlers.audit)
	secretRef := handlers.secrets.Store("ADMINROLEGSECRET", user.ID, mfaSecretResource(user.ID))
	confirmedAt := now
	if err := db.Create(&model.UserMFAConfig{
		ID:            "mfa_admin_role_guard_" + suffix,
		UserID:        user.ID,
		TOTPSecretRef: secretRef,
		Enabled:       true,
		ConfirmedAt:   &confirmedAt,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.AppConfig{Key: "security.stepUpMfa.enabled", Value: "true"}).Error; err != nil {
		t.Fatal(err)
	}
	assertion := model.StepUpAssertion{
		ID:                "mfaas_admin_role_guard_" + suffix,
		UserID:            user.ID,
		SessionID:         session.ID,
		Purpose:           stepUpPurposeUserAdminUpdate,
		VerifiedAt:        now,
		LastActivityAt:    now,
		IdleExpiresAt:     now.Add(10 * time.Minute),
		AbsoluteExpiresAt: now.Add(time.Hour),
	}
	if err := db.Create(&assertion).Error; err != nil {
		t.Fatal(err)
	}

	recorder, ctx := newMFAIntegrationContext(http.MethodPut, "/api/v1/users/"+user.ID, map[string]any{
		"email":    user.Email,
		"name":     user.Name,
		"role":     "user",
		"language": user.Language,
		"disabled": false,
	}, sessionToken)
	ctx.Params = gin.Params{{Key: "userId", Value: user.ID}}
	handlers.UpdateUser(ctx)
	if recorder.Code != http.StatusConflict || jsonString(t, recorder.Body.Bytes(), "code") != "mfa.last_admin_required" {
		t.Fatalf("last MFA administrator demotion = %d %s", recorder.Code, recorder.Body.String())
	}
	var stored model.User
	if err := db.First(&stored, "id = ?", user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Role != "platform_admin" || stored.Disabled {
		t.Fatalf("last MFA administrator changed despite guard: role=%s disabled=%t", stored.Role, stored.Disabled)
	}
}

func TestUserUpdateDoesNotRequireAssertionWhenStepUpPolicyIsDisabled(t *testing.T) {
	db := newMFAIntegrationDB(t)
	limitMFAIntegrationConnections(t, db, 1)
	now := time.Now()
	suffix := randomHex(4)
	actor := model.User{
		ID:       "usr_policy_off_actor_" + suffix,
		Email:    "policy-off-actor-" + suffix + "@example.com",
		Name:     "Policy Off Actor",
		AuthType: "local",
		Role:     "platform_admin",
		Language: "en-US",
	}
	target := model.User{
		ID:       "usr_policy_off_target_" + suffix,
		Email:    "policy-off-target-" + suffix + "@example.com",
		Name:     "Policy Off Target",
		AuthType: "local",
		Role:     "user",
		Language: "en-US",
	}
	if err := db.Create(&[]model.User{actor, target}).Error; err != nil {
		t.Fatal(err)
	}
	sessionToken := "sess_policy_off_actor_" + suffix
	if err := db.Create(&model.UserSession{
		ID:        "ses_policy_off_actor_" + suffix,
		UserID:    actor.ID,
		TokenHash: hashToken(sessionToken),
		ExpiresAt: now.Add(time.Hour),
	}).Error; err != nil {
		t.Fatal(err)
	}
	handlers := &Handlers{db: db, configs: newConfigCache(db), mode: "development", rateLimiter: newRateLimiter()}
	recorder, ctx := newMFAIntegrationContext(http.MethodPut, "/api/v1/users/"+target.ID, map[string]any{
		"email":    target.Email,
		"name":     "Updated without Step-up",
		"role":     target.Role,
		"language": target.Language,
		"disabled": false,
	}, sessionToken)
	ctx.Params = gin.Params{{Key: "userId", Value: target.ID}}
	handlers.UpdateUser(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("policy-disabled user update = %d %s", recorder.Code, recorder.Body.String())
	}
	var stored model.User
	if err := db.First(&stored, "id = ?", target.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Name != "Updated without Step-up" {
		t.Fatalf("user name = %q", stored.Name)
	}
}

func TestDisableMFARollsBackWhenSuccessAuditCannotBeWritten(t *testing.T) {
	db := newMFAIntegrationDB(t)
	t.Setenv("APP_ENV", "development")
	t.Setenv("SECRET_ENCRYPTION_KEY", "mfa-audit-rollback-test-key")
	now := time.Now()
	suffix := randomHex(4)
	user := model.User{ID: "usr_audit_rollback_" + suffix, Email: "audit-rollback-" + suffix + "@example.com", Name: "Audit Rollback", AuthType: "local", Role: "user", Language: "en-US"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	sessionToken := "sess_audit_rollback_" + suffix
	session := model.UserSession{ID: "ses_audit_rollback_" + suffix, UserID: user.ID, TokenHash: hashToken(sessionToken), ExpiresAt: now.Add(time.Hour), PrimaryAuthenticatedAt: &now}
	if err := db.Create(&session).Error; err != nil {
		t.Fatal(err)
	}
	handlers := &Handlers{db: db, configs: newConfigCache(db), mode: "development", rateLimiter: newRateLimiter()}
	handlers.secrets = secret.NewStore(db, handlers.audit)
	secretRef := handlers.secrets.Store("AUDITROLLBACKSECRET", user.ID, mfaSecretResource(user.ID))
	confirmedAt := now
	if err := db.Create(&model.UserMFAConfig{ID: "mfa_audit_rollback_" + suffix, UserID: user.ID, TOTPSecretRef: secretRef, Enabled: true, ConfirmedAt: &confirmedAt}).Error; err != nil {
		t.Fatal(err)
	}
	assertion := model.StepUpAssertion{ID: "mfaas_audit_rollback_" + suffix, UserID: user.ID, SessionID: session.ID, Purpose: stepUpPurposeMFAManage, VerifiedAt: now, LastActivityAt: now, IdleExpiresAt: now.Add(10 * time.Minute), AbsoluteExpiresAt: now.Add(time.Hour)}
	if err := db.Create(&assertion).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Migrator().DropTable(&model.AuditLog{}); err != nil {
		t.Fatal(err)
	}

	recorder, ctx := newMFAIntegrationContext(http.MethodDelete, "/api/v1/auth/mfa", nil, sessionToken)
	handlers.DisableMFA(ctx)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("disable without audit table = %d %s", recorder.Code, recorder.Body.String())
	}
	var configCount int64
	if err := db.Model(&model.UserMFAConfig{}).Where("user_id = ?", user.ID).Count(&configCount).Error; err != nil || configCount != 1 {
		t.Fatalf("MFA deletion was not rolled back: count=%d err=%v", configCount, err)
	}
}

func newMFAIntegrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	databaseURL := os.Getenv("AUTH_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("AUTH_TEST_DATABASE_URL is not configured")
	}
	adminDB, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		t.Fatalf("open integration database: %v", err)
	}
	schema := fmt.Sprintf("mfa_api_test_%d", time.Now().UnixNano())
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
	if err := db.AutoMigrate(
		&model.User{},
		&model.UserSession{},
		&model.UserMFAConfig{},
		&model.MFARecoveryCode{},
		&model.StepUpAssertion{},
		&model.SecretValue{},
		&model.AuditLog{},
		&model.AppConfig{},
	); err != nil {
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

func limitMFAIntegrationConnections(t *testing.T, db *gorm.DB, maximum int) {
	t.Helper()
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(maximum)
	sqlDB.SetMaxIdleConns(maximum)
}

func newMFAIntegrationContext(method, path string, body any, sessionToken string) (*httptest.ResponseRecorder, *gin.Context) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	var payload []byte
	if body != nil {
		payload, _ = json.Marshal(body)
	}
	ctx.Request = httptest.NewRequest(method, path, bytes.NewReader(payload))
	digest := sha256.Sum256([]byte(sessionToken))
	ctx.Request.RemoteAddr = fmt.Sprintf("127.%d.%d.%d:12345", digest[0], digest[1], digest[2])
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionToken})
	return recorder, ctx
}

func jsonBool(t *testing.T, data []byte, key string) bool {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatal(err)
	}
	value, _ := body[key].(bool)
	return value
}

func jsonString(t *testing.T, data []byte, key string) string {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatal(err)
	}
	value, _ := body[key].(string)
	return value
}
