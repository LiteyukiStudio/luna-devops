package api

import (
	"context"
	"time"

	"github.com/LiteyukiStudio/devops/internal/api/identityapi"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
	"github.com/LiteyukiStudio/devops/internal/redisconfig"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const developmentRateLimit = identityapi.DevelopmentRateLimit
const defaultAdmissionPolicyID = identityapi.DefaultAdmissionPolicyID
const lunaCLIApplicationID = identityapi.LunaCLIApplicationID
const currentUserContextKey = identityapi.CurrentUserContextKey
const currentAccessTokenContextKey = identityapi.CurrentAccessTokenContextKey
const authRegistrationSettingsID = identityapi.AuthRegistrationSettingsID
const sessionDuration = identityapi.SessionDuration
const rememberDuration = identityapi.RememberDuration
const lunaCLIClientID = identityapi.LunaCLIClientID
const oauthDeviceCodeGrantType = identityapi.OAuthDeviceCodeGrantType
const oauthCredentialRateLimit = identityapi.OAuthCredentialRateLimit
const oauthIPRateLimit = identityapi.OAuthIPRateLimit
const oauthRateLimitWindow = identityapi.OAuthRateLimitWindow

type oauthClientAttemptFlow = identityapi.OAuthClientAttemptFlow

const (
	oauthClientAttemptDeviceStart       = identityapi.OAuthClientAttemptDeviceStart
	oauthClientAttemptDeviceCode        = identityapi.OAuthClientAttemptDeviceCode
	oauthClientAttemptAuthorizationCode = identityapi.OAuthClientAttemptAuthorizationCode
	oauthClientAttemptRefresh           = identityapi.OAuthClientAttemptRefresh
	oauthClientAttemptRevoke            = identityapi.OAuthClientAttemptRevoke
	oauthClientAttemptUnsupported       = identityapi.OAuthClientAttemptUnsupported
)

type oauthTokenResponse = identityapi.OAuthTokenResponse
type oidcIdentityClaims = identityapi.OIDCIdentityClaims
type loginInput = identityapi.LoginInput
type updateCurrentUserInput = identityapi.UpdateCurrentUserInput
type platformMailSendFunc = identityapi.PlatformMailSendFunc
type authProviderInput = identityapi.AuthProviderInput
type authProviderOutput = identityapi.AuthProviderOutput
type oauthApplicationInput = identityapi.OAuthApplicationInput
type oauthDeviceAuthorizationInput = identityapi.OAuthDeviceAuthorizationInput
type oauthTokenInput = identityapi.OAuthTokenInput
type oauthTokenRevocationInput = identityapi.OAuthTokenRevocationInput
type oauthApplicationResponse = identityapi.OAuthApplicationResponse
type oauthApplicationSecretResponse = identityapi.OAuthApplicationSecretResponse
type oauthGrantResponse = identityapi.OAuthGrantResponse
type oauthAuthorizationRequest = identityapi.OAuthAuthorizationRequest
type oauthAuthorizationDecisionInput = identityapi.OAuthAuthorizationDecisionInput
type oauthAuthorizationDecisionResponse = identityapi.OAuthAuthorizationDecisionResponse
type oauthAuthorizationServerMetadataResponse = identityapi.OAuthAuthorizationServerMetadataResponse
type oauthProtocolErrorResponse = identityapi.OAuthProtocolErrorResponse
type oauthDeviceAuthorizationResponse = identityapi.OAuthDeviceAuthorizationResponse
type oauthDeviceVerificationResponse = identityapi.OAuthDeviceVerificationResponse
type oauthDeviceVerificationInput = identityapi.OAuthDeviceVerificationInput
type oauthDeviceVerificationResult = identityapi.OAuthDeviceVerificationResult

type oauthClientAuthentication struct {
	applicationID    string
	clientID         string
	clientSecretHash string
	public           bool
}

func (value oauthClientAuthentication) identityValue() identityapi.OAuthClientAuthentication {
	return identityapi.OAuthClientAuthentication{
		ApplicationID:    value.applicationID,
		ClientID:         value.clientID,
		ClientSecretHash: value.clientSecretHash,
		Public:           value.public,
	}
}

var (
	errRememberTokenInvalid     = identityapi.ErrRememberTokenInvalid
	errRememberTokenReused      = identityapi.ErrRememberTokenReused
	errRememberUserDisabled     = identityapi.ErrRememberUserDisabled
	errOIDCDisabled             = identityapi.ErrOIDCDisabled
	errOIDCEmailRequired        = identityapi.ErrOIDCEmailRequired
	errOIDCGroupDenied          = identityapi.ErrOIDCGroupDenied
	errOIDCAdmissionDenied      = identityapi.ErrOIDCAdmissionDenied
	errOIDCInvalidIdentity      = identityapi.ErrOIDCInvalidIdentity
	errOIDCRegistrationDisabled = identityapi.ErrOIDCRegistrationDisabled
)

type rateLimiter struct {
	redis    *redis.Client
	delegate *identityapi.RedisRateLimiter
}

func newRateLimiter(redisAddr ...string) *rateLimiter {
	delegate := identityapi.NewRateLimiter(redisAddr...)
	return &rateLimiter{redis: delegate.RedisClient(), delegate: delegate}
}

func newRateLimiterWithRedis(options redisconfig.Options) *rateLimiter {
	delegate := identityapi.NewRateLimiterWithRedis(options)
	return &rateLimiter{redis: delegate.RedisClient(), delegate: delegate}
}

func (limiter *rateLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	return limiter.delegate.Allow(ctx, key, limit, window)
}

func (limiter *rateLimiter) Reset(ctx context.Context, key string) error {
	return limiter.delegate.Reset(ctx, key)
}

func (limiter *rateLimiter) allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	return limiter.Allow(ctx, key, limit, window)
}

