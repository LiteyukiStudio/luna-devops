package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	mfaSecretResourcePrefix = "mfa:"
	// OIDC enrollment requires a browser session created by primary login within this server-side window.
	mfaEnrollmentOIDCSessionMaxAge = 5 * time.Minute
)

type mfaEnrollInput struct {
	CurrentPassword string `json:"currentPassword"`
}

type mfaConfirmInput struct {
	Code string `json:"code" binding:"required"`
}

type mfaVerifyInput struct {
	Purpose      string `json:"purpose" binding:"required"`
	Code         string `json:"code"`
	RecoveryCode string `json:"recoveryCode"`
}

func (h *Handlers) GetMFAStatus(ctx *gin.Context) {
	user, _, ok := h.currentMFAUserSession(ctx)
	if !ok {
		return
	}

	var config model.UserMFAConfig
	err := h.db.First(&config, "user_id = ?", user.ID).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	remaining := int64(0)
	if config.Enabled {
		if err := h.db.Model(&model.MFARecoveryCode{}).Where("user_id = ? and used_at is null", user.ID).Count(&remaining).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		}
	}
	ctx.JSON(http.StatusOK, gin.H{
		"enabled":                config.Enabled,
		"pending":                config.ID != "" && !config.Enabled,
		"policyEnabled":          h.stepUpMFAEnabled(),
		"enrollmentReauthMode":   mfaEnrollmentReauthMode(user),
		"confirmedAt":            config.ConfirmedAt,
		"recoveryCodesRemaining": remaining,
	})
}

func (h *Handlers) EnrollMFA(ctx *gin.Context) {
	user, session, ok := h.currentMFAUserSession(ctx)
	if !ok || !h.allowMFAAttempt(ctx, user.ID, "enroll", 3, time.Hour) {
		return
	}
	var input mfaEnrollInput
	if !bindJSON(ctx, &input) {
		return
	}
	if !h.reauthenticateMFAEnrollment(ctx, user, session, input.CurrentPassword, time.Now()) {
		return
	}

	var existing model.UserMFAConfig
	if err := h.db.First(&existing, "user_id = ?", user.ID).Error; err == nil && existing.Enabled {
		h.audit(user.ID, "mfa.enroll", user.ID, false, "MFA already enabled")
		writeErrorCode(ctx, http.StatusConflict, "mfa.already_enabled", "MFA 已启用，请先解绑后再重新绑定")
		return
	} else if err != nil && err != gorm.ErrRecordNotFound {
		h.audit(user.ID, "mfa.enroll", user.ID, false, "failed to inspect MFA enrollment")
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	enrollment, err := generateTOTPEnrollment(user.Email)
	if err != nil {
		h.audit(user.ID, "mfa.enroll", user.ID, false, "failed to generate TOTP enrollment")
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	resource := mfaSecretResource(user.ID)
	_ = h.db.Where("resource = ?", resource).Delete(&model.SecretValue{}).Error
	secretRef := h.secrets.Store(enrollment.Secret, user.ID, resource)
	if secretRef == "" {
		h.audit(user.ID, "mfa.enroll", user.ID, false, "failed to store TOTP secret")
		writeErrorCode(ctx, http.StatusInternalServerError, "mfa.secret_store_failed", "无法安全保存 TOTP 密钥")
		return
	}

	config := model.UserMFAConfig{
		ID:            id.New("mfa"),
		UserID:        user.ID,
		TOTPSecretRef: secretRef,
		Enabled:       false,
	}
	err = h.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"totp_secret_ref":             secretRef,
			"enabled":                     false,
			"confirmed_at":                nil,
			"recovery_codes_generated_at": nil,
			"last_totp_counter":           nil,
			"updated_at":                  time.Now(),
		}),
	}).Create(&config).Error
	if err != nil {
		_ = h.db.Where("resource = ?", resource).Delete(&model.SecretValue{}).Error
		h.audit(user.ID, "mfa.enroll", user.ID, false, "failed to persist MFA enrollment")
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	_ = h.db.Where("user_id = ?", user.ID).Delete(&model.MFARecoveryCode{}).Error
	_ = h.db.Where("user_id = ?", user.ID).Delete(&model.StepUpAssertion{}).Error
	h.audit(user.ID, "mfa.enroll", user.ID, true, "TOTP enrollment created")
	ctx.JSON(http.StatusCreated, gin.H{
		"secret":        enrollment.Secret,
		"otpauthUrl":    enrollment.OTPAuthURL,
		"qrCodeDataUrl": enrollment.QRCodeDataURL,
	})
}

