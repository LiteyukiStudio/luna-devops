package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	stepUpPurposeContextKey = "luna.devops.step_up_purpose"

	stepUpPurposeRuntimeExec              = "runtime_exec"
	stepUpPurposeRuntimeTerminal          = "runtime_terminal"
	stepUpPurposeDataExport               = "data_export"
	stepUpPurposeSecretUpdate             = "secret_update"
	stepUpPurposeRegistryCredentialUpdate = "registry_credential_update"
	stepUpPurposeKubeconfigUpdate         = "kubeconfig_update"
	stepUpPurposeAuthProviderUpdate       = "auth_provider_update"
	stepUpPurposeUserAdminUpdate          = "user_admin_update"
	stepUpPurposeMFAManage                = "mfa_manage"
	stepUpPurposeSecuritySettingsUpdate   = "security_settings_update"
	stepUpPurposeDataRetentionCleanup     = "data_retention_cleanup"
	stepUpPurposePasswordUpdate           = "password_update"

	defaultStepUpIdleTimeout     = 10 * time.Minute
	defaultStepUpAbsoluteTimeout = 60 * time.Minute
)

var allowedStepUpPurposes = map[string]struct{}{
	stepUpPurposeRuntimeExec:              {},
	stepUpPurposeRuntimeTerminal:          {},
	stepUpPurposeDataExport:               {},
	stepUpPurposeSecretUpdate:             {},
	stepUpPurposeRegistryCredentialUpdate: {},
	stepUpPurposeKubeconfigUpdate:         {},
	stepUpPurposeAuthProviderUpdate:       {},
	stepUpPurposeUserAdminUpdate:          {},
	stepUpPurposeMFAManage:                {},
	stepUpPurposeSecuritySettingsUpdate:   {},
	stepUpPurposeDataRetentionCleanup:     {},
	stepUpPurposePasswordUpdate:           {},
}

var errStepUpAuthorizationChanged = errors.New("step-up authorization changed")

func (h *Handlers) requireStepUp(ctx *gin.Context, user model.User, purpose string) bool {
	if !h.stepUpMFAEnabled() {
		return true
	}
	purpose = normalizeStepUpPurpose(purpose)
	if verifiedPurpose, ok := ctx.Get(stepUpPurposeContextKey); ok && verifiedPurpose == purpose && purpose != "" {
		return true
	}
	return h.requireMFAAssertion(ctx, user, purpose)
}

// stepUpMiddleware is used after authentication and coarse route authorization.
// Resource-level and payload-conditional checks stay in handlers so MFA never replaces authorization.
func (h *Handlers) stepUpMiddleware(purpose string) gin.HandlerFunc {
	purpose = normalizeStepUpPurpose(purpose)
	if purpose == "" {
		panic("invalid step-up MFA purpose")
	}
	return func(ctx *gin.Context) {
		user, ok := h.currentUser(ctx)
		if !ok {
			ctx.Abort()
			return
		}
		if !h.requireStepUp(ctx, user, purpose) {
			ctx.Abort()
			return
		}
		ctx.Set(stepUpPurposeContextKey, purpose)
		ctx.Next()
	}
}

func (h *Handlers) requireMFAAssertion(ctx *gin.Context, user model.User, purpose string) bool {
	purpose = normalizeStepUpPurpose(purpose)
	if purpose == "" {
		h.audit(user.ID, "mfa.step_up_rejected", "unknown", false, "invalid purpose")
		writeErrorCode(ctx, http.StatusBadRequest, "mfa.invalid_purpose", "不支持的二次验证用途")
		return false
	}
	if requestUsesBearerToken(ctx) {
		h.audit(user.ID, "mfa.step_up_required", purpose, false, "personal access tokens cannot satisfy step-up MFA")
		writeErrorCode(ctx, http.StatusForbidden, "mfa.session_required", "二次验证仅支持浏览器会话")
		return false
	}

	session, ok := h.currentSessionFromCookie(ctx)
	if !ok || session.UserID != user.ID {
		h.audit(user.ID, "mfa.step_up_required", purpose, false, "missing browser session")
		writeMFARequired(ctx, purpose)
		return false
	}

	now := time.Now()
	h.cleanupExpiredStepUpAssertions(now)
	var assertion model.StepUpAssertion
	err := h.db.First(
		&assertion,
		"user_id = ? and session_id = ? and purpose = ? and idle_expires_at > ? and absolute_expires_at > ?",
		user.ID,
		session.ID,
		purpose,
		now,
		now,
	).Error
	if err != nil || !stepUpAssertionActive(assertion, now) {
		h.audit(user.ID, "mfa.step_up_required", purpose, false, "assertion missing or expired")
		writeMFARequired(ctx, purpose)
		return false
	}

	idleTimeout, _ := h.stepUpTimeouts()
	idleExpiresAt := refreshedStepUpIdleExpiry(now, idleTimeout, assertion.AbsoluteExpiresAt)
	result := h.db.Model(&model.StepUpAssertion{}).
		Where("id = ? and idle_expires_at > ? and absolute_expires_at > ?", assertion.ID, now, now).
		Updates(map[string]any{"last_activity_at": now, "idle_expires_at": idleExpiresAt, "updated_at": now})
	if result.Error != nil || result.RowsAffected != 1 {
		h.audit(user.ID, "mfa.step_up_required", purpose, false, "assertion refresh failed")
		writeMFARequired(ctx, purpose)
		return false
	}
	return true
}

