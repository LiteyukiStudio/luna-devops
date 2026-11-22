package identityapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *Handlers) GetOAuthAuthorizationRequest(ctx *gin.Context) {
	user, ok := h.oauthCookieUser(ctx)
	if !ok {
		return
	}
	if ctx.Query("response_type") != "code" {
		writeErrorCode(ctx, http.StatusBadRequest, "oauth.response_type.invalid", "Only the authorization code response type is supported")
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
		true,
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
		!input.Approved,
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
	}
	authentication, ok := h.authenticateOAuthTokenClient(
		ctx,
		false,
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
	default:
		oauthError(ctx, http.StatusBadRequest, "unsupported_grant_type", "Supported grant types are authorization_code and refresh_token")
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
		telemetry.LogError(ctx.Request.Context(), "OAuth token revocation failed", "oauth.revoke.failed",
			"oauth.revoke", "database.oauth_revoke.failed", err)
		telemetry.MarkHTTPErrorLogged(ctx)
		oauthError(ctx, http.StatusInternalServerError, "server_error", "Token revocation failed")
		return
	}
	ctx.Header("Cache-Control", "no-store")
	ctx.Status(http.StatusOK)
}

func (h *Handlers) RegisterOAuthClient(ctx *gin.Context) {
	if !h.allowOAuthDynamicRegistrationAttempt(ctx) {
		return
	}
	mediaType, _, mediaErr := mime.ParseMediaType(ctx.GetHeader("Content-Type"))
	if mediaErr != nil || !strings.EqualFold(mediaType, "application/json") {
		oauthError(ctx, http.StatusBadRequest, "invalid_client_metadata", "Content-Type must be application/json")
		return
	}
	if ctx.Request.Body == nil {
		oauthError(ctx, http.StatusBadRequest, "invalid_client_metadata", "Client registration JSON payload is required")
		return
	}
	rawBody, err := io.ReadAll(io.LimitReader(ctx.Request.Body, oauthDynamicRegistrationMaxBodyBytes+1))
	if err != nil || len(rawBody) > oauthDynamicRegistrationMaxBodyBytes || len(bytes.TrimSpace(rawBody)) == 0 {
		oauthError(ctx, http.StatusBadRequest, "invalid_client_metadata", "Client registration request body is invalid or exceeds maximum size")
		return
	}
	var input oauthDynamicClientRegistrationInput
	if err := json.Unmarshal(rawBody, &input); err != nil {
		oauthError(ctx, http.StatusBadRequest, "invalid_client_metadata", "Client registration JSON payload is invalid")
		return
	}

	redirectURIs, ok := validateDynamicClientRedirectURIs(input.RedirectURIs)
	if !ok {
		oauthError(ctx, http.StatusBadRequest, "invalid_redirect_uri", "At least one valid HTTPS or loopback HTTP redirect URI is required")
		return
	}

	authMethod := strings.TrimSpace(input.TokenEndpointAuthMethod)
	if authMethod == "" {
		authMethod = model.OAuthTokenEndpointAuthMethodClientSecretBasic
	}
	switch authMethod {
	case model.OAuthTokenEndpointAuthMethodNone,
		model.OAuthTokenEndpointAuthMethodClientSecretBasic,
		model.OAuthTokenEndpointAuthMethodClientSecretPost:
	default:
		oauthError(ctx, http.StatusBadRequest, "invalid_client_metadata", "Unsupported token_endpoint_auth_method")
		return
	}

	grantTypes, ok := normalizeDynamicClientGrantTypes(input.GrantTypes)
	if !ok {
		oauthError(ctx, http.StatusBadRequest, "invalid_client_metadata", "Supported grant_types are authorization_code and refresh_token")
		return
	}
	responseTypes, ok := normalizeDynamicClientResponseTypes(input.ResponseTypes)
	if !ok {
		oauthError(ctx, http.StatusBadRequest, "invalid_client_metadata", "Supported response_types is code")
		return
	}

	allowedScopes := strings.Join(authz.SupportedOAuthScopes(), ",")
	if rawScope := strings.TrimSpace(input.Scope); rawScope != "" {
		allowedScopes = normalizeOAuthScope(rawScope)
		if allowedScopes == "" {
			oauthError(ctx, http.StatusBadRequest, "invalid_client_metadata", "Requested OAuth scope contains unsupported values")
			return
		}
	}

	clientName := strings.TrimSpace(input.ClientName)
	softwareID := strings.TrimSpace(input.SoftwareID)
	softwareVersion := strings.TrimSpace(input.SoftwareVersion)
	clientURI := strings.TrimSpace(input.ClientURI)
	logoURI := strings.TrimSpace(input.LogoURI)
	if len(clientName) > 100 || len(softwareID) > 128 || len(softwareVersion) > 64 ||
		hasControlCharacters(clientName) || hasControlCharacters(softwareID) || hasControlCharacters(softwareVersion) ||
		!validDynamicClientMetadataURI(clientURI) || !validDynamicClientMetadataURI(logoURI) {
		oauthError(ctx, http.StatusBadRequest, "invalid_client_metadata", "Client presentation or software metadata is invalid")
		return
	}
	if clientName == "" {
		clientName = "MCP OAuth Client"
	}

	now := time.Now().UTC()
	plainSecret := ""
	secretHash := ""
	var secretExpiresAt *int64
	if authMethod != model.OAuthTokenEndpointAuthMethodNone {
		plainSecret = "lyo_secret_" + randomHex(32)
		secretHash = hashToken(plainSecret)
		zero := int64(0)
		secretExpiresAt = &zero
	}

	application := model.OAuthApplication{
		ID:                      id.New("oapp"),
		OwnerUserID:             nil,
		Name:                    clientName,
		Description:             "",
		HomepageURL:             clientURI,
		LogoURL:                 logoURI,
		ClientID:                "lyo_app_" + randomHex(16),
		ClientSecretHash:        secretHash,
		TokenEndpointAuthMethod: authMethod,
		RegistrationSource:      model.OAuthRegistrationSourceDynamic,
		GrantTypes:              encodeStringList(grantTypes),
		RedirectURIs:            encodeStringList(redirectURIs),
		AllowedScopes:           allowedScopes,
		AccessTokenLifetimeDays: oauthDefaultDynamicClientLifetimeDays,
		CreatedAt:               now,
		UpdatedAt:               now,
	}
	if err := h.dbFor(ctx).Create(&application).Error; err != nil {
		telemetry.LogError(ctx.Request.Context(), "Dynamic OAuth client registration save failed", "oauth.register.failed",
			"oauth.register", "database.oauth_register.failed", err)
		telemetry.MarkHTTPErrorLogged(ctx)
		oauthError(ctx, http.StatusInternalServerError, "server_error", "Dynamic client registration could not be saved")
		return
	}
	h.auditWithContext("", "oauth_application.register", application.ID, true, authMethod, ctx.Request.Context())

	ctx.Header("Cache-Control", "no-store")
	ctx.Header("Pragma", "no-cache")
	ctx.JSON(http.StatusCreated, oauthDynamicClientRegistrationResponse{
		ClientID:                application.ClientID,
		ClientSecret:            plainSecret,
		ClientIDIssuedAt:        application.CreatedAt.Unix(),
		ClientSecretExpiresAt:   secretExpiresAt,
		RedirectURIs:            redirectURIs,
		TokenEndpointAuthMethod: authMethod,
		GrantTypes:              grantTypes,
		ResponseTypes:           responseTypes,
		ClientName:              application.Name,
		ClientURI:               clientURI,
		LogoURI:                 logoURI,
		Scope:                   oauthScopeText(application.AllowedScopes),
		SoftwareID:              softwareID,
		SoftwareVersion:         softwareVersion,
	})
}

