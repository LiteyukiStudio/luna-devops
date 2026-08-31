package api

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *Handlers) GetOAuthAuthorizationRequest(ctx *gin.Context) {
	user, ok := h.oauthCookieUser(ctx)
	if !ok {
		return
	}
	application, scope, valid := h.validateOAuthAuthorizationRequest(
		ctx,
		ctx.Query("client_id"),
		ctx.Query("redirect_uri"),
		ctx.Query("scope"),
		ctx.Query("code_challenge"),
		ctx.Query("code_challenge_method"),
		user,
	)
	if !valid {
		return
	}
	var grant model.OAuthGrant
	grantErr := h.dbFor(ctx).First(
		&grant,
		"application_id = ? and user_id = ? and revoked_at is null",
		application.ID,
		user.ID,
	).Error
	previouslyAuthorized := grantErr == nil && oauthScopeSubset(scope, grant.Scope)
	ctx.JSON(http.StatusOK, oauthAuthorizationRequest{
		Application:             oauthApplicationToResponse(application),
		Scope:                   scope,
		AccessTokenLifetimeDays: application.AccessTokenLifetimeDays,
		PreviouslyAuthorized:    previouslyAuthorized,
	})
}

func (h *Handlers) DecideOAuthAuthorization(ctx *gin.Context) {
	user, ok := h.oauthCookieUser(ctx)
	if !ok {
		return
	}
	var input oauthAuthorizationDecisionInput
	if !bindJSON(ctx, &input) {
		return
	}
	application, scope, valid := h.validateOAuthAuthorizationRequest(
		ctx,
		input.ClientID,
		input.RedirectURI,
		input.Scope,
		input.CodeChallenge,
		input.CodeChallengeMethod,
		user,
	)
	if !valid {
		return
	}
	if !input.Approved {
		values := url.Values{"error": {"access_denied"}}
		if input.State != "" {
			values.Set("state", input.State)
		}
		h.auditWithContext(user.ID, "oauth_grant.deny", application.ID, true, scope, ctx.Request.Context())
		ctx.JSON(http.StatusOK, oauthAuthorizationDecisionResponse{
			RedirectURL: appendOAuthRedirectValues(input.RedirectURI, values),
		})
		return
	}

	plainCode := "lyo_code_" + randomHex(32)
	authorizationCode := model.OAuthAuthorizationCode{
		ID: id.New("ocod"), ApplicationID: application.ID, UserID: user.ID,
		CodeHash: hashToken(plainCode), RedirectURI: input.RedirectURI, Scope: scope,
		CodeChallenge: input.CodeChallenge, CodeChallengeMethod: "S256", ExpiresAt: time.Now().Add(oauthAuthorizationCodeTTL),
	}
	err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		return recordOAuthAuthorizationConsent(tx, &authorizationCode)
	})
	if err != nil {
		if errors.Is(err, errOAuthInvalidGrant) || errors.Is(err, errOAuthInvalidScope) {
			writeErrorCode(ctx, http.StatusForbidden, "oauth.scope.forbidden", "OAuth application or user permissions changed before authorization completed")
			return
		}
		writeErrorCode(ctx, http.StatusInternalServerError, "oauth.authorization.failed", "OAuth authorization could not be completed")
		return
	}
	values := url.Values{"code": {plainCode}}
	if input.State != "" {
		values.Set("state", input.State)
	}
	h.auditWithContext(user.ID, "oauth_grant.authorize", authorizationCode.ID, true, scope, ctx.Request.Context())
	ctx.JSON(http.StatusOK, oauthAuthorizationDecisionResponse{
		RedirectURL: appendOAuthRedirectValues(input.RedirectURI, values),
	})
}

func (h *Handlers) ExchangeOAuthToken(ctx *gin.Context) {
	var input oauthTokenInput
	if !bindOAuthForm(ctx, &input) {
		return
	}
	grantType := strings.TrimSpace(input.GrantType)
	flow := oauthClientAttemptFlowForGrantType(grantType)
	credential := ""
	switch flow {
	case oauthClientAttemptAuthorizationCode:
		credential = input.Code
	case oauthClientAttemptRefresh:
		credential = input.RefreshToken
	case oauthClientAttemptDeviceCode:
		credential = input.DeviceCode
	}
	allowPublicClient := grantType == oauthDeviceCodeGrantType || grantType == "refresh_token"
	authentication, ok := h.authenticateOAuthTokenClient(
		ctx,
		allowPublicClient,
		flow,
		input.ClientID,
		input.ClientSecret,
		credential,
	)
	if !ok {
		return
	}
	switch grantType {
	case "authorization_code":
		h.exchangeOAuthAuthorizationCode(ctx, authentication, input)
	case "refresh_token":
		h.exchangeOAuthRefreshToken(ctx, authentication, input)
	case oauthDeviceCodeGrantType:
		h.exchangeOAuthDeviceCode(ctx, authentication, input.DeviceCode)
	default:
		oauthError(ctx, http.StatusBadRequest, "unsupported_grant_type", "Supported grant types are authorization_code, refresh_token, and device_code")
	}
}

func (h *Handlers) RevokeOAuthToken(ctx *gin.Context) {
	var input oauthTokenRevocationInput
	if !bindOAuthForm(ctx, &input) {
		return
	}
	plainToken := strings.TrimSpace(input.Token)
	if plainToken == "" {
		oauthError(ctx, http.StatusBadRequest, "invalid_request", "token is required")
		return
	}
	authentication, ok := h.authenticateOAuthTokenClient(
		ctx,
		true,
		oauthClientAttemptRevoke,
		input.ClientID,
		input.ClientSecret,
		plainToken,
	)
	if !ok {
		return
	}
	err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		application, err := lockOAuthClientApplication(tx, authentication, true)
		if err != nil {
			return err
		}
		return revokeOAuthTokenFamily(tx, application.ID, hashToken(plainToken), time.Now())
	})
	if err != nil {
		if errors.Is(err, errOAuthInvalidClient) || errors.Is(err, errOAuthInvalidGrant) {
			oauthError(ctx, http.StatusUnauthorized, "invalid_client", "Client authentication failed")
			return
		}
		oauthError(ctx, http.StatusInternalServerError, "server_error", "Token revocation failed")
		return
	}
	ctx.Header("Cache-Control", "no-store")
	ctx.Status(http.StatusOK)
}

