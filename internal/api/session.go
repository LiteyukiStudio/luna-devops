package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/redisconfig"
	"github.com/LiteyukiStudio/devops/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const developmentRateLimit = 10000

const currentUserContextKey = "luna.devops.current_user"

const (
	sessionDuration  = 24 * time.Hour
	rememberDuration = 30 * 24 * time.Hour
)

var (
	errRememberTokenInvalid = errors.New("remember token is invalid or expired")
	errRememberTokenReused  = errors.New("remember token reuse detected")
	errRememberUserDisabled = errors.New("remember token user is unavailable")
)

func (h *Handlers) currentUser(ctx *gin.Context) (model.User, bool) {
	if user, ok := currentUserFromContext(ctx); ok {
		return user, true
	}
	if strings.HasPrefix(strings.ToLower(ctx.GetHeader("Authorization")), "bearer ") {
		user, ok := h.currentUserFromAccessToken(ctx)
		if ok {
			ctx.Set(currentUserContextKey, user)
		}
		return user, ok
	}

	plainToken, err := ctx.Cookie(sessionCookieName)
	if err != nil {
		writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.session.missing")
		return model.User{}, false
	}

	var session model.UserSession
	err = h.db.First(&session, "token_hash = ? and expires_at > ?", hashToken(plainToken), time.Now()).Error
	if err != nil {
		clearSessionCookie(ctx)
		writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.session.expired")
		return model.User{}, false
	}

	var user model.User
	if err := h.db.First(&user, "id = ? and disabled = ?", session.UserID, false).Error; err != nil {
		clearSessionCookie(ctx)
		writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.account.disabled")
		return model.User{}, false
	}

	ctx.Set(currentUserContextKey, user)
	return user, true
}

func currentUserFromContext(ctx *gin.Context) (model.User, bool) {
	value, exists := ctx.Get(currentUserContextKey)
	if !exists {
		return model.User{}, false
	}
	user, ok := value.(model.User)
	return user, ok && user.ID != ""
}

func (h *Handlers) platformAdminMiddleware() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		user, ok := h.currentUser(ctx)
		if !ok {
			ctx.Abort()
			return
		}
		if user.Role != "platform_admin" {
			writeErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
			ctx.Abort()
			return
		}
		ctx.Next()
	}
}

func (h *Handlers) currentSessionFromCookie(ctx *gin.Context) (model.UserSession, bool) {
	plainToken, err := ctx.Cookie(sessionCookieName)
	if err != nil {
		return model.UserSession{}, false
	}

	var session model.UserSession
	err = h.db.First(&session, "token_hash = ? and expires_at > ?", hashToken(plainToken), time.Now()).Error
	if err != nil {
		return model.UserSession{}, false
	}

	return session, true
}

func (h *Handlers) currentUserFromAccessToken(ctx *gin.Context) (model.User, bool) {
	header := ctx.GetHeader("Authorization")
	if !strings.HasPrefix(strings.ToLower(header), "bearer ") {
		return model.User{}, false
	}

	plainToken := strings.TrimSpace(header[len("Bearer "):])
	var token model.AccessToken
	err := h.db.First(
		&token,
		"token_hash = ? and revoked_at is null and (expires_at is null or expires_at > ?)",
		hashToken(plainToken),
		time.Now(),
	).Error
	if err != nil || !accessTokenAllows(token.Scope, requiredScopeForRequest(ctx)) {
		writeError(ctx, http.StatusForbidden, "Access Token scope 不足或已失效")
		return model.User{}, false
	}

	var user model.User
	if err := h.db.First(&user, "id = ? and disabled = ?", token.UserID, false).Error; err != nil {
		writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.account.disabled")
		return model.User{}, false
	}

	return user, true
}

func requiredScopeForRequest(ctx *gin.Context) string {
	return service.RequiredAccessTokenScope(ctx.FullPath(), ctx.Request.Method)
}

func accessTokenAllows(scopeText, required string) bool {
	return service.AccessTokenAllows(scopeText, required)
}

func (h *Handlers) hasPlatformAdmin() bool {
	exists, err := platformAdminExists(h.db)
	return err == nil && exists
}

func platformAdminExists(db *gorm.DB) (bool, error) {
	var count int64
	if err := db.Model(&model.User{}).Where("role = ? and disabled = ?", "platform_admin", false).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (h *Handlers) requirePlatformAdmin(ctx *gin.Context) bool {
	user, ok := h.currentUser(ctx)
	if !ok {
		return false
	}
	if user.Role != "platform_admin" {
		writeErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
		return false
	}
	return true
}

func (h *Handlers) createSession(ctx *gin.Context, userID string) bool {
	return h.createSessionWithImpersonation(ctx, userID, "")
}

func (h *Handlers) createSessionWithImpersonation(ctx *gin.Context, userID string, impersonatorID string) bool {
	session, plainToken := newUserSession(userID, impersonatorID, time.Now())
	if err := h.db.Create(&session).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return false
	}

	setSessionCookie(ctx, plainToken, h.mode == "production", false)
	return true
}