func (h *Handlers) ConfirmMFA(ctx *gin.Context) {
	user, _, ok := h.currentMFAUserSession(ctx)
	if !ok || !h.allowMFAAttempt(ctx, user.ID, "confirm", 6, 5*time.Minute) {
		return
	}
	var input mfaConfirmInput
	if !bindJSON(ctx, &input) {
		return
	}

	now := time.Now()
	var pending model.UserMFAConfig
	if err := h.db.First(&pending, "user_id = ?", user.ID).Error; err != nil {
		h.audit(user.ID, "mfa.confirm", user.ID, false, mfaAuditFailure(err))
		writeMFAError(ctx, err)
		return
	}
	if pending.Enabled {
		h.audit(user.ID, "mfa.confirm", user.ID, false, mfaAuditFailure(errMFAAlreadyEnabled))
		writeMFAError(ctx, errMFAAlreadyEnabled)
		return
	}
	secretValue := h.secrets.Resolve(pending.TOTPSecretRef)
	counter, valid := matchTOTPCounter(secretValue, input.Code, now)
	if !valid {
		h.audit(user.ID, "mfa.confirm", user.ID, false, mfaAuditFailure(errMFAInvalidCode))
		writeMFAError(ctx, errMFAInvalidCode)
		return
	}
	codes, hashes, err := generateRecoveryCodes()
	if err != nil {
		h.audit(user.ID, "mfa.confirm", user.ID, false, "failed to generate recovery codes")
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	err = h.db.Transaction(func(tx *gorm.DB) error {
		var config model.UserMFAConfig
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&config, "user_id = ?", user.ID).Error; err != nil {
			return err
		}
		if config.Enabled {
			return errMFAAlreadyEnabled
		}
		if config.TOTPSecretRef != pending.TOTPSecretRef {
			return errMFAEnrollmentChanged
		}
		if config.LastTOTPCounter != nil && counter <= *config.LastTOTPCounter {
			return errMFAInvalidCode
		}
		if err := tx.Where("user_id = ?", user.ID).Delete(&model.MFARecoveryCode{}).Error; err != nil {
			return err
		}
		rows := recoveryCodeRows(user.ID, hashes, now)
		if err := tx.Create(&rows).Error; err != nil {
			return err
		}
		return tx.Model(&config).Updates(map[string]any{
			"enabled":                     true,
			"confirmed_at":                now,
			"recovery_codes_generated_at": now,
			"last_totp_counter":           counter,
			"updated_at":                  now,
		}).Error
	})
	if err != nil {
		h.audit(user.ID, "mfa.confirm", user.ID, false, mfaAuditFailure(err))
		writeMFAError(ctx, err)
		return
	}
	h.audit(user.ID, "mfa.confirm", user.ID, true, "TOTP enrollment confirmed")
	ctx.JSON(http.StatusOK, gin.H{"enabled": true, "recoveryCodes": codes})
}

