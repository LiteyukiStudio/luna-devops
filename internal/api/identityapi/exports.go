package identityapi

import (
	"context"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const DevelopmentRateLimit = developmentRateLimit
const DefaultAdmissionPolicyID = defaultAdmissionPolicyID
const LunaCLIApplicationID = lunaCLIApplicationID
const CurrentUserContextKey = currentUserContextKey
const CurrentAccessTokenContextKey = currentAccessTokenContextKey
const AuthRegistrationSettingsID = authRegistrationSettingsID
const SessionDuration = sessionDuration
const RememberDuration = rememberDuration
const LunaCLIClientID = lunaCLIClientID
const OAuthDeviceCodeGrantType = oauthDeviceCodeGrantType
const OAuthCredentialRateLimit = oauthCredentialRateLimit
const OAuthIPRateLimit = oauthIPRateLimit
const OAuthRateLimitWindow = oauthRateLimitWindow

type OAuthClientAttemptFlow = oauthClientAttemptFlow

const (
	OAuthClientAttemptDeviceStart       = oauthClientAttemptDeviceStart
	OAuthClientAttemptDeviceCode        = oauthClientAttemptDeviceCode
	OAuthClientAttemptAuthorizationCode = oauthClientAttemptAuthorizationCode
	OAuthClientAttemptRefresh           = oauthClientAttemptRefresh
	OAuthClientAttemptRevoke            = oauthClientAttemptRevoke
	OAuthClientAttemptUnsupported       = oauthClientAttemptUnsupported
)

type OAuthTokenResponse = oauthTokenResponse
type OIDCIdentityClaims = oidcIdentityClaims
type LoginInput = loginInput
type UpdateCurrentUserInput = updateCurrentUserInput
type PlatformMailSendFunc = platformMailSendFunc
type AuthProviderInput = authProviderInput
type AuthProviderOutput = authProviderOutput
type OAuthApplicationInput = oauthApplicationInput
type OAuthDeviceAuthorizationInput = oauthDeviceAuthorizationInput
type OAuthTokenInput = oauthTokenInput
type OAuthTokenRevocationInput = oauthTokenRevocationInput
type OAuthApplicationResponse = oauthApplicationResponse
type OAuthApplicationSecretResponse = oauthApplicationSecretResponse
type OAuthGrantResponse = oauthGrantResponse
type OAuthAuthorizationRequest = oauthAuthorizationRequest
type OAuthAuthorizationDecisionInput = oauthAuthorizationDecisionInput
type OAuthAuthorizationDecisionResponse = oauthAuthorizationDecisionResponse
type OAuthAuthorizationServerMetadataResponse = oauthAuthorizationServerMetadataResponse
type OAuthProtocolErrorResponse = oauthProtocolErrorResponse
type OAuthDeviceAuthorizationResponse = oauthDeviceAuthorizationResponse
type OAuthDeviceVerificationResponse = oauthDeviceVerificationResponse
type OAuthDeviceVerificationInput = oauthDeviceVerificationInput
type OAuthDeviceVerificationResult = oauthDeviceVerificationResult

type OAuthClientAuthentication struct {
	ApplicationID    string
	ClientID         string
	ClientSecretHash string
	Public           bool
}

func (value OAuthClientAuthentication) internal() oauthClientAuthentication {
	return oauthClientAuthentication{
		applicationID:    value.ApplicationID,
		clientID:         value.ClientID,
		clientSecretHash: value.ClientSecretHash,
		public:           value.Public,
	}
}

var (
	ErrRememberTokenInvalid     = errRememberTokenInvalid
	ErrRememberTokenReused      = errRememberTokenReused
	ErrRememberUserDisabled     = errRememberUserDisabled
	ErrOIDCDisabled             = errOIDCDisabled
	ErrOIDCEmailRequired        = errOIDCEmailRequired
	ErrOIDCGroupDenied          = errOIDCGroupDenied
	ErrOIDCAdmissionDenied      = errOIDCAdmissionDenied
	ErrOIDCInvalidIdentity      = errOIDCInvalidIdentity
	ErrOIDCRegistrationDisabled = errOIDCRegistrationDisabled
)

func (h *Handler) CurrentUser(ctx *gin.Context) (model.User, bool) { return h.currentUser(ctx) }

func CurrentUserFromContext(ctx *gin.Context) (model.User, bool) {
	return currentUserFromContext(ctx)
}

func (h *Handler) PlatformAdminMiddleware() gin.HandlerFunc { return h.platformAdminMiddleware() }

func (h *Handler) CurrentSessionFromCookie(ctx *gin.Context) (model.UserSession, bool) {
	return h.currentSessionFromCookie(ctx)
}

func (h *Handler) CurrentUserFromAccessToken(ctx *gin.Context) (model.User, bool) {
	return h.currentUserFromAccessToken(ctx)
}

func CurrentAccessTokenFromContext(ctx *gin.Context) (model.AccessToken, bool) {
	return currentAccessTokenFromContext(ctx)
}

func MissingRequiredAccessTokenScope(scopeText, path, method string) (string, error) {
	return missingRequiredAccessTokenScope(scopeText, path, method)
}

func AccessTokenAllows(scopeText, required string) bool {
	return accessTokenAllows(scopeText, required)
}

func (h *Handler) RequirePlatformAdmin(ctx *gin.Context) bool {
	return h.requirePlatformAdmin(ctx)
}

func (h *Handler) EnsureAdmissionPolicy(ctx context.Context) (model.AuthAdmissionPolicy, error) {
	return h.ensureAdmissionPolicy(ctx)
}

func (h *Handler) RotateRememberLogin(userID, plainToken string, ctx context.Context) (model.User, string, string, error) {
	return h.rotateRememberLogin(userID, plainToken, ctx)
}

func NewUserSession(userID, impersonatorID string, now time.Time) (model.UserSession, string) {
	return newUserSession(userID, impersonatorID, now)
}

func NewUserSessionInFamily(userID, impersonatorID, familyID string, now time.Time) (model.UserSession, string) {
	return newUserSessionInFamily(userID, impersonatorID, familyID, now)
}

func NewUserSessionInFamilyWithPrimaryAuthentication(userID, impersonatorID, familyID string, now time.Time, primaryAuthenticatedAt *time.Time, expiresAt time.Time) (model.UserSession, string) {
	return newUserSessionInFamilyWithPrimaryAuthentication(userID, impersonatorID, familyID, now, primaryAuthenticatedAt, expiresAt)
}

func NewUserRememberToken(userID string, now time.Time) (model.UserRememberToken, string) {
	return newUserRememberToken(userID, now)
}

func NewUserRememberTokenInFamily(userID, familyID string, expiresAt time.Time) (model.UserRememberToken, string) {
	return newUserRememberTokenInFamily(userID, familyID, expiresAt)
}

func (h *Handler) RevokeCurrentSessionAndRememberTokens(plainToken string, ctx context.Context) (string, error) {
	return h.revokeCurrentSessionAndRememberTokens(plainToken, ctx)
}

func (h *Handler) CleanupExpiredRememberTokenFamilies(userID string, now time.Time, ctx context.Context) error {
	return h.cleanupExpiredRememberTokenFamilies(userID, now, ctx)
}

func RevokeUserAuthentication(tx *gorm.DB, userID string) error {
	return revokeUserAuthentication(tx, userID)
}

func (h *Handler) FindOrCreateOIDCUser(provider model.AuthProvider, claims OIDCIdentityClaims, ctx context.Context) (model.User, error) {
	return h.findOrCreateOIDCUser(provider, claims, ctx)
}

func (h *Handler) ResolveSecretContext(ctx context.Context, ref string) string {
	return h.resolveSecretContext(ctx, ref)
}

func (h *Handler) CreateSession(ctx *gin.Context, userID string) bool {
	return h.createSession(ctx, userID)
}

func (h *Handler) CreateLoginCredentials(ctx *gin.Context, userID string, remember bool) bool {
	return h.createLoginCredentials(ctx, userID, remember)
}

func (h *Handler) CreateRememberToken(ctx *gin.Context, userID string, requested ...bool) bool {
	return h.createRememberToken(ctx, userID, requested...)
}

func (h *Handler) AuditWithContext(userID, action, resource string, success bool, message string, ctx context.Context) {
	h.auditWithContext(userID, action, resource, success, message, ctx)
}

func (h *Handler) AuditWithSafeMetadata(userID, action, resource string, success bool, message string, metadata any, ctx context.Context) {
	h.auditWithSafeMetadata(userID, action, resource, success, message, metadata, ctx)
}

func (h *Handler) AllowSensitiveAuthAttempt(ctx *gin.Context, action string, limit int, window time.Duration) bool {
	return h.allowSensitiveAuthAttempt(ctx, action, limit, window)
}

func (h *Handler) AllowLoginAccountAttempt(ctx *gin.Context, account string, limit int, window time.Duration) bool {
	return h.allowLoginAccountAttempt(ctx, account, limit, window)
}

func (h *Handler) AllowOAuthClientAttempt(ctx *gin.Context, clientID string) bool {
	return h.allowOAuthClientAttempt(ctx, clientID)
}

func OAuthClientAttemptFlowForGrantType(grantType string) OAuthClientAttemptFlow {
	return oauthClientAttemptFlowForGrantType(grantType)
}

func (h *Handler) AllowOAuthTokenClientAttempt(ctx *gin.Context, clientID string, flow OAuthClientAttemptFlow, credential string) bool {
	return h.allowOAuthTokenClientAttempt(ctx, clientID, flow, credential)
}

func (h *Handler) AllowOAuthDeviceVerificationAttempt(ctx *gin.Context, userID string) bool {
	return h.allowOAuthDeviceVerificationAttempt(ctx, userID)
}

func ConfigBool(value string) bool { return configBool(value) }

func RequestUsesBearerToken(ctx *gin.Context) bool { return requestUsesBearerToken(ctx) }

func LockActiveUserRole(tx *gorm.DB, userID, requiredRole string) (model.User, error) {
	return lockActiveUserRole(tx, userID, requiredRole)
}

func JSONList(values []string) []string { return jsonList(values) }

func ContainsString(values []string, target string) bool { return containsString(values, target) }

func AuditResourceType(action string) string { return auditResourceType(action) }

func NormalizedRegistrationEmail(value string) (string, error) {
	return normalizedRegistrationEmail(value)
}

func CurrentUserResponse(user model.User) gin.H { return currentUserResponse(user) }

func AuthRegistrationSettingsResponse(settings model.AuthRegistrationSettings) gin.H {
	return authRegistrationSettingsResponse(settings)
}

func SendRegistrationEmailWith(ctx context.Context, db *gorm.DB, resolver notification.SecretResolver, challenge model.EmailRegistrationChallenge, code string, send PlatformMailSendFunc) error {
	return sendRegistrationEmailWith(ctx, db, resolver, challenge, code, send)
}

func SetSessionCookie(ctx *gin.Context, token string, secure bool, persistent bool) {
	setSessionCookie(ctx, token, secure, persistent)
}

func SetRememberCookie(ctx *gin.Context, userID, token string, secure bool) {
	setRememberCookie(ctx, userID, token, secure)
}

func ClearSessionCookie(ctx *gin.Context) { clearSessionCookie(ctx) }

func ClearRememberCookie(ctx *gin.Context, userID string) { clearRememberCookie(ctx, userID) }

func RememberCookieNameForUser(userID string) string { return rememberCookieNameForUser(userID) }

func ShouldRevokeUserAuthentication(originalRole, nextRole string, originallyDisabled, nextDisabled, passwordChanged bool) bool {
	return shouldRevokeUserAuthentication(originalRole, nextRole, originallyDisabled, nextDisabled, passwordChanged)
}

func DevelopmentAdminFreeQuotaCredits(configured string) (decimal.Decimal, error) {
	return developmentAdminFreeQuotaCredits(configured)
}

func AuthProviderResponse(provider model.AuthProvider) AuthProviderOutput {
	return authProviderResponse(provider)
}

func AuthProviderFromInput(input AuthProviderInput, providerID, existingSecretRef string) (model.AuthProvider, bool) {
	return authProviderFromInput(input, providerID, existingSecretRef)
}

func DefaultUserProjectName(user model.User) string { return defaultUserProjectName(user) }

func DNSSafeProjectIdentifier(value string) string { return dnsSafeProjectIdentifier(value) }

func SlugWithNumericSuffix(base string, index int) string {
	return slugWithNumericSuffix(base, index)
}

func NormalizeAccessTokenScope(scopeText string) string {
	return normalizeAccessTokenScope(scopeText)
}

func UserCanCreateAccessTokenScope(user model.User, scopeText string) bool {
	return userCanCreateAccessTokenScope(user, scopeText)
}

func ValidAccessTokenLifetimeDays(days int) bool { return validAccessTokenLifetimeDays(days) }

func OIDCCallbackURL(publicBaseURL string) string { return oidcCallbackURL(publicBaseURL) }

func OIDCAdmissionEmail(claims OIDCIdentityClaims, requireVerified bool) (string, bool) {
	return oidcAdmissionEmail(claims, requireVerified)
}

func EncodeStringList(values []string) string { return encodeStringList(values) }

func DecodeStringList(value string) []string { return decodeStringList(value) }

func UpdateOwnedOAuthApplication(db *gorm.DB, applicationID, ownerUserID string, next model.OAuthApplication) (model.OAuthApplication, error) {
	return updateOwnedOAuthApplication(db, applicationID, ownerUserID, next)
}

func RotateOwnedOAuthApplicationSecret(db *gorm.DB, applicationID, ownerUserID, clientSecretHash string) (model.OAuthApplication, error) {
	return rotateOwnedOAuthApplicationSecret(db, applicationID, ownerUserID, clientSecretHash)
}

func DeleteOwnedOAuthApplication(db *gorm.DB, applicationID, ownerUserID string) (model.OAuthApplication, error) {
	return deleteOwnedOAuthApplication(db, applicationID, ownerUserID)
}

func RecordOAuthAuthorizationConsent(tx *gorm.DB, code *model.OAuthAuthorizationCode) error {
	return recordOAuthAuthorizationConsent(tx, code)
}

func RevokeOAuthFamily(tx *gorm.DB, grantID, familyID string, now time.Time) error {
	return revokeOAuthFamily(tx, grantID, familyID, now)
}

func RevokeOAuthGrant(tx *gorm.DB, grantID string, now time.Time) error {
	return revokeOAuthGrant(tx, grantID, now)
}

func RevokeOAuthApplication(tx *gorm.DB, applicationID string, now time.Time) error {
	return revokeOAuthApplication(tx, applicationID, now)
}

func ExchangeOAuthAuthorizationCodeValue(db *gorm.DB, authentication OAuthClientAuthentication, plainCode, redirectURI, verifier string, now time.Time) (OAuthTokenResponse, error) {
	return exchangeOAuthAuthorizationCodeValue(db, authentication.internal(), plainCode, redirectURI, verifier, now)
}