func (limiter *rateLimiter) reset(ctx context.Context, key string) error {
	return limiter.Reset(ctx, key)
}

type gitOAuthStateValue = identityapi.GitOAuthStateValue
type oidcAuthStateValue = identityapi.OIDCAuthStateValue
type oauthStateStore = identityapi.OAuthStateStore
type redisOAuthStateStore = identityapi.RedisOAuthStateStore

func newOAuthStateStore(redisAddr string) oauthStateStore {
	return identityapi.NewOAuthStateStore(redisAddr)
}

func newOAuthStateStoreWithRedis(options redisconfig.Options) oauthStateStore {
	return identityapi.NewOAuthStateStoreWithRedis(options)
}

type identityHost struct {
	domainHost
}

func (host identityHost) CurrentAIPlatformUser(ctx *gin.Context) (model.User, bool) {
	return host.handlers.domains.ai.CurrentAIPlatformUser(ctx)
}

func (host identityHost) CurrentAIPlatformSession(ctx *gin.Context) (string, string, bool) {
	return host.handlers.domains.ai.AIPlatformSession(ctx.Request.Context())
}

func (host identityHost) ExternalBaseURL(ctx *gin.Context) string {
	return host.handlers.externalBaseURL(ctx)
}

func (host identityHost) AdminConfiguredEgressContext(ctx context.Context, timeout time.Duration) context.Context {
	return host.handlers.adminConfiguredEgressContext(ctx, timeout)
}

func (host identityHost) NormalizeUserBrandColorPreset(value string) (string, bool) {
	return normalizeUserBrandColorPreset(value)
}

func (host identityHost) NormalizeUserInterfaceStyle(value string) (string, bool) {
	return normalizeUserInterfaceStyle(value)
}

func (host identityHost) TrustedProxyCIDRs() []string {
	return host.handlers.config.TrustedProxyCIDRs
}

func (host identityHost) RateLimiter() identityapi.Limiter {
	if host.handlers.rateLimiter == nil {
		host.handlers.rateLimiter = newRateLimiter()
	}
	return host.handlers.rateLimiter
}

func (host identityHost) OAuthStateStore() identityapi.OAuthStateStore {
	return host.handlers.oauthStates
}

func (h *Handlers) currentUser(ctx *gin.Context) (model.User, bool) {
	return h.domains.identity.CurrentUser(ctx)
}