func (h *Handlers) VerifyMFA(ctx *gin.Context) {
	user, session, ok := h.currentMFAUserSession(ctx)
	if !ok || !h.allowMFAAttempt(ctx, user.ID, "verify", 6, 5*time.Minute) {
		return
	}
	var input mfaVerifyInput
	if !bindJSON(ctx, &input) {
		return
	}
	purpose := normalizeStepUpPurpose(input.Purpose)
	if purpose == "" {
		h.audit(user.ID, "mfa.verify", "unknown", false, "invalid purpose")
		writeErrorCode(ctx, http.StatusBadRequest, "mfa.invalid_purpose", "不支持的二次验证用途")
		return
	}
	code := strings.TrimSpace(input.Code)
	recoveryCode := normalizeRecoveryCode(input.RecoveryCode)
	if (code == "") == (recoveryCode == "") {
		h.audit(user.ID, "mfa.verify", purpose, false, "exactly one MFA credential is required")
		writeErrorCode(ctx, http.StatusBadRequest, "mfa.credential_required", "必须且只能提供动态验证码或恢复码之一")
		return
	}

	var config model.UserMFAConfig
	if err := h.db.First(&config, "user_id = ? and enabled = ?", user.ID, true).Error; err != nil {
		h.audit(user.ID, "mfa.verify", purpose, false, "MFA is not enabled")
		writeErrorCode(ctx, http.StatusConflict, "mfa.not_enabled", "当前账号尚未启用 MFA")
		return
	}

	usedRecoveryCode := false
	valid := false
	if code != "" {
		secretValue := h.secrets.Resolve(config.TOTPSecretRef)
		valid = secretValue != "" && h.consumeTOTPCode(user.ID, config.TOTPSecretRef, secretValue, code, time.Now())
	} else {
		valid = h.consumeRecoveryCode(user.ID, recoveryCode)
		usedRecoveryCode = valid
	}
	if !valid {
		h.audit(user.ID, "mfa.verify", purpose, false, "invalid MFA credential")
		writeErrorCode(ctx, http.StatusUnauthorized, "mfa.invalid_code", "动态验证码或恢复码无效")
		return
	}

	if err := h.createStepUpAssertion(user.ID, session.ID, purpose, time.Now()); err != nil {
		h.audit(user.ID, "mfa.verify", purpose, false, "failed to persist assertion")
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if usedRecoveryCode {
		h.audit(user.ID, "mfa.recovery_code_used", purpose, true, "one-time recovery code consumed")
	}
	h.audit(user.ID, "mfa.verify", purpose, true, "step-up assertion created")
	ctx.JSON(http.StatusOK, gin.H{"verified": true, "purpose": purpose})
}

func (h *Handlers) RegenerateMFARecoveryCodes(ctx *gin.Context) {
	user, _, ok := h.currentMFAUserSession(ctx)
	if !ok || !h.requireMFAAssertion(ctx, user, stepUpPurposeMFAManage) {
		return
	}
	var config model.UserMFAConfig
	if err := h.db.First(&config, "user_id = ? and enabled = ?", user.ID, true).Error; err != nil {
		h.audit(user.ID, "mfa.recovery_codes_regenerate", user.ID, false, "MFA is not enabled")
		writeErrorCode(ctx, http.StatusConflict, "mfa.not_enabled", "当前账号尚未启用 MFA")
		return
	}
	codes, hashes, err := generateRecoveryCodes()
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	now := time.Now()
	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", user.ID).Delete(&model.MFARecoveryCode{}).Error; err != nil {
			return err
		}
		rows := recoveryCodeRows(user.ID, hashes, now)
		if err := tx.Create(&rows).Error; err != nil {
			return err
		}
		return tx.Model(&model.UserMFAConfig{}).Where("user_id = ?", user.ID).Updates(map[string]any{
			"recovery_codes_generated_at": now,
			"updated_at":                  now,
		}).Error
	})
	if err != nil {
		h.audit(user.ID, "mfa.recovery_codes_regenerate", user.ID, false, "failed to replace recovery codes")
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(user.ID, "mfa.recovery_codes_regenerate", user.ID, true, "recovery codes replaced")
	ctx.JSON(http.StatusOK, gin.H{"recoveryCodes": codes})
}