func newUserSession(userID, impersonatorID string, now time.Time) (model.UserSession, string) {
	return newUserSessionInFamilyWithPrimaryAuthentication(userID, impersonatorID, "", now, &now, now.Add(sessionDuration))
}

func newUserSessionInFamily(userID, impersonatorID, familyID string, now time.Time) (model.UserSession, string) {
	return newUserSessionInFamilyWithPrimaryAuthentication(userID, impersonatorID, familyID, now, &now, now.Add(sessionDuration))
}

func newUserSessionInFamilyWithPrimaryAuthentication(userID, impersonatorID, familyID string, now time.Time, primaryAuthenticatedAt *time.Time, expiresAt time.Time) (model.UserSession, string) {
	plainToken := "sess_" + randomHex(32)
	return model.UserSession{
		ID:                     id.New("ses"),
		UserID:                 userID,
		ImpersonatorID:         impersonatorID,
		RememberFamilyID:       familyID,
		PrimaryAuthenticatedAt: cloneTime(primaryAuthenticatedAt),
		TokenHash:              hashToken(plainToken),
		ExpiresAt:              expiresAt,
	}, plainToken
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func newUserRememberToken(userID string, now time.Time) (model.UserRememberToken, string) {
	return newUserRememberTokenInFamily(userID, id.New("remf"), now.Add(rememberDuration))
}

func newUserRememberTokenInFamily(userID, familyID string, expiresAt time.Time) (model.UserRememberToken, string) {
	plainToken := "rem_" + randomHex(32)
	return model.UserRememberToken{
		ID:        id.New("rem"),
		UserID:    userID,
		FamilyID:  familyID,
		TokenHash: hashToken(plainToken),
		ExpiresAt: expiresAt,
	}, plainToken
}

func (h *Handlers) createLoginCredentials(ctx *gin.Context, userID string, remember bool) bool {
	if !remember {
		return h.createSession(ctx, userID)
	}

	now := time.Now()
	rememberToken, rememberPlainToken := newUserRememberToken(userID, now)
	session, sessionPlainToken := newUserSessionInFamily(userID, "", rememberToken.FamilyID, now)
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&rememberToken).Error; err != nil {
			return err
		}
		return tx.Create(&session).Error
	}); err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return false
	}

	setSessionCookie(ctx, sessionPlainToken, h.mode == "production", true)
	setRememberCookie(ctx, userID, rememberPlainToken, h.mode == "production")
	return true
}

// Calls that omit requested default to no persistent login. OIDC currently
// uses that default until it exposes an explicit remember choice.
func (h *Handlers) createRememberToken(ctx *gin.Context, userID string, requested ...bool) bool {
	if len(requested) == 0 || !requested[0] {
		return true
	}
	plainSessionToken, err := ctx.Cookie(sessionCookieName)
	if err != nil {
		writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.session.missing")
		return false
	}
	now := time.Now()
	rememberToken, rememberPlainToken := newUserRememberToken(userID, now)
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.UserSession{}).
			Where("user_id = ? and token_hash = ?", userID, hashToken(plainSessionToken)).
			Update("remember_family_id", rememberToken.FamilyID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return tx.Create(&rememberToken).Error
	}); err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return false
	}
	setSessionCookie(ctx, plainSessionToken, h.mode == "production", true)
	setRememberCookie(ctx, userID, rememberPlainToken, h.mode == "production")
	return true
}

