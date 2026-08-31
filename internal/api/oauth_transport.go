package api

import (
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
)

type oauthApplicationInput struct {
	Name                    string   `json:"name" binding:"required"`
	Description             string   `json:"description"`
	HomepageURL             string   `json:"homepageUrl"`
	LogoURL                 string   `json:"logoUrl"`
	RedirectURIs            []string `json:"redirectUris" binding:"required"`
	AllowedScopes           string   `json:"allowedScopes" binding:"required"`
	AccessTokenLifetimeDays int      `json:"accessTokenLifetimeDays"`
}

type oauthDeviceAuthorizationInput struct {
	ClientID string `form:"client_id" binding:"required"`
	Scope    string `form:"scope"`
}

type oauthTokenInput struct {
	GrantType    string `form:"grant_type" binding:"required"`
	ClientID     string `form:"client_id"`
	ClientSecret string `form:"client_secret"`
	Code         string `form:"code" binding:"required_if=GrantType authorization_code"`
	CodeVerifier string `form:"code_verifier" binding:"required_if=GrantType authorization_code"`
	RedirectURI  string `form:"redirect_uri" binding:"required_if=GrantType authorization_code"`
	RefreshToken string `form:"refresh_token" binding:"required_if=GrantType refresh_token"`
	DeviceCode   string `form:"device_code" binding:"required_if=GrantType urn:ietf:params:oauth:grant-type:device_code"`
}

type oauthTokenRevocationInput struct {
	Token         string `form:"token" binding:"required"`
	TokenTypeHint string `form:"token_type_hint" binding:"omitempty,oneof=access_token refresh_token"`
	ClientID      string `form:"client_id"`
	ClientSecret  string `form:"client_secret"`
}

type oauthApplicationResponse struct {
	ID                      string     `json:"id"`
	OwnerUserID             *string    `json:"ownerUserId,omitempty"`
	Name                    string     `json:"name"`
	Description             string     `json:"description"`
	HomepageURL             string     `json:"homepageUrl"`
	LogoURL                 string     `json:"logoUrl"`
	ClientID                string     `json:"clientId"`
	RedirectURIs            []string   `json:"redirectUris"`
	AllowedScopes           string     `json:"allowedScopes"`
	AccessTokenLifetimeDays int        `json:"accessTokenLifetimeDays"`
	RevokedAt               *time.Time `json:"revokedAt"`
	CreatedAt               time.Time  `json:"createdAt"`
	UpdatedAt               time.Time  `json:"updatedAt"`
}

type oauthApplicationSecretResponse struct {
	Application  oauthApplicationResponse `json:"application"`
	ClientSecret string                   `json:"clientSecret"`
}

type oauthGrantResponse struct {
	ID          string                   `json:"id"`
	Application oauthApplicationResponse `json:"application"`
	Scope       string                   `json:"scope"`
	CreatedAt   time.Time                `json:"createdAt"`
	UpdatedAt   time.Time                `json:"updatedAt"`
}

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

type oauthAuthorizationDecisionResponse struct {
	RedirectURL string `json:"redirectUrl"`
}

type oauthAuthorizationServerMetadataResponse struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	DeviceAuthorizationEndpoint       string   `json:"device_authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	RevocationEndpoint                string   `json:"revocation_endpoint"`
	ResponseTypesSupported            []string `json:"response_types_supported"`
	GrantTypesSupported               []string `json:"grant_types_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported"`
}

type oauthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    *int64 `json:"expires_in,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope"`
}

type oauthProtocolErrorResponse struct {
	Code        string `json:"error"`
	Description string `json:"error_description"`
	RequestID   string `json:"requestId"`
}

type oauthDeviceAuthorizationResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int64  `json:"expires_in"`
	Interval                int    `json:"interval"`
}

type oauthDeviceVerificationResponse struct {
	Application oauthApplicationResponse `json:"application"`
	Scope       string                   `json:"scope"`
	UserCode    string                   `json:"userCode"`
	ExpiresAt   time.Time                `json:"expiresAt"`
}

type oauthDeviceVerificationInput struct {
	Approved bool   `json:"approved"`
	UserCode string `json:"userCode" binding:"required"`
}

type oauthDeviceVerificationResult struct {
	Status string `json:"status"`
}

func oauthApplicationToResponse(application model.OAuthApplication) oauthApplicationResponse {
	redirectURIs := decodeStringList(application.RedirectURIs)
	if redirectURIs == nil {
		redirectURIs = []string{}
	}
	return oauthApplicationResponse{
		ID:                      application.ID,
		OwnerUserID:             application.OwnerUserID,
		Name:                    application.Name,
		Description:             application.Description,
		HomepageURL:             application.HomepageURL,
		LogoURL:                 application.LogoURL,
		ClientID:                application.ClientID,
		RedirectURIs:            redirectURIs,
		AllowedScopes:           application.AllowedScopes,
		AccessTokenLifetimeDays: application.AccessTokenLifetimeDays,
		RevokedAt:               application.RevokedAt,
		CreatedAt:               application.CreatedAt,
		UpdatedAt:               application.UpdatedAt,
	}
}
