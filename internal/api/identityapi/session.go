package identityapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/openapiscope"
	"github.com/LiteyukiStudio/devops/internal/redisconfig"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

const developmentRateLimit = 10000

const (
	currentUserContextKey        = "luna.devops.current_user"
	currentAccessTokenContextKey = "luna.devops.current_access_token"
)

// 账号密码与 OIDC 登录统一签发 15 天会话；会话保存在服务端，删除记录即可立即吊销。
const sessionDuration = 15 * 24 * time.Hour

func (h *Handlers) currentUser(ctx *gin.Context) (model.User, bool) {
	if user, ok := currentUserFromContext(ctx); ok {
		return user, true
	}
	if user, handled := h.currentAIPlatformUser(ctx); handled {
		return user, user.ID != ""
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
	err = h.dbFor(ctx).First(&session, "token_hash = ? and expires_at > ?", hashToken(plainToken), time.Now()).Error
	if err != nil {
		clearSessionCookie(ctx)
		writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.session.expired")
		return model.User{}, false
	}

	var user model.User
	if err := h.dbFor(ctx).First(&user, "id = ? and disabled = ?", session.UserID, false).Error; err != nil {
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
		if user.Role != authz.PlatformRoleAdmin {
			writeErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
			ctx.Abort()
			return
		}
		ctx.Next()
	}
}

func (h *Handlers) currentSessionFromCookie(ctx *gin.Context) (model.UserSession, bool) {
	if userID, sessionID, ok := h.currentAIPlatformSession(ctx); ok && userID != "" && sessionID != "" {
		var session model.UserSession
		err := h.dbFor(ctx).First(
			&session,
			"id = ? and user_id = ? and expires_at > ?",
			sessionID,
			userID,
			time.Now(),
		).Error
		return session, err == nil
	}
	plainToken, err := ctx.Cookie(sessionCookieName)
	if err != nil {
		return model.UserSession{}, false
	}

	var session model.UserSession
	err = h.dbFor(ctx).First(&session, "token_hash = ? and expires_at > ?", hashToken(plainToken), time.Now()).Error
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
	err := h.dbFor(ctx).First(
		&token,
		"token_hash = ? and source in ? and revoked_at is null and (expires_at is null or expires_at > ?)",
		hashToken(plainToken),
		[]string{model.AccessTokenSourcePersonal, model.AccessTokenSourceOAuth},
		time.Now(),
	).Error
	if err != nil {
		writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.token.invalid")
		return model.User{}, false
	}
	missingScope, err := missingRequiredAccessTokenScope(token.Scope, ctx.FullPath(), ctx.Request.Method)
	if err != nil {
		writeScopeContractUnavailableError(ctx, err.Error())
		return model.User{}, false
	}
	if missingScope != "" {
		writeScopeInsufficientError(ctx, missingScope)
		return model.User{}, false
	}
	var user model.User
	if err := h.dbFor(ctx).First(&user, "id = ? and disabled = ?", token.UserID, false).Error; err != nil {
		writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.account.disabled")
		return model.User{}, false
	}

	ctx.Set(currentAccessTokenContextKey, token)
	return user, true
}

func currentAccessTokenFromContext(ctx *gin.Context) (model.AccessToken, bool) {
	value, exists := ctx.Get(currentAccessTokenContextKey)
	if !exists {
		return model.AccessToken{}, false
	}
	token, ok := value.(model.AccessToken)
	return token, ok && token.ID != ""
}

func missingRequiredAccessTokenScope(scopeText, path, method string) (string, error) {
	requiredScopes, err := openapiscope.RequiredScopes(path, method)
	if err != nil {
		return "", err
	}
	for _, requiredScope := range requiredScopes {
		if !authz.AccessTokenAllows(scopeText, requiredScope) {
			return requiredScope, nil
		}
	}
	return "", nil
}

func platformAdminExists(db *gorm.DB) (bool, error) {
	var count int64
	if err := db.Model(&model.User{}).Where("role = ? and disabled = ?", authz.PlatformRoleAdmin, false).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (h *Handlers) requirePlatformAdmin(ctx *gin.Context) bool {
	user, ok := h.currentUser(ctx)
	if !ok {
		return false
	}
	if user.Role != authz.PlatformRoleAdmin {
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
	if err := h.dbFor(ctx).Create(&session).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return false
	}

	// 代管会话只在当前浏览器会话内有效；普通登录写入持久 Cookie，与服务端会话同时到期。
	setSessionCookie(ctx, plainToken, h.mode == "production", impersonatorID == "")
	return true
}

func newUserSession(userID, impersonatorID string, now time.Time) (model.UserSession, string) {
	plainToken := "sess_" + randomHex(32)
	authenticatedAt := now
	return model.UserSession{
		ID:                     id.New("ses"),
		UserID:                 userID,
		ImpersonatorID:         impersonatorID,
		PrimaryAuthenticatedAt: &authenticatedAt,
		TokenHash:              hashToken(plainToken),
		ExpiresAt:              now.Add(sessionDuration),
	}, plainToken
}

// revokeUserAuthentication 删除用户的全部会话，用于禁用账号、修改密码等需要立即吊销登录的场景。
func revokeUserAuthentication(tx *gorm.DB, userID string) error {
	return tx.Where("user_id = ?", userID).Delete(&model.UserSession{}).Error
}

// revokeCurrentSession 删除当前浏览器的会话（退出登录）。
func (h *Handlers) revokeCurrentSession(plainToken string, ctx context.Context) error {
	return h.dbWithContext(ctx).Where("token_hash = ?", hashToken(plainToken)).Delete(&model.UserSession{}).Error
}

func setSessionCookie(ctx *gin.Context, token string, secure bool, persistent bool) {
	maxAge := 0
	if persistent {
		maxAge = int(sessionDuration / time.Second)
	}
	ctx.SetSameSite(http.SameSiteLaxMode)
	ctx.SetCookie(sessionCookieName, token, maxAge, "/", "", secure, true)
}

func clearSessionCookie(ctx *gin.Context) {
	ctx.SetSameSite(http.SameSiteLaxMode)
	ctx.SetCookie(sessionCookieName, "", -1, "/", "", false, true)
}

type RedisRateLimiter struct {
	redis *redis.Client
}

type rateLimiter = RedisRateLimiter

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

func NewRateLimiter(redisAddr ...string) *RedisRateLimiter {
	return newRateLimiter(redisAddr...)
}

func newRateLimiterWithRedis(options redisconfig.Options) *rateLimiter {
	client := redis.NewClient(options.GoRedis())
	if err := telemetry.InstrumentRedis(client); err != nil {
		telemetry.LogWarn(context.Background(), "Redis telemetry initialization failed",
			"redis.instrumentation.failed", "redis.instrumentation.initialize",
			"telemetry.initialization.failed", err)
	}
	return &RedisRateLimiter{redis: client}
}

func NewRateLimiterWithRedis(options redisconfig.Options) *RedisRateLimiter {
	return newRateLimiterWithRedis(options)
}

func (l *rateLimiter) allow(parent context.Context, key string, limit int, window time.Duration) (bool, error) {
	if limit < 1 || window <= 0 {
		return false, errors.New("rate limit and window must be positive")
	}
	ctx, cancel := context.WithTimeout(parent, 250*time.Millisecond)
	defer cancel()
	redisKey := "rate_limit:" + key
	count, err := incrementRateLimitScript.Run(ctx, l.redis, []string{redisKey}, window.Milliseconds()).Int64()
	if err != nil {
		return false, err
	}
	return count <= int64(limit), nil
}

func (l *RedisRateLimiter) Allow(parent context.Context, key string, limit int, window time.Duration) (bool, error) {
	return l.allow(parent, key, limit, window)
}

func (l *rateLimiter) reset(parent context.Context, key string) error {
	ctx, cancel := context.WithTimeout(parent, 250*time.Millisecond)
	defer cancel()
	return l.redis.Del(ctx, "rate_limit:"+key).Err()
}

func (l *RedisRateLimiter) Reset(parent context.Context, key string) error {
	return l.reset(parent, key)
}

func (l *RedisRateLimiter) RedisClient() *redis.Client { return l.redis }

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
	allowed, err := h.rateLimiter.Allow(ctx.Request.Context(), key, limit, window)
	if allowed {
		return true
	}
	if err != nil && h.mode == "development" {
		return true
	}
	writeError(ctx, http.StatusTooManyRequests, "请求过于频繁，请稍后再试")
	return false
}