func (h *Handlers) DisableMFA(ctx *gin.Context) {
	user, _, ok := h.currentMFAUserSession(ctx)
	if !ok || !h.requireMFAAssertion(ctx, user, stepUpPurposeMFAManage) {
		return
	}
	if h.stepUpMFAEnabled() && user.Role == "platform_admin" && !h.hasAnotherMFAEnabledPlatformAdmin(user.ID) {
		h.audit(user.ID, "mfa.disable", user.ID, false, "last MFA-enabled platform admin cannot disable MFA")
		writeErrorCode(ctx, http.StatusConflict, "mfa.last_admin_required", "全局二次验证开启时必须保留至少一名已绑定 MFA 的平台管理员")
		return
	}
	resource := mfaSecretResource(user.ID)
	err := h.db.Transaction(func(tx *gorm.DB) error {
		return deleteUserMFAState(tx, user.ID, resource)
	})
	if err != nil {
		h.audit(user.ID, "mfa.disable", user.ID, false, "failed to disable MFA")
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(user.ID, "mfa.disable", user.ID, true, "MFA disabled and assertions revoked")
	ctx.Status(http.StatusNoContent)
}

func (h *Handlers) AdminResetUserMFA(ctx *gin.Context) {
	actor, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if actor.Role != "platform_admin" {
		writeErrorKey(ctx, http.StatusForbidden, actor.Language, "config.admin.required")
		return
	}
	if !h.requireMFAAssertion(ctx, actor, stepUpPurposeUserAdminUpdate) {
		return
	}

	targetID := strings.TrimSpace(ctx.Param("userId"))
	if targetID == actor.ID {
		h.audit(actor.ID, "mfa.admin_reset", targetID, false, "administrators must manage their own MFA from account settings")
		writeErrorCode(ctx, http.StatusConflict, "mfa.admin_reset_self_forbidden", "请从个人安全设置管理当前账号的 MFA")
		return
	}

	err := h.db.Transaction(func(tx *gorm.DB) error {
		var target model.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&target, "id = ?", targetID).Error; err != nil {
			return err
		}
		var config model.UserMFAConfig
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&config, "user_id = ?", target.ID).Error; err != nil {
			return err
		}
		if h.stepUpMFAEnabled() && target.Role == "platform_admin" && config.Enabled {
			var enabledAdmins []model.UserMFAConfig
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Table("user_mfa_configs").
				Select("user_mfa_configs.*").
				Joins("join users on users.id = user_mfa_configs.user_id").
				Where("users.role = ? and users.disabled = ? and users.deleted_at is null and user_mfa_configs.enabled = ?", "platform_admin", false, true).
				Find(&enabledAdmins).Error; err != nil {
				return err
			}
			otherEnabledAdmin := false
			for _, adminConfig := range enabledAdmins {
				if adminConfig.UserID != target.ID {
					otherEnabledAdmin = true
					break
				}
			}
			if !otherEnabledAdmin {
				return errMFALastAdminRequired
			}
		}
		return deleteUserMFAState(tx, target.ID, mfaSecretResource(target.ID))
	})
	if err != nil {
		h.audit(actor.ID, "mfa.admin_reset", targetID, false, mfaAuditFailure(err))
		switch err {
		case errMFALastAdminRequired:
			writeErrorCode(ctx, http.StatusConflict, "mfa.last_admin_required", "全局二次验证开启时必须保留至少一名已绑定 MFA 的平台管理员")
		case gorm.ErrRecordNotFound:
			writeErrorCode(ctx, http.StatusNotFound, "mfa.reset_target_not_found", "用户或 MFA 配置不存在")
		default:
			writeError(ctx, http.StatusInternalServerError, err.Error())
		}
		return
	}

	h.audit(actor.ID, "mfa.admin_reset", targetID, true, "target MFA credentials and assertions deleted")
	ctx.Status(http.StatusNoContent)
}

func mfaEnrollmentReauthMode(user model.User) string {
	if strings.EqualFold(strings.TrimSpace(user.AuthType), "local") {
		return "password"
	}
	return "fresh_session"
}

func (h *Handlers) reauthenticateMFAEnrollment(ctx *gin.Context, user model.User, session model.UserSession, currentPassword string, now time.Time) bool {
	if mfaEnrollmentReauthenticated(user, session, currentPassword, now) {
		return true
	}
	if mfaEnrollmentReauthMode(user) == "password" {
		h.audit(user.ID, "mfa.enroll", user.ID, false, "local primary reauthentication failed")
		writeErrorCode(ctx, http.StatusUnauthorized, "mfa.reauth_required", "请输入当前密码后重新验证")
		return false
	}
	h.audit(user.ID, "mfa.enroll", user.ID, false, "OIDC session is not fresh enough for enrollment")
	writeErrorCode(ctx, http.StatusUnauthorized, "mfa.reauth_required", "请重新完成 OIDC 登录后再绑定 MFA")
	return false
}

func mfaEnrollmentReauthenticated(user model.User, session model.UserSession, currentPassword string, now time.Time) bool {
	if mfaEnrollmentReauthMode(user) == "password" {
		return strings.TrimSpace(currentPassword) != "" && bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(currentPassword)) == nil
	}
	age := now.Sub(session.CreatedAt)
	return session.ImpersonatorID == "" && !session.CreatedAt.IsZero() && age >= 0 && age <= mfaEnrollmentOIDCSessionMaxAge
}