func (h *Handlers) cleanupExpiredStepUpAssertions(now time.Time) {
	_ = h.db.Where("idle_expires_at <= ? or absolute_expires_at <= ?", now, now).Delete(&model.StepUpAssertion{}).Error
}

func (h *Handlers) stepUpMFAEnabled() bool {
	return configBool(h.configValue("security.stepUpMfa.enabled"))
}

func stepUpMFAEnabledInTransaction(tx *gorm.DB) (bool, error) {
	var row model.AppConfig
	err := tx.First(&row, "key = ?", "security.stepUpMfa.enabled").Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return configBool(row.Value), nil
}

func lockActiveUserRole(tx *gorm.DB, userID, requiredRole string) (model.User, error) {
	var user model.User
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, "id = ? and disabled = ?", userID, false).Error; err != nil {
		return model.User{}, errStepUpAuthorizationChanged
	}
	if requiredRole != "" && user.Role != requiredRole {
		return model.User{}, errStepUpAuthorizationChanged
	}
	return user, nil
}

func lockStepUpActor(tx *gorm.DB, userID, sessionID, purpose, requiredRole string) (model.User, error) {
	user, err := lockActiveUserRole(tx, userID, requiredRole)
	if err != nil {
		return model.User{}, err
	}
	now := time.Now()
	var session model.UserSession
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(
		&session,
		"id = ? and user_id = ? and expires_at > ?",
		sessionID,
		userID,
		now,
	).Error; err != nil {
		return model.User{}, errStepUpAuthorizationChanged
	}
	var assertion model.StepUpAssertion
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(
		&assertion,
		"user_id = ? and session_id = ? and purpose = ? and idle_expires_at > ? and absolute_expires_at > ?",
		userID,
		sessionID,
		normalizeStepUpPurpose(purpose),
		now,
		now,
	).Error; err != nil || !stepUpAssertionActive(assertion, now) {
		return model.User{}, errStepUpAuthorizationChanged
	}
	return user, nil
}

func (h *Handlers) stepUpTimeouts() (time.Duration, time.Duration) {
	idle := configMinutes(h.configValue("security.stepUpMfa.idleTimeoutMinutes"), defaultStepUpIdleTimeout)
	absolute := configMinutes(h.configValue("security.stepUpMfa.absoluteTimeoutMinutes"), defaultStepUpAbsoluteTimeout)
	if idle > absolute {
		idle = absolute
	}
	return idle, absolute
}

func configMinutes(value string, fallback time.Duration) time.Duration {
	minutes, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || minutes <= 0 {
		return fallback
	}
	return time.Duration(minutes) * time.Minute
}

func configBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "yes", "on", "enabled":
		return true
	default:
		return false
	}
}

func normalizeStepUpPurpose(purpose string) string {
	purpose = strings.ToLower(strings.TrimSpace(purpose))
	if _, ok := allowedStepUpPurposes[purpose]; !ok {
		return ""
	}
	return purpose
}

func requestUsesBearerToken(ctx *gin.Context) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(ctx.GetHeader("Authorization"))), "bearer ")
}

func writeMFARequired(ctx *gin.Context, purpose string) {
	ctx.JSON(http.StatusForbidden, gin.H{
		"code":    "mfa_required",
		"error":   "需要完成敏感操作二次验证",
		"purpose": purpose,
	})
}

func stepUpAssertionActive(assertion model.StepUpAssertion, now time.Time) bool {
	return assertion.ID != "" && assertion.IdleExpiresAt.After(now) && assertion.AbsoluteExpiresAt.After(now)
}

func refreshedStepUpIdleExpiry(now time.Time, idleTimeout time.Duration, absoluteExpiresAt time.Time) time.Time {
	refreshed := now.Add(idleTimeout)
	if refreshed.After(absoluteExpiresAt) {
		return absoluteExpiresAt
	}
	return refreshed
}