func normalizeDynamicClientGrantTypes(values []string) ([]string, bool) {
	if values == nil {
		return []string{"authorization_code", "refresh_token"}, true
	}
	if len(values) == 0 {
		return nil, false
	}
	hasAuthCode := false
	hasRefresh := false
	for _, raw := range values {
		switch strings.TrimSpace(raw) {
		case "authorization_code":
			hasAuthCode = true
		case "refresh_token":
			hasRefresh = true
		default:
			return nil, false
		}
	}
	if !hasAuthCode {
		return nil, false
	}
	if hasRefresh {
		return []string{"authorization_code", "refresh_token"}, true
	}
	return []string{"authorization_code"}, true
}

func normalizeDynamicClientResponseTypes(values []string) ([]string, bool) {
	if values == nil {
		return []string{"code"}, true
	}
	if len(values) == 0 {
		return nil, false
	}
	for _, raw := range values {
		if strings.TrimSpace(raw) != "code" {
			return nil, false
		}
	}
	return []string{"code"}, true
}

func hasControlCharacters(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

func (h *Handlers) GetOAuthAuthorizationServerMetadata(ctx *gin.Context) {
	baseURL := strings.TrimRight(h.externalBaseURL(ctx), "/")
	if baseURL == "" {
		writeErrorCode(ctx, http.StatusServiceUnavailable, "oauth.public_base_url.required", "PUBLIC_BASE_URL is required")
		return
	}
	ctx.Header("Cache-Control", "no-store")
	ctx.JSON(http.StatusOK, oauthAuthorizationServerMetadataResponse{
		Issuer:                            baseURL,
		AuthorizationEndpoint:             baseURL + "/oauth/authorize",
		TokenEndpoint:                     baseURL + "/api/v1/oauth/token",
		RevocationEndpoint:                baseURL + "/api/v1/oauth/revoke",
		RegistrationEndpoint:              baseURL + "/api/v1/oauth/register",
		ScopesSupported:                   authz.SupportedOAuthScopes(),
		ResponseTypesSupported:            []string{"code"},
		GrantTypesSupported:               []string{"authorization_code", "refresh_token"},
		TokenEndpointAuthMethodsSupported: []string{"none", "client_secret_basic", "client_secret_post"},
		CodeChallengeMethodsSupported:     []string{"S256"},
	})
}

func (h *Handlers) GetOAuthProtectedResourceMetadata(ctx *gin.Context) {
	baseURL := strings.TrimRight(h.externalBaseURL(ctx), "/")
	if baseURL == "" {
		writeErrorCode(ctx, http.StatusServiceUnavailable, "oauth.public_base_url.required", "PUBLIC_BASE_URL is required")
		return
	}
	ctx.Header("Cache-Control", "no-store")
	ctx.JSON(http.StatusOK, oauthProtectedResourceMetadataResponse{
		Resource:               baseURL + "/api/v1/mcp",
		AuthorizationServers:   []string{baseURL},
		BearerMethodsSupported: []string{"header"},
		ScopesSupported:        authz.SupportedOAuthScopes(),
		ResourceDocumentation:  "https://luna-devops.liteyuki.org/use/mcp",
	})
}

func (h *Handlers) validateOAuthAuthorizationRequest(ctx *gin.Context, clientID, redirectURI, requestedScope, challenge, challengeMethod string, user model.User, allowDynamicScopeNarrowing bool) (model.OAuthApplication, string, bool) {
	var application model.OAuthApplication
	if h.dbFor(ctx).First(&application, "client_id = ? and revoked_at is null", strings.TrimSpace(clientID)).Error != nil {
		writeErrorCode(ctx, http.StatusNotFound, "oauth.application.not_found", "OAuth application not found")
		return model.OAuthApplication{}, "", false
	}
	if !oauthApplicationAllowsGrantType(application, "authorization_code") {
		writeErrorCode(ctx, http.StatusForbidden, "oauth.grant_type.forbidden", "OAuth application does not allow authorization code")
		return model.OAuthApplication{}, "", false
	}
	if !oauthApplicationRedirectURIAllowed(application, redirectURI) {
		writeErrorCode(ctx, http.StatusBadRequest, "oauth.redirect_uri.invalid", "Redirect URI does not match the registered application")
		return model.OAuthApplication{}, "", false
	}
	var scope string
	if allowDynamicScopeNarrowing && application.RegistrationSource == model.OAuthRegistrationSourceDynamic {
		if strings.TrimSpace(requestedScope) == "" {
			scope = authz.FilterUserAuthorizableOAuthScope(user.Role, application.AllowedScopes)
		} else {
			normalized := normalizeOAuthScope(requestedScope)
			if normalized == "" || !oauthApplicationAllowsScope(application, normalized) {
				writeErrorCode(ctx, http.StatusForbidden, "oauth.scope.forbidden", "Requested OAuth scope is not allowed")
				return model.OAuthApplication{}, "", false
			}
			scope = authz.FilterUserAuthorizableOAuthScope(user.Role, normalized)
		}
		if scope == "" {
			writeErrorCode(ctx, http.StatusForbidden, "oauth.scope.forbidden", "Requested OAuth scope is not allowed")
			return model.OAuthApplication{}, "", false
		}
	} else {
		scope = normalizeOAuthScope(requestedScope)
		if strings.TrimSpace(requestedScope) == "" || scope == "" || !oauthApplicationAllowsScope(application, scope) || !userCanAuthorizeOAuthScope(user, scope) {
			writeErrorCode(ctx, http.StatusForbidden, "oauth.scope.forbidden", "Requested OAuth scope is not allowed")
			return model.OAuthApplication{}, "", false
		}
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
	if errors.Is(err, errOAuthUnauthorizedClient) {
		oauthError(ctx, http.StatusBadRequest, "unauthorized_client", "Client is not authorized to use this grant type")
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
	response, err := exchangeOAuthRefreshTokenValue(h.dbFor(ctx), authentication, plainRefreshToken, input.Scope, time.Now())
	if errors.Is(err, errOAuthInvalidClient) {
		oauthError(ctx, http.StatusUnauthorized, "invalid_client", "Client authentication failed")
		return
	}
	if errors.Is(err, errOAuthUnauthorizedClient) {
		oauthError(ctx, http.StatusBadRequest, "unauthorized_client", "Client is not authorized to use this grant type")
		return
	}
	if errors.Is(err, errOAuthInvalidScope) {
		oauthError(ctx, http.StatusBadRequest, "invalid_scope", "Requested scope exceeds the scope granted by the resource owner")
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