func (h *Handlers) rotateRememberLogin(userID, plainToken string) (model.User, string, string, error) {
	now := time.Now()
	if err := h.cleanupExpiredRememberTokenFamilies(userID, now); err != nil {
		return model.User{}, "", "", err
	}
	var newSessionToken string
	var newRememberToken string
	var user model.User
	reused := false

	err := h.db.Transaction(func(tx *gorm.DB) error {
		var lockedUser model.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedUser, "id = ?", userID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errRememberTokenInvalid
			}
			return err
		}
		var current model.UserRememberToken
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(
			&current,
			"token_hash = ? and user_id = ?",
			hashToken(plainToken),
			userID,
		).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errRememberTokenInvalid
			}
			return err
		}
		if current.RevokedAt != nil || !current.ExpiresAt.After(now) {
			return errRememberTokenInvalid
		}
		if current.ConsumedAt != nil {
			if err := revokeRememberFamily(tx, current.UserID, current.FamilyID, now); err != nil {
				return err
			}
			reused = true
			return nil
		}
		if lockedUser.Disabled {
			return errRememberUserDisabled
		}
		user = lockedUser
		var familySessions []model.UserSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? and remember_family_id = ?", current.UserID, current.FamilyID).
			Order("created_at asc, id asc").
			Find(&familySessions).Error; err != nil {
			return err
		}
		primaryAuthenticatedAt := earliestPrimaryAuthentication(familySessions)
		result := tx.Model(&model.UserRememberToken{}).
			Where("id = ? and consumed_at is null and revoked_at is null", current.ID).
			Update("consumed_at", now)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errRememberTokenInvalid
		}
		newRemember, plainRemember := newUserRememberTokenInFamily(current.UserID, current.FamilyID, current.ExpiresAt)
		sessionExpiresAt := now.Add(sessionDuration)
		if current.ExpiresAt.Before(sessionExpiresAt) {
			sessionExpiresAt = current.ExpiresAt
		}
		newSession, plainSession := newUserSessionInFamilyWithPrimaryAuthentication(current.UserID, "", current.FamilyID, now, primaryAuthenticatedAt, sessionExpiresAt)
		if err := tx.Create(&newRemember).Error; err != nil {
			return err
		}
		if err := tx.Create(&newSession).Error; err != nil {
			return err
		}
		if err := deleteRememberFamilySessionsExcept(tx, current.UserID, current.FamilyID, newSession.ID); err != nil {
			return err
		}
		newRememberToken = plainRemember
		newSessionToken = plainSession
		return nil
	})
	if err != nil {
		return model.User{}, "", "", err
	}
	if reused {
		return model.User{}, "", "", errRememberTokenReused
	}
	return user, newSessionToken, newRememberToken, nil
}

func earliestPrimaryAuthentication(sessions []model.UserSession) *time.Time {
	var earliest *time.Time
	for _, session := range sessions {
		if session.PrimaryAuthenticatedAt == nil || session.PrimaryAuthenticatedAt.IsZero() {
			continue
		}
		if earliest == nil || session.PrimaryAuthenticatedAt.Before(*earliest) {
			earliest = cloneTime(session.PrimaryAuthenticatedAt)
		}
	}
	return earliest
}

func deleteRememberFamilySessionsExcept(tx *gorm.DB, userID, familyID, keepSessionID string) error {
	query := tx.Model(&model.UserSession{}).Where("user_id = ? and remember_family_id = ?", userID, familyID)
	if strings.TrimSpace(keepSessionID) != "" {
		query = query.Where("id <> ?", keepSessionID)
	}
	var sessionIDs []string
	if err := query.Pluck("id", &sessionIDs).Error; err != nil {
		return err
	}
	if len(sessionIDs) == 0 {
		return nil
	}
	if err := tx.Where("session_id in ?", sessionIDs).Delete(&model.StepUpAssertion{}).Error; err != nil {
		return err
	}
	return tx.Where("id in ?", sessionIDs).Delete(&model.UserSession{}).Error
}

