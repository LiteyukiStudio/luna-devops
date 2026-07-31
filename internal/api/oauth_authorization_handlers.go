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
	"gorm.io/gorm/clause"
)

type oauthAuthorizationRequest struct {
	Application             oauthApplicationResponse `json:"application"`
	Scope                   string                   `json:"scope"`
	AccessTokenLifetimeDays int                      `json:"accessTokenLifetimeDays"`
	PreviouslyAuthorized    bool                     `json:"previouslyAuthorized"`
}

type oauthAuthorizationDecisionInput struct {
	Approved            bool   `json:"approved"`
	ClientID            string `json:"clientId" binding:"required"`
	RedirectURI         string `json:"redirectUri" binding:"required"`
	Scope               string `json:"scope" binding:"required"`
	State               string `json:"state"`
	CodeChallenge       string `json:"codeChallenge" binding:"required"`
	CodeChallengeMethod string `json:"codeChallengeMethod" binding:"required"`
}

type oauthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    *int64 `json:"expires_in,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope"`
}

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
	previouslyAuthorized := h.dbFor(ctx).First(
		&grant,
		"application_id = ? and user_id = ? and scope = ? and revoked_at is null",
		application.ID,
		user.ID,
		scope,
	).Error == nil
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
		ctx.JSON(http.StatusOK, gin.H{"redirectUrl": appendOAuthRedirectValues(input.RedirectURI, values)})
		return
	}

	plainCode := "lyo_code_" + randomHex(32)
	var grant model.OAuthGrant
	err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(
			&grant,
			"application_id = ? and user_id = ? and revoked_at is null",
			application.ID,
			user.ID,
		).Error
		if err != nil && err != gorm.ErrRecordNotFound {
			return err
		}
		if err == gorm.ErrRecordNotFound {
			grant = model.OAuthGrant{ID: id.New("ogrt"), ApplicationID: application.ID, UserID: user.ID, Scope: scope}
			if err := tx.Create(&grant).Error; err != nil {
				return err
			}
		} else if grant.Scope != scope {
			if err := revokeOAuthGrant(tx, grant.ID, time.Now()); err != nil {
				return err
			}
			grant = model.OAuthGrant{ID: id.New("ogrt"), ApplicationID: application.ID, UserID: user.ID, Scope: scope}
			if err := tx.Create(&grant).Error; err != nil {
				return err
			}
		}
		return tx.Create(&model.OAuthAuthorizationCode{
			ID: id.New("ocod"), ApplicationID: application.ID, GrantID: grant.ID, UserID: user.ID,
			CodeHash: hashToken(plainCode), RedirectURI: input.RedirectURI, Scope: scope,
			CodeChallenge: input.CodeChallenge, CodeChallengeMethod: "S256", ExpiresAt: time.Now().Add(oauthAuthorizationCodeTTL),
		}).Error
	})
	if err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "oauth.authorization.failed", "OAuth authorization could not be completed")
		return
	}
	values := url.Values{"code": {plainCode}}
	if input.State != "" {
		values.Set("state", input.State)
	}
	h.auditWithContext(user.ID, "oauth_grant.authorize", grant.ID, true, scope, ctx.Request.Context())
	ctx.JSON(http.StatusOK, gin.H{"redirectUrl": appendOAuthRedirectValues(input.RedirectURI, values)})
}

func (h *Handlers) ExchangeOAuthToken(ctx *gin.Context) {
	grantType := strings.TrimSpace(ctx.PostForm("grant_type"))
	allowPublicClient := grantType == oauthDeviceCodeGrantType || grantType == "refresh_token"
	application, ok := h.authenticateOAuthTokenClient(ctx, allowPublicClient)
	if !ok {
		return
	}
	switch grantType {
	case "authorization_code":
		h.exchangeOAuthAuthorizationCode(ctx, application)
	case "refresh_token":
		h.exchangeOAuthRefreshToken(ctx, application)
	case oauthDeviceCodeGrantType:
		h.exchangeOAuthDeviceCode(ctx, application)
	default:
		oauthError(ctx, http.StatusBadRequest, "unsupported_grant_type", "Supported grant types are authorization_code, refresh_token, and device_code")
	}
}