func (h *Handlers) currentMFAUserSession(ctx *gin.Context) (model.User, model.UserSession, bool) {
	if requestUsesBearerToken(ctx) {
		writeErrorCode(ctx, http.StatusForbidden, "mfa.session_required", "MFA 管理与验证仅支持浏览器会话")
		return model.User{}, model.UserSession{}, false
	}
	user, ok := h.currentUser(ctx)
	if !ok {
		return model.User{}, model.UserSession{}, false
	}
	session, ok := h.currentSessionFromCookie(ctx)
	if !ok || session.UserID != user.ID {
		writeErrorCode(ctx, http.StatusUnauthorized, "mfa.session_required", "当前浏览器会话无效")
		return model.User{}, model.UserSession{}, false
	}
	return user, session, true
}

func (h *Handlers) allowMFAAttempt(ctx *gin.Context, userID, action string, limit int, window time.Duration) bool {
	if h.rateLimiter == nil {
		h.rateLimiter = newRateLimiter()
	}
	action = strings.TrimSpace(action)
	keys := []struct {
		key   string
		limit int
	}{
		{key: "mfa:" + action + ":user:" + strings.TrimSpace(userID), limit: limit},
		{key: "mfa:" + action + ":ip:" + ctx.ClientIP(), limit: maxInt(limit*5, 20)},
	}
	for _, item := range keys {
		allowed, err := h.rateLimiter.allow(item.key, item.limit, window)
		if err != nil {
			if h.mode == "development" {
				return true
			}
			h.audit(userID, "mfa.rate_limit_unavailable", action, false, "Redis rate limiter unavailable")
			writeErrorCode(ctx, http.StatusServiceUnavailable, "mfa.rate_limit_unavailable", "MFA 安全限流暂时不可用")
			return false
		}
		if !allowed {
			h.audit(userID, "mfa.rate_limited", action, false, "too many MFA attempts")
			writeErrorCode(ctx, http.StatusTooManyRequests, "mfa.rate_limited", "MFA 验证尝试过于频繁")
			return false
		}
	}
	return true
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func (h *Handlers) createStepUpAssertion(userID, sessionID, purpose string, now time.Time) error {
	idleTimeout, absoluteTimeout := h.stepUpTimeouts()
	absoluteExpiresAt := now.Add(absoluteTimeout)
	assertion := model.StepUpAssertion{
		ID:                id.New("mfaas"),
		UserID:            userID,
		SessionID:         sessionID,
		Purpose:           purpose,
		VerifiedAt:        now,
		LastActivityAt:    now,
		IdleExpiresAt:     refreshedStepUpIdleExpiry(now, idleTimeout, absoluteExpiresAt),
		AbsoluteExpiresAt: absoluteExpiresAt,
	}
	return h.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "session_id"}, {Name: "purpose"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"user_id", "verified_at", "last_activity_at", "idle_expires_at", "absolute_expires_at", "updated_at",
		}),
	}).Create(&assertion).Error
}

func (h *Handlers) consumeRecoveryCode(userID, normalizedCode string) bool {
	if len(normalizedCode) != 16 {
		return false
	}
	consumed := false
	err := h.db.Transaction(func(tx *gorm.DB) error {
		var rows []model.MFARecoveryCode
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? and used_at is null", userID).Find(&rows).Error; err != nil {
			return err
		}
		matchedID := ""
		for _, row := range rows {
			if bcrypt.CompareHashAndPassword([]byte(row.CodeHash), []byte(normalizedCode)) == nil {
				matchedID = row.ID
			}
		}
		if matchedID == "" {
			return nil
		}
		now := time.Now()
		result := tx.Model(&model.MFARecoveryCode{}).Where("id = ? and used_at is null", matchedID).Update("used_at", now)
		if result.Error != nil {
			return result.Error
		}
		consumed = result.RowsAffected == 1
		return nil
	})
	return err == nil && consumed
}

