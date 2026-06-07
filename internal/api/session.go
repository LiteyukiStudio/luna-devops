package api

import (
	"context"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"net/http"
	"strings"
	"time"
)

func (h *Handlers) currentUser(ctx *gin.Context) (model.User, bool) {
	if strings.HasPrefix(strings.ToLower(ctx.GetHeader("Authorization")), "bearer ") {
		return h.currentUserFromAccessToken(ctx)
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

	return user, true
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
	var count int64
	_ = h.db.Model(&model.User{}).Where("role = ? and disabled = ?", "platform_admin", false).Count(&count).Error
	return count > 0
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
	plainToken := "sess_" + randomHex(32)
	session := model.UserSession{
		ID:             id.New("ses"),
		UserID:         userID,
		ImpersonatorID: impersonatorID,
		TokenHash:      hashToken(plainToken),
		ExpiresAt:      time.Now().Add(24 * time.Hour),
	}
	if err := h.db.Create(&session).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return false
	}

	setSessionCookie(ctx, plainToken, h.mode == "production")
	return true
}

func (h *Handlers) createRememberToken(ctx *gin.Context, userID string) bool {
	_ = h.db.Where("expires_at <= ?", time.Now()).Delete(&model.UserRememberToken{}).Error

	plainToken := "rem_" + randomHex(32)
	token := model.UserRememberToken{
		ID:        id.New("rem"),
		UserID:    userID,
		TokenHash: hashToken(plainToken),
		ExpiresAt: time.Now().Add(30 * 24 * time.Hour),
	}
	if err := h.db.Create(&token).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return false
	}

	setRememberCookie(ctx, userID, plainToken, h.mode == "production")
	return true
}

func setSessionCookie(ctx *gin.Context, token string, secure bool) {
	ctx.SetSameSite(http.SameSiteLaxMode)
	ctx.SetCookie(sessionCookieName, token, 86400, "/", "", secure, true)
}

func setRememberCookie(ctx *gin.Context, userID string, token string, secure bool) {
	ctx.SetSameSite(http.SameSiteLaxMode)
	ctx.SetCookie(rememberCookieNameForUser(userID), token, 30*86400, "/", "", secure, true)
}

func clearSessionCookie(ctx *gin.Context) {
	ctx.SetSameSite(http.SameSiteLaxMode)
	ctx.SetCookie(sessionCookieName, "", -1, "/", "", false, true)
}

func rememberCookieNameForUser(userID string) string {
	return rememberCookiePrefix + strings.NewReplacer("-", "_", ".", "_", ":", "_").Replace(userID)
}

type rateLimiter struct {
	redis *redis.Client
}

func newRateLimiter(redisAddr ...string) *rateLimiter {
	addr := ""
	if len(redisAddr) > 0 {
		addr = strings.TrimSpace(redisAddr[0])
	}
	return &rateLimiter{redis: redis.NewClient(&redis.Options{Addr: addr})}
}

func (l *rateLimiter) allow(key string, limit int, window time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	redisKey := "rate_limit:" + key
	count, err := l.redis.Incr(ctx, redisKey).Result()
	if err != nil {
		return false
	}
	if count == 1 {
		_ = l.redis.Expire(ctx, redisKey, window).Err()
	}
	return count <= int64(limit)
}

func (h *Handlers) allowSensitiveAuthAttempt(ctx *gin.Context, action string, limit int, window time.Duration) bool {
	if h.rateLimiter == nil {
		h.rateLimiter = newRateLimiter()
	}
	key := action + ":" + ctx.ClientIP()
	if h.rateLimiter.allow(key, limit, window) {
		return true
	}
	writeError(ctx, http.StatusTooManyRequests, "请求过于频繁，请稍后再试")
	return false
}
