package identityapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *Handlers) StartOAuthDeviceAuthorization(ctx *gin.Context) {
	baseURL := strings.TrimRight(h.externalBaseURL(ctx), "/")
	if baseURL == "" {
		oauthError(ctx, http.StatusServiceUnavailable, "temporarily_unavailable", "PUBLIC_BASE_URL is required")
		return
	}
	var input oauthDeviceAuthorizationInput
	if !bindOAuthForm(ctx, &input) {
		return
	}
	if requestedScopes, present := ctx.GetPostFormArray("scope"); present {
		for _, requestedScope := range requestedScopes {
			if strings.TrimSpace(requestedScope) != "" {
				oauthError(ctx, http.StatusBadRequest, "invalid_scope", "Luna CLI device authorization does not accept a requested scope")
				return
			}
		}
	}
	clientID := strings.TrimSpace(input.ClientID)
	if !h.allowOAuthClientAttempt(ctx, clientID) {
		return
	}
	var application model.OAuthApplication
	if clientID != lunaCLIClientID || h.dbFor(ctx).First(
		&application,
		"id = ? and client_id = ? and revoked_at is null",
		lunaCLIApplicationID,
		lunaCLIClientID,
	).Error != nil {
		oauthError(ctx, http.StatusBadRequest, "invalid_client", "The device authorization client is not available")
		return
	}
	plainDeviceCode := "lyo_device_" + randomHex(32)
	userCode := newOAuthDeviceUserCode()
	now := time.Now()
	authorization := model.OAuthDeviceAuthorization{
		ID:              id.New("odvc"),
		ApplicationID:   application.ID,
		DeviceCodeHash:  hashToken(plainDeviceCode),
		UserCodeHash:    hashToken(normalizeOAuthDeviceUserCode(userCode)),
		Status:          "pending",
		IntervalSeconds: int(oauthDevicePollInterval / time.Second),
		ExpiresAt:       now.Add(oauthDeviceCodeTTL),
	}
	if err := h.dbFor(ctx).Create(&authorization).Error; err != nil {
		oauthError(ctx, http.StatusInternalServerError, "server_error", "Device authorization could not be created")
		return
	}
	verificationURI := baseURL + "/oauth/device"
	ctx.Header("Cache-Control", "no-store")
	ctx.Header("Pragma", "no-cache")
	ctx.JSON(http.StatusOK, oauthDeviceAuthorizationResponse{
		DeviceCode:              plainDeviceCode,
		UserCode:                userCode,
		VerificationURI:         verificationURI,
		VerificationURIComplete: verificationURI + "?user_code=" + userCode,
		ExpiresIn:               int64(oauthDeviceCodeTTL / time.Second),
		Interval:                authorization.IntervalSeconds,
	})
}

func (h *Handlers) GetOAuthDeviceVerification(ctx *gin.Context) {
	user, ok := h.oauthCookieUser(ctx)
	if !ok {
		return
	}
	if !h.allowOAuthDeviceVerificationAttempt(ctx, user.ID) {
		return
	}
	userCode := normalizeOAuthDeviceUserCode(ctx.Query("user_code"))
	authorization, application, ok := h.oauthDeviceVerificationRequest(ctx, userCode)
	if !ok {
		return
	}
	ctx.JSON(http.StatusOK, oauthDeviceVerificationResponse{
		Application: oauthApplicationToResponse(application),
		UserCode:    formatOAuthDeviceUserCode(userCode),
		ExpiresAt:   authorization.ExpiresAt,
	})
}

func (h *Handlers) DecideOAuthDeviceVerification(ctx *gin.Context) {
	user, ok := h.oauthCookieUser(ctx)
	if !ok {
		return
	}
	if !h.allowOAuthDeviceVerificationAttempt(ctx, user.ID) {
		return
	}
	var input oauthDeviceVerificationInput
	if !bindJSON(ctx, &input) {
		return
	}
	userCode := normalizeOAuthDeviceUserCode(input.UserCode)
	var snapshot model.OAuthDeviceAuthorization
	if err := h.dbFor(ctx).First(&snapshot, "user_code_hash = ?", hashToken(userCode)).Error; err != nil {
		writeErrorCode(ctx, http.StatusNotFound, "oauth.device.invalid_code", "Device verification code is invalid or expired")
		return
	}
	var authorization model.OAuthDeviceAuthorization
	err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		updated, err := decideOAuthDeviceConsent(tx, snapshot.ID, user.ID, input.Approved)
		authorization = updated
		return err
	})
	if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, errOAuthInvalidGrant) {
		writeErrorCode(ctx, http.StatusNotFound, "oauth.device.invalid_code", "Device verification code is invalid or expired")
		return
	}
	if err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "oauth.device.decision_failed", "Device authorization could not be updated")
		return
	}
	status := "denied"
	if input.Approved {
		status = "approved"
	}
	h.auditWithContext(user.ID, "oauth_device."+status, authorization.ID, true, "", ctx.Request.Context())
	ctx.JSON(http.StatusOK, oauthDeviceVerificationResult{Status: status})
}