func (h *Handlers) consumeTOTPCode(userID, expectedSecretRef, secretValue, code string, now time.Time) bool {
	counter, valid := matchTOTPCounter(secretValue, code, now)
	if !valid {
		return false
	}
	consumed := false
	err := h.db.Transaction(func(tx *gorm.DB) error {
		var config model.UserMFAConfig
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&config, "user_id = ? and enabled = ?", userID, true).Error; err != nil {
			return err
		}
		if config.TOTPSecretRef != expectedSecretRef || (config.LastTOTPCounter != nil && counter <= *config.LastTOTPCounter) {
			return nil
		}
		result := tx.Model(&config).Updates(map[string]any{"last_totp_counter": counter, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		consumed = result.RowsAffected == 1
		return nil
	})
	return err == nil && consumed
}

func deleteUserMFAState(tx *gorm.DB, userID, secretResource string) error {
	if err := tx.Where("user_id = ?", userID).Delete(&model.StepUpAssertion{}).Error; err != nil {
		return err
	}
	if err := tx.Where("user_id = ?", userID).Delete(&model.MFARecoveryCode{}).Error; err != nil {
		return err
	}
	if err := tx.Where("user_id = ?", userID).Delete(&model.UserMFAConfig{}).Error; err != nil {
		return err
	}
	return tx.Where("resource = ?", secretResource).Delete(&model.SecretValue{}).Error
}

func (h *Handlers) hasMFAEnabledPlatformAdmin() bool {
	var count int64
	_ = h.db.Table("users").
		Joins("join user_mfa_configs on user_mfa_configs.user_id = users.id and user_mfa_configs.enabled = ?", true).
		Where("users.role = ? and users.disabled = ? and users.deleted_at is null", "platform_admin", false).
		Count(&count).Error
	return count > 0
}

func (h *Handlers) hasAnotherMFAEnabledPlatformAdmin(excludedUserID string) bool {
	var count int64
	_ = h.db.Table("users").
		Joins("join user_mfa_configs on user_mfa_configs.user_id = users.id and user_mfa_configs.enabled = ?", true).
		Where("users.role = ? and users.disabled = ? and users.deleted_at is null and users.id <> ?", "platform_admin", false, excludedUserID).
		Count(&count).Error
	return count > 0
}

func recoveryCodeRows(userID string, hashes []string, now time.Time) []model.MFARecoveryCode {
	rows := make([]model.MFARecoveryCode, 0, len(hashes))
	for _, hash := range hashes {
		rows = append(rows, model.MFARecoveryCode{ID: id.New("mfr"), UserID: userID, CodeHash: hash, CreatedAt: now})
	}
	return rows
}

func mfaSecretResource(userID string) string {
	return mfaSecretResourcePrefix + strings.TrimSpace(userID) + ":totp"
}

type mfaSentinelError string

func (err mfaSentinelError) Error() string { return string(err) }

const (
	errMFAInvalidCode       = mfaSentinelError("invalid MFA code")
	errMFAAlreadyEnabled    = mfaSentinelError("MFA already enabled")
	errMFAEnrollmentChanged = mfaSentinelError("MFA enrollment changed")
	errMFALastAdminRequired = mfaSentinelError("last MFA-enabled platform admin is required")
)

func writeMFAError(ctx *gin.Context, err error) {
	switch err {
	case errMFAInvalidCode:
		writeErrorCode(ctx, http.StatusUnauthorized, "mfa.invalid_code", "动态验证码无效")
	case errMFAAlreadyEnabled:
		writeErrorCode(ctx, http.StatusConflict, "mfa.already_enabled", "MFA 已启用")
	case errMFAEnrollmentChanged:
		writeErrorCode(ctx, http.StatusConflict, "mfa.enrollment_changed", "MFA 绑定已更新，请重新扫码确认")
	case gorm.ErrRecordNotFound:
		writeErrorCode(ctx, http.StatusConflict, "mfa.enrollment_required", "请先开始 TOTP 绑定")
	default:
		writeError(ctx, http.StatusInternalServerError, err.Error())
	}
}

func mfaAuditFailure(err error) string {
	switch err {
	case errMFAInvalidCode:
		return "invalid TOTP code"
	case errMFAAlreadyEnabled:
		return "MFA already enabled"
	case errMFAEnrollmentChanged:
		return "MFA enrollment changed while confirming"
	case errMFALastAdminRequired:
		return "last MFA-enabled platform admin cannot be reset"
	case gorm.ErrRecordNotFound:
		return "MFA enrollment not found"
	default:
		return "MFA persistence failed"
	}
}