func (h *Handlers) RevokeOAuthToken(ctx *gin.Context) {
	application, ok := h.authenticateOAuthTokenClient(ctx, true)
	if !ok {
		return
	}
	plainToken := strings.TrimSpace(ctx.PostForm("token"))
	if plainToken != "" {
		tokenHash := hashToken(plainToken)
		_ = h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
			grantID := ""
			var accessToken model.AccessToken
			if err := tx.First(
				&accessToken,
				"token_hash = ? and oauth_application_id = ?",
				tokenHash,
				application.ID,
			).Error; err == nil {
				grantID = accessToken.OAuthGrantID
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if grantID == "" {
				var refreshToken model.OAuthRefreshToken
				if err := tx.First(
					&refreshToken,
					"token_hash = ? and application_id = ?",
					tokenHash,
					application.ID,
				).Error; err == nil {
					grantID = refreshToken.GrantID
				} else if !errors.Is(err, gorm.ErrRecordNotFound) {
					return err
				}
			}
			if grantID == "" {
				return nil
			}
			return revokeOAuthGrant(tx, grantID, time.Now())
		})
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
	ctx.JSON(http.StatusOK, gin.H{
		"issuer":                                baseURL,
		"authorization_endpoint":                baseURL + "/oauth/authorize",
		"device_authorization_endpoint":         baseURL + "/api/v1/oauth/device/authorization",
		"token_endpoint":                        baseURL + "/api/v1/oauth/token",
		"revocation_endpoint":                   baseURL + "/api/v1/oauth/revoke",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token", oauthDeviceCodeGrantType},
		"token_endpoint_auth_methods_supported": []string{"client_secret_basic", "client_secret_post", "none"},
		"code_challenge_methods_supported":      []string{"S256"},
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
	if strings.TrimSpace(requestedScope) == "" || scope == "" || !oauthScopeSubset(scope, application.AllowedScopes) || !userCanAuthorizeOAuthScope(user, scope) {
		writeErrorCode(ctx, http.StatusForbidden, "oauth.scope.forbidden", "Requested OAuth scope is not allowed")
		return model.OAuthApplication{}, "", false
	}
	if challengeMethod != "S256" || !validPKCEChallenge(challenge) {
		writeErrorCode(ctx, http.StatusBadRequest, "oauth.pkce.required", "PKCE with the S256 method is required")
		return model.OAuthApplication{}, "", false
	}
	return application, scope, true
}

func (h *Handlers) exchangeOAuthAuthorizationCode(ctx *gin.Context, application model.OAuthApplication) {
	plainCode := strings.TrimSpace(ctx.PostForm("code"))
	redirectURI := strings.TrimSpace(ctx.PostForm("redirect_uri"))
	verifier := ctx.PostForm("code_verifier")
	var response oauthTokenResponse
	err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		var code model.OAuthAuthorizationCode
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&code, "code_hash = ?", hashToken(plainCode)).Error; err != nil {
			return err
		}
		if code.ApplicationID != application.ID || code.ConsumedAt != nil || !code.ExpiresAt.After(time.Now()) || code.RedirectURI != redirectURI || !verifyPKCE(verifier, code.CodeChallenge) {
			return errOAuthInvalidGrant
		}
		var grant model.OAuthGrant
		if err := tx.First(&grant, "id = ? and application_id = ? and user_id = ? and revoked_at is null", code.GrantID, application.ID, code.UserID).Error; err != nil {
			return err
		}
		now := time.Now()
		if err := tx.Model(&code).Update("consumed_at", now).Error; err != nil {
			return err
		}
		issued, err := issueOAuthTokens(tx, application, grant, now)
		if err == nil {
			response = issued
		}
		return err
	})
	if err != nil {
		oauthError(ctx, http.StatusBadRequest, "invalid_grant", "Authorization code is invalid, expired, consumed, or does not match the request")
		return
	}
	writeOAuthTokenResponse(ctx, response)
}