func currentUserFromContext(ctx *gin.Context) (model.User, bool) {
	return identityapi.CurrentUserFromContext(ctx)
}

func (h *Handlers) currentSessionFromCookie(ctx *gin.Context) (model.UserSession, bool) {
	return h.domains.identity.CurrentSessionFromCookie(ctx)
}

func (h *Handlers) currentUserFromAccessToken(ctx *gin.Context) (model.User, bool) {
	return h.domains.identity.CurrentUserFromAccessToken(ctx)
}

func currentAccessTokenFromContext(ctx *gin.Context) (model.AccessToken, bool) {
	return identityapi.CurrentAccessTokenFromContext(ctx)
}

func missingRequiredAccessTokenScope(scopeText, path, method string) (string, error) {
	return identityapi.MissingRequiredAccessTokenScope(scopeText, path, method)
}

func accessTokenAllows(scopeText, required string) bool {
	return identityapi.AccessTokenAllows(scopeText, required)
}

func (h *Handlers) requirePlatformAdmin(ctx *gin.Context) bool {
	return h.domains.identity.RequirePlatformAdmin(ctx)
}

func (h *Handlers) allowLoginAccountAttempt(ctx *gin.Context, account string, limit int, window time.Duration) bool {
	return h.domains.identity.AllowLoginAccountAttempt(ctx, account, limit, window)
}

func (h *Handlers) allowOAuthClientAttempt(ctx *gin.Context, clientID string) bool {
	return h.domains.identity.AllowOAuthClientAttempt(ctx, clientID)
}

func oauthClientAttemptFlowForGrantType(grantType string) oauthClientAttemptFlow {
	return identityapi.OAuthClientAttemptFlowForGrantType(grantType)
}

func (h *Handlers) allowOAuthTokenClientAttempt(ctx *gin.Context, clientID string, flow oauthClientAttemptFlow, credential string) bool {
	return h.domains.identity.AllowOAuthTokenClientAttempt(ctx, clientID, flow, credential)
}

func (h *Handlers) allowOAuthDeviceVerificationAttempt(ctx *gin.Context, userID string) bool {
	return h.domains.identity.AllowOAuthDeviceVerificationAttempt(ctx, userID)
}

func (h *Handlers) ensureAdmissionPolicy(ctx context.Context) (model.AuthAdmissionPolicy, error) {
	return h.domains.identity.EnsureAdmissionPolicy(ctx)
}

func (h *Handlers) rotateRememberLogin(userID, plainToken string, ctx context.Context) (model.User, string, string, error) {
	return h.domains.identity.RotateRememberLogin(userID, plainToken, ctx)
}

func newUserSession(userID, impersonatorID string, now time.Time) (model.UserSession, string) {
	return identityapi.NewUserSession(userID, impersonatorID, now)
}

func newUserSessionInFamily(userID, impersonatorID, familyID string, now time.Time) (model.UserSession, string) {
	return identityapi.NewUserSessionInFamily(userID, impersonatorID, familyID, now)
}

func newUserSessionInFamilyWithPrimaryAuthentication(userID, impersonatorID, familyID string, now time.Time, primaryAuthenticatedAt *time.Time, expiresAt time.Time) (model.UserSession, string) {
	return identityapi.NewUserSessionInFamilyWithPrimaryAuthentication(userID, impersonatorID, familyID, now, primaryAuthenticatedAt, expiresAt)
}

func newUserRememberToken(userID string, now time.Time) (model.UserRememberToken, string) {
	return identityapi.NewUserRememberToken(userID, now)
}

func newUserRememberTokenInFamily(userID, familyID string, expiresAt time.Time) (model.UserRememberToken, string) {
	return identityapi.NewUserRememberTokenInFamily(userID, familyID, expiresAt)
}

func (h *Handlers) revokeCurrentSessionAndRememberTokens(plainToken string, ctx context.Context) (string, error) {
	return h.domains.identity.RevokeCurrentSessionAndRememberTokens(plainToken, ctx)
}

func (h *Handlers) cleanupExpiredRememberTokenFamilies(userID string, now time.Time, ctx context.Context) error {
	return h.domains.identity.CleanupExpiredRememberTokenFamilies(userID, now, ctx)
}