func (h *Handlers) GetOAuthAuthorizationServerMetadata(ctx *gin.Context) {
	baseURL := strings.TrimRight(h.externalBaseURL(ctx), "/")
	if baseURL == "" {
		writeErrorCode(ctx, http.StatusServiceUnavailable, "oauth.public_base_url.required", "PUBLIC_BASE_URL is required")
		return
	}
	ctx.JSON(http.StatusOK, oauthAuthorizationServerMetadataResponse{
		Issuer:                            baseURL,
		AuthorizationEndpoint:             baseURL + "/oauth/authorize",
		DeviceAuthorizationEndpoint:       baseURL + "/api/v1/oauth/device/authorization",
		TokenEndpoint:                     baseURL + "/api/v1/oauth/token",
		RevocationEndpoint:                baseURL + "/api/v1/oauth/revoke",
		ResponseTypesSupported:            []string{"code"},
		GrantTypesSupported:               []string{"authorization_code", "refresh_token", oauthDeviceCodeGrantType},
		TokenEndpointAuthMethodsSupported: []string{"client_secret_basic", "client_secret_post", "none"},
		CodeChallengeMethodsSupported:     []string{"S256"},
	})
}

func (h *Handlers) validateOAuthAuthorizationRequest(ctx *gin.Context, clientID, redirectURI, requestedScope, challenge, challengeMethod string, user model.User) (model.OAuthApplication, string, bool) {
	if ctx.Query("response_type") != "" && ctx.Query("response_type") != "code" {
		writeErrorCode(ctx, http.StatusBadRequest, "oauth.response_type.invalid", "Only the authorization code response type is supported")
		return model.OAuthApplication{}, "", false
	}
	var application model.OAuthApplication
	if h.dbFor(ctx).First(&application, "client_id = ? and revoked_at is null", strings.TrimSpace(clientID)).Error != nil {
		writeErrorCode(ctx, http.StatusNotFound, "oauth.application.not_found", "OAuth application not found")
		return model.OAuthApplication{}, "", false
	}
	if !exactRedirectURIAllowed(application, redirectURI) {
		writeErrorCode(ctx, http.StatusBadRequest, "oauth.redirect_uri.invalid", "Redirect URI does not match the registered application")
		return model.OAuthApplication{}, "", false
	}
	scope := normalizeOAuthScope(requestedScope)
	if strings.TrimSpace(requestedScope) == "" || scope == "" || !oauthApplicationAllowsScope(application, scope) || !userCanAuthorizeOAuthScope(user, scope) {
		writeErrorCode(ctx, http.StatusForbidden, "oauth.scope.forbidden", "Requested OAuth scope is not allowed")
		return model.OAuthApplication{}, "", false
	}
	if challengeMethod != "S256" || !validPKCEChallenge(challenge) {
		writeErrorCode(ctx, http.StatusBadRequest, "oauth.pkce.required", "PKCE with the S256 method is required")
		return model.OAuthApplication{}, "", false
	}
	return application, scope, true
}

func (h *Handlers) exchangeOAuthAuthorizationCode(ctx *gin.Context, authentication oauthClientAuthentication, input oauthTokenInput) {
	plainCode := strings.TrimSpace(input.Code)
	redirectURI := strings.TrimSpace(input.RedirectURI)
	response, err := exchangeOAuthAuthorizationCodeValue(
		h.dbFor(ctx),
		authentication,
		plainCode,
		redirectURI,
		input.CodeVerifier,
		time.Now(),
	)
	if errors.Is(err, errOAuthInvalidClient) {
		oauthError(ctx, http.StatusUnauthorized, "invalid_client", "Client authentication failed")
		return
	}
	if err != nil {
		oauthError(ctx, http.StatusBadRequest, "invalid_grant", "Authorization code is invalid, expired, consumed, or does not match the request")
		return
	}
	writeOAuthTokenResponse(ctx, response)
}

func (h *Handlers) exchangeOAuthRefreshToken(ctx *gin.Context, authentication oauthClientAuthentication, input oauthTokenInput) {
	plainRefreshToken := strings.TrimSpace(input.RefreshToken)
	response, err := exchangeOAuthRefreshTokenValue(h.dbFor(ctx), authentication, plainRefreshToken, time.Now())
	if errors.Is(err, errOAuthInvalidClient) {
		oauthError(ctx, http.StatusUnauthorized, "invalid_client", "Client authentication failed")
		return
	}
	if err != nil {
		oauthError(ctx, http.StatusBadRequest, "invalid_grant", "Refresh token is invalid, expired, consumed, or revoked")
		return
	}
	writeOAuthTokenResponse(ctx, response)
}

func writeOAuthTokenResponse(ctx *gin.Context, response oauthTokenResponse) {
	ctx.Header("Cache-Control", "no-store")
	ctx.Header("Pragma", "no-cache")
	ctx.JSON(http.StatusOK, response)
}

type oauthSentinelError string

func (e oauthSentinelError) Error() string { return string(e) }

const (
	errOAuthInvalidGrant  oauthSentinelError = "invalid OAuth grant"
	errOAuthInvalidClient oauthSentinelError = "invalid OAuth client"
)