func (h *Handlers) exchangeOAuthDeviceCode(ctx *gin.Context, authentication oauthClientAuthentication, deviceCode string) {
	plainDeviceCode := strings.TrimSpace(deviceCode)
	response, protocolError, err := exchangeOAuthDeviceCodeValue(h.dbFor(ctx), authentication, plainDeviceCode, time.Now())
	if errors.Is(err, errOAuthInvalidClient) {
		oauthError(ctx, http.StatusUnauthorized, "invalid_client", "Client authentication failed")
		return
	}
	if errors.Is(err, errOAuthInvalidGrant) || errors.Is(err, errOAuthInvalidScope) {
		oauthError(ctx, http.StatusBadRequest, "invalid_grant", "Device code authorization is no longer valid")
		return
	}
	if err != nil {
		oauthError(ctx, http.StatusInternalServerError, "server_error", "Device token exchange failed")
		return
	}
	switch protocolError {
	case "authorization_pending":
		oauthError(ctx, http.StatusBadRequest, protocolError, "The user has not completed authorization")
	case "slow_down":
		oauthError(ctx, http.StatusBadRequest, protocolError, "Polling is too frequent")
	case "access_denied":
		oauthError(ctx, http.StatusBadRequest, protocolError, "The user denied the authorization request")
	case "expired_token":
		oauthError(ctx, http.StatusBadRequest, protocolError, "The device code has expired")
	case "invalid_grant":
		oauthError(ctx, http.StatusBadRequest, protocolError, "The device code is invalid or consumed")
	default:
		writeOAuthTokenResponse(ctx, response)
	}
}

func (h *Handlers) oauthDeviceVerificationRequest(ctx *gin.Context, userCode string) (model.OAuthDeviceAuthorization, model.OAuthApplication, bool) {
	if len(userCode) != 8 {
		writeErrorCode(ctx, http.StatusNotFound, "oauth.device.invalid_code", "Device verification code is invalid or expired")
		return model.OAuthDeviceAuthorization{}, model.OAuthApplication{}, false
	}
	var authorization model.OAuthDeviceAuthorization
	if h.dbFor(ctx).First(
		&authorization,
		"user_code_hash = ? and status = ? and expires_at > ?",
		hashToken(userCode),
		"pending",
		time.Now(),
	).Error != nil {
		writeErrorCode(ctx, http.StatusNotFound, "oauth.device.invalid_code", "Device verification code is invalid or expired")
		return model.OAuthDeviceAuthorization{}, model.OAuthApplication{}, false
	}
	var application model.OAuthApplication
	if h.dbFor(ctx).First(&application, "id = ? and revoked_at is null", authorization.ApplicationID).Error != nil {
		writeErrorCode(ctx, http.StatusNotFound, "oauth.application.not_found", "OAuth application not found")
		return model.OAuthDeviceAuthorization{}, model.OAuthApplication{}, false
	}
	if !isLunaCLIApplication(application) {
		writeErrorCode(ctx, http.StatusNotFound, "oauth.application.not_found", "OAuth application not found")
		return model.OAuthDeviceAuthorization{}, model.OAuthApplication{}, false
	}
	return authorization, application, true
}

func newOAuthDeviceUserCode() string {
	return formatOAuthDeviceUserCode(strings.ToUpper(randomHex(4)))
}

func normalizeOAuthDeviceUserCode(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	return strings.Map(func(char rune) rune {
		if char >= 'A' && char <= 'F' || char >= '0' && char <= '9' {
			return char
		}
		return -1
	}, value)
}

func formatOAuthDeviceUserCode(value string) string {
	value = normalizeOAuthDeviceUserCode(value)
	if len(value) != 8 {
		return value
	}
	return value[:4] + "-" + value[4:]
}

const errOAuthInvalidScope oauthSentinelError = "invalid OAuth scope"