func (h *Handlers) exchangeOAuthRefreshToken(ctx *gin.Context, application model.OAuthApplication) {
	plainRefreshToken := strings.TrimSpace(ctx.PostForm("refresh_token"))
	var response oauthTokenResponse
	reused := false
	err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		var refreshToken model.OAuthRefreshToken
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&refreshToken, "token_hash = ?", hashToken(plainRefreshToken)).Error; err != nil {
			return err
		}
		if refreshToken.ApplicationID != application.ID || refreshToken.RevokedAt != nil || !refreshToken.ExpiresAt.After(time.Now()) {
			return errOAuthInvalidGrant
		}
		if refreshToken.ConsumedAt != nil {
			if err := revokeOAuthGrant(tx, refreshToken.GrantID, time.Now()); err != nil {
				return err
			}
			reused = true
			return nil
		}
		var grant model.OAuthGrant
		if err := tx.First(&grant, "id = ? and application_id = ? and user_id = ? and revoked_at is null", refreshToken.GrantID, application.ID, refreshToken.UserID).Error; err != nil {
			return err
		}
		now := time.Now()
		if err := tx.Model(&refreshToken).Update("consumed_at", now).Error; err != nil {
			return err
		}
		issued, err := issueOAuthTokens(tx, application, grant, now)
		if err == nil {
			response = issued
		}
		return err
	})
	if err != nil || reused {
		oauthError(ctx, http.StatusBadRequest, "invalid_grant", "Refresh token is invalid, expired, consumed, or revoked")
		return
	}
	writeOAuthTokenResponse(ctx, response)
}

func issueOAuthTokens(tx *gorm.DB, application model.OAuthApplication, grant model.OAuthGrant, now time.Time) (oauthTokenResponse, error) {
	plainAccessToken := "lyo_" + randomHex(32)
	accessToken := model.AccessToken{
		ID: id.New("tok"), UserID: grant.UserID, Name: application.Name, Scope: grant.Scope,
		TokenHash: hashToken(plainAccessToken), Source: "oauth", OAuthApplicationID: application.ID, OAuthGrantID: grant.ID,
	}
	response := oauthTokenResponse{AccessToken: plainAccessToken, TokenType: "Bearer", Scope: oauthScopeText(grant.Scope)}
	if application.AccessTokenLifetimeDays > 0 {
		expiresAt := now.Add(time.Duration(application.AccessTokenLifetimeDays) * 24 * time.Hour)
		accessToken.ExpiresAt = &expiresAt
		expiresIn := int64(expiresAt.Sub(now).Seconds())
		response.ExpiresIn = &expiresIn
	}
	if err := tx.Create(&accessToken).Error; err != nil {
		return oauthTokenResponse{}, err
	}
	if application.AccessTokenLifetimeDays == 0 {
		return response, nil
	}
	plainRefreshToken := "lyo_refresh_" + randomHex(32)
	refreshToken := model.OAuthRefreshToken{
		ID: id.New("ortk"), ApplicationID: application.ID, GrantID: grant.ID, UserID: grant.UserID,
		TokenHash: hashToken(plainRefreshToken), Scope: grant.Scope, ExpiresAt: now.Add(oauthRefreshTokenTTL),
	}
	if err := tx.Create(&refreshToken).Error; err != nil {
		return oauthTokenResponse{}, err
	}
	response.RefreshToken = plainRefreshToken
	return response, nil
}

func writeOAuthTokenResponse(ctx *gin.Context, response oauthTokenResponse) {
	ctx.Header("Cache-Control", "no-store")
	ctx.Header("Pragma", "no-cache")
	ctx.JSON(http.StatusOK, response)
}

type oauthSentinelError string

func (e oauthSentinelError) Error() string { return string(e) }

const errOAuthInvalidGrant oauthSentinelError = "invalid OAuth grant"