func (h *Handlers) cleanupExpiredRememberTokenFamilies(userID string, now time.Time) error {
	return h.db.Transaction(func(tx *gorm.DB) error {
		var families []struct {
			FamilyID string
		}
		if err := tx.Model(&model.UserRememberToken{}).
			Select("family_id").
			Where("user_id = ? and family_id <> ''", userID).
			Group("family_id").
			Having("max(expires_at) <= ?", now).
			Scan(&families).Error; err != nil {
			return err
		}
		for _, family := range families {
			if err := deleteRememberFamilySessionsExcept(tx, userID, family.FamilyID, ""); err != nil {
				return err
			}
			if err := tx.Where("user_id = ? and family_id = ?", userID, family.FamilyID).Delete(&model.UserRememberToken{}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func revokeRememberFamily(tx *gorm.DB, userID, familyID string, revokedAt time.Time) error {
	if strings.TrimSpace(familyID) == "" {
		return errRememberTokenInvalid
	}
	if err := tx.Model(&model.UserRememberToken{}).
		Where("user_id = ? and family_id = ? and revoked_at is null", userID, familyID).
		Update("revoked_at", revokedAt).Error; err != nil {
		return err
	}
	var sessionIDs []string
	if err := tx.Model(&model.UserSession{}).
		Where("user_id = ? and remember_family_id = ?", userID, familyID).
		Pluck("id", &sessionIDs).Error; err != nil {
		return err
	}
	if len(sessionIDs) > 0 {
		if err := tx.Where("session_id in ?", sessionIDs).Delete(&model.StepUpAssertion{}).Error; err != nil {
			return err
		}
	}
	return tx.Where("user_id = ? and remember_family_id = ?", userID, familyID).Delete(&model.UserSession{}).Error
}

func revokeUserAuthentication(tx *gorm.DB, userID string) error {
	if err := tx.Where("user_id = ?", userID).Delete(&model.StepUpAssertion{}).Error; err != nil {
		return err
	}
	if err := tx.Model(&model.UserRememberToken{}).
		Where("user_id = ? and revoked_at is null", userID).
		Update("revoked_at", time.Now()).Error; err != nil {
		return err
	}
	return tx.Where("user_id = ?", userID).Delete(&model.UserSession{}).Error
}

func (h *Handlers) revokeCurrentSessionAndRememberTokens(plainToken string) (string, error) {
	userID := ""
	err := h.db.Transaction(func(tx *gorm.DB) error {
		var session model.UserSession
		if err := tx.First(&session, "token_hash = ?", hashToken(plainToken)).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		userID = session.UserID
		if strings.TrimSpace(session.RememberFamilyID) != "" {
			return revokeRememberFamily(tx, session.UserID, session.RememberFamilyID, time.Now())
		}
		if err := tx.Where("session_id = ?", session.ID).Delete(&model.StepUpAssertion{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", session.ID).Delete(&model.UserSession{}).Error
	})
	return userID, err
}

func setSessionCookie(ctx *gin.Context, token string, secure bool, persistent bool) {
	maxAge := 0
	if persistent {
		maxAge = int(sessionDuration / time.Second)
	}
	ctx.SetSameSite(http.SameSiteLaxMode)
	ctx.SetCookie(sessionCookieName, token, maxAge, "/", "", secure, true)
}

func setRememberCookie(ctx *gin.Context, userID string, token string, secure bool) {
	ctx.SetSameSite(http.SameSiteLaxMode)
	ctx.SetCookie(rememberCookieNameForUser(userID), token, 30*86400, "/", "", secure, true)
}

func clearSessionCookie(ctx *gin.Context) {
	ctx.SetSameSite(http.SameSiteLaxMode)
	ctx.SetCookie(sessionCookieName, "", -1, "/", "", false, true)
}

func clearRememberCookie(ctx *gin.Context, userID string) {
	if strings.TrimSpace(userID) == "" {
		return
	}
	ctx.SetSameSite(http.SameSiteLaxMode)
	ctx.SetCookie(rememberCookieNameForUser(userID), "", -1, "/", "", false, true)
}

func rememberCookieNameForUser(userID string) string {
	return rememberCookiePrefix + strings.NewReplacer("-", "_", ".", "_", ":", "_").Replace(userID)
}

type rateLimiter struct {
	redis *redis.Client
}

var incrementRateLimitScript = redis.NewScript(`
local count = redis.call("INCR", KEYS[1])
if redis.call("PTTL", KEYS[1]) < 0 then
  redis.call("PEXPIRE", KEYS[1], ARGV[1])
end
return count
`)

func newRateLimiter(redisAddr ...string) *rateLimiter {
	addr := ""
	if len(redisAddr) > 0 {
		addr = strings.TrimSpace(redisAddr[0])
	}
	return newRateLimiterWithRedis(redisconfig.Options{Addr: addr})
}

func newRateLimiterWithRedis(options redisconfig.Options) *rateLimiter {
	return &rateLimiter{redis: redis.NewClient(options.GoRedis())}
}

func (l *rateLimiter) allow(key string, limit int, window time.Duration) (bool, error) {
	if limit < 1 || window <= 0 {
		return false, errors.New("rate limit and window must be positive")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	redisKey := "rate_limit:" + key
	count, err := incrementRateLimitScript.Run(ctx, l.redis, []string{redisKey}, window.Milliseconds()).Int64()
	if err != nil {
		return false, err
	}
	return count <= int64(limit), nil
}

func (l *rateLimiter) reset(key string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	return l.redis.Del(ctx, "rate_limit:"+key).Err()
}

func (h *Handlers) allowSensitiveAuthAttempt(ctx *gin.Context, action string, limit int, window time.Duration) bool {
	return h.allowSensitiveAuthKey(ctx, action, ctx.ClientIP(), limit, window)
}

func (h *Handlers) allowLoginAccountAttempt(ctx *gin.Context, account string, limit int, window time.Duration) bool {
	normalizedAccount := strings.ToLower(strings.TrimSpace(account))
	return h.allowSensitiveAuthKey(ctx, "login_account", hashToken(normalizedAccount), limit, window)
}

func (h *Handlers) allowSensitiveAuthKey(ctx *gin.Context, action, subject string, limit int, window time.Duration) bool {
	if h.rateLimiter == nil {
		h.rateLimiter = newRateLimiter()
	}
	if h.mode == "development" && limit < developmentRateLimit {
		limit = developmentRateLimit
	}
	key := action + ":" + subject
	allowed, err := h.rateLimiter.allow(key, limit, window)
	if allowed {
		return true
	}
	if err != nil && h.mode == "development" {
		return true
	}
	writeError(ctx, http.StatusTooManyRequests, "请求过于频繁，请稍后再试")
	return false
}