func revokeUserAuthentication(tx *gorm.DB, userID string) error {
	return identityapi.RevokeUserAuthentication(tx, userID)
}

func (h *Handlers) findOrCreateOIDCUser(provider model.AuthProvider, claims oidcIdentityClaims, ctx context.Context) (model.User, error) {
	return h.domains.identity.FindOrCreateOIDCUser(provider, claims, ctx)
}

func (h *Handlers) resolveSecretContext(ctx context.Context, ref string) string {
	return h.domains.identity.ResolveSecretContext(ctx, ref)
}

func (h *Handlers) createSession(ctx *gin.Context, userID string) bool {
	return h.domains.identity.CreateSession(ctx, userID)
}

func (h *Handlers) createLoginCredentials(ctx *gin.Context, userID string, remember bool) bool {
	return h.domains.identity.CreateLoginCredentials(ctx, userID, remember)
}

func (h *Handlers) createRememberToken(ctx *gin.Context, userID string, requested ...bool) bool {
	return h.domains.identity.CreateRememberToken(ctx, userID, requested...)
}

func (h *Handlers) auditWithContext(userID, action, resource string, success bool, message string, ctx context.Context) {
	h.domains.identity.AuditWithContext(userID, action, resource, success, message, ctx)
}

type runtimeClusterAuditMetadata = identityapi.RuntimeClusterAuditMetadata

type safeAuditMetadata interface {
	runtimeClusterAuditMetadata
}

func auditWithSafeMetadata[T safeAuditMetadata](h *Handlers, userID, action, resource string, success bool, message string, metadata T, ctx context.Context) {
	h.domains.identity.AuditWithSafeMetadata(userID, action, resource, success, message, metadata, ctx)
}

func configBool(value string) bool { return identityapi.ConfigBool(value) }

func requestUsesBearerToken(ctx *gin.Context) bool {
	return identityapi.RequestUsesBearerToken(ctx)
}

func lockActiveUserRole(tx *gorm.DB, userID, requiredRole string) (model.User, error) {
	return identityapi.LockActiveUserRole(tx, userID, requiredRole)
}

func jsonList(values []string) []string { return identityapi.JSONList(values) }

func containsString(values []string, target string) bool {
	return identityapi.ContainsString(values, target)
}

func auditResourceType(action string) string { return identityapi.AuditResourceType(action) }

func normalizedRegistrationEmail(value string) (string, error) {
	return identityapi.NormalizedRegistrationEmail(value)
}

func currentUserResponse(user model.User) gin.H { return identityapi.CurrentUserResponse(user) }

func authRegistrationSettingsResponse(settings model.AuthRegistrationSettings) gin.H {
	return identityapi.AuthRegistrationSettingsResponse(settings)
}

func sendRegistrationEmailWith(ctx context.Context, db *gorm.DB, resolver notification.SecretResolver, challenge model.EmailRegistrationChallenge, code string, send platformMailSendFunc) error {
	return identityapi.SendRegistrationEmailWith(ctx, db, resolver, challenge, code, send)
}

func setSessionCookie(ctx *gin.Context, token string, secure bool, persistent bool) {
	identityapi.SetSessionCookie(ctx, token, secure, persistent)
}

func setRememberCookie(ctx *gin.Context, userID, token string, secure bool) {
	identityapi.SetRememberCookie(ctx, userID, token, secure)
}

func clearSessionCookie(ctx *gin.Context) { identityapi.ClearSessionCookie(ctx) }

func clearRememberCookie(ctx *gin.Context, userID string) {
	identityapi.ClearRememberCookie(ctx, userID)
}

func rememberCookieNameForUser(userID string) string {
	return identityapi.RememberCookieNameForUser(userID)
}

func shouldRevokeUserAuthentication(originalRole, nextRole string, originallyDisabled, nextDisabled, passwordChanged bool) bool {
	return identityapi.ShouldRevokeUserAuthentication(originalRole, nextRole, originallyDisabled, nextDisabled, passwordChanged)
}

func developmentAdminFreeQuotaCredits(configured string) (decimal.Decimal, error) {
	return identityapi.DevelopmentAdminFreeQuotaCredits(configured)
}

func authProviderResponse(provider model.AuthProvider) authProviderOutput {
	return identityapi.AuthProviderResponse(provider)
}

func authProviderFromInput(input authProviderInput, providerID, existingSecretRef string) (model.AuthProvider, bool) {
	return identityapi.AuthProviderFromInput(input, providerID, existingSecretRef)
}

func defaultUserProjectName(user model.User) string {
	return identityapi.DefaultUserProjectName(user)
}

func dnsSafeProjectIdentifier(value string) string {
	return identityapi.DNSSafeProjectIdentifier(value)
}

func slugWithNumericSuffix(base string, index int) string {
	return identityapi.SlugWithNumericSuffix(base, index)
}

func normalizeAccessTokenScope(scopeText string) string {
	return identityapi.NormalizeAccessTokenScope(scopeText)
}

func userCanCreateAccessTokenScope(user model.User, scopeText string) bool {
	return identityapi.UserCanCreateAccessTokenScope(user, scopeText)
}

func validAccessTokenLifetimeDays(days int) bool {
	return identityapi.ValidAccessTokenLifetimeDays(days)
}

func oidcCallbackURL(publicBaseURL string) string {
	return identityapi.OIDCCallbackURL(publicBaseURL)
}

func oidcAdmissionEmail(claims oidcIdentityClaims, requireVerified bool) (string, bool) {
	return identityapi.OIDCAdmissionEmail(claims, requireVerified)
}

func encodeStringList(values []string) string { return identityapi.EncodeStringList(values) }

func decodeStringList(value string) []string { return identityapi.DecodeStringList(value) }

func updateOwnedOAuthApplication(db *gorm.DB, applicationID, ownerUserID string, next model.OAuthApplication) (model.OAuthApplication, error) {
	return identityapi.UpdateOwnedOAuthApplication(db, applicationID, ownerUserID, next)
}

func rotateOwnedOAuthApplicationSecret(db *gorm.DB, applicationID, ownerUserID, clientSecretHash string) (model.OAuthApplication, error) {
	return identityapi.RotateOwnedOAuthApplicationSecret(db, applicationID, ownerUserID, clientSecretHash)
}

func deleteOwnedOAuthApplication(db *gorm.DB, applicationID, ownerUserID string) (model.OAuthApplication, error) {
	return identityapi.DeleteOwnedOAuthApplication(db, applicationID, ownerUserID)
}

func recordOAuthAuthorizationConsent(tx *gorm.DB, code *model.OAuthAuthorizationCode) error {
	return identityapi.RecordOAuthAuthorizationConsent(tx, code)
}

func revokeOAuthFamily(tx *gorm.DB, grantID, familyID string, now time.Time) error {
	return identityapi.RevokeOAuthFamily(tx, grantID, familyID, now)
}

func revokeOAuthGrant(tx *gorm.DB, grantID string, now time.Time) error {
	return identityapi.RevokeOAuthGrant(tx, grantID, now)
}

func revokeOAuthApplication(tx *gorm.DB, applicationID string, now time.Time) error {
	return identityapi.RevokeOAuthApplication(tx, applicationID, now)
}

func exchangeOAuthAuthorizationCodeValue(db *gorm.DB, authentication oauthClientAuthentication, plainCode, redirectURI, verifier string, now time.Time) (oauthTokenResponse, error) {
	return identityapi.ExchangeOAuthAuthorizationCodeValue(db, authentication.identityValue(), plainCode, redirectURI, verifier, now)
}

var (
	ErrInitialAdminConfigInvalid       = identityapi.ErrInitialAdminConfigInvalid
	ErrInitialAdminDatabaseUnavailable = identityapi.ErrInitialAdminDatabaseUnavailable
	ErrInitialAdminRecoveryRequired    = identityapi.ErrInitialAdminRecoveryRequired
)

func EnsureInitialAdmin(ctx context.Context, db *gorm.DB, mode string, input InitialAdminConfig) error {
	return identityapi.EnsureInitialAdmin(ctx, db, mode, input)
}
