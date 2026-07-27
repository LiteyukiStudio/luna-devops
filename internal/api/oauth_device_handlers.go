package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type oauthDeviceVerificationInput struct {
	Approved bool   `json:"approved"`
	UserCode string `json:"userCode" binding:"required"`
}

func (h *Handlers) StartOAuthDeviceAuthorization(ctx *gin.Context) {
	baseURL := strings.TrimRight(h.externalBaseURL(ctx), "/")
	if baseURL == "" {
		oauthError(ctx, http.StatusServiceUnavailable, "temporarily_unavailable", "PUBLIC_BASE_URL is required")
		return
	}
	clientID := strings.TrimSpace(ctx.PostForm("client_id"))
	if !h.allowOAuthClientAttempt(ctx, clientID) {
		return
	}
	var application model.OAuthApplication
	if clientID != lunaCLIClientID || h.db.First(
		&application,
		"id = ? and client_id = ? and revoked_at is null",
		lunaCLIApplicationID,
		lunaCLIClientID,
	).Error != nil {
		oauthError(ctx, http.StatusBadRequest, "invalid_client", "The device authorization client is not available")
		return
	}
	requestedScope := strings.TrimSpace(ctx.PostForm("scope"))
	scope := ""
	if requestedScope != "" {
		scope = normalizeAccessTokenScope(requestedScope)
		if scope == "" {
			oauthError(ctx, http.StatusBadRequest, "invalid_scope", "Requested scope is invalid")
			return
		}
	}

	plainDeviceCode := "lyo_device_" + randomHex(32)
	userCode := newOAuthDeviceUserCode()
	now := time.Now()
	authorization := model.OAuthDeviceAuthorization{
		ID:              id.New("odvc"),
		ApplicationID:   application.ID,
		DeviceCodeHash:  hashToken(plainDeviceCode),
		UserCodeHash:    hashToken(normalizeOAuthDeviceUserCode(userCode)),
		Scope:           scope,
		Status:          "pending",
		IntervalSeconds: int(oauthDevicePollInterval / time.Second),
		ExpiresAt:       now.Add(oauthDeviceCodeTTL),
	}
	if err := h.db.Create(&authorization).Error; err != nil {
		oauthError(ctx, http.StatusInternalServerError, "server_error", "Device authorization could not be created")
		return
	}
	verificationURI := baseURL + "/oauth/device"
	ctx.Header("Cache-Control", "no-store")
	ctx.Header("Pragma", "no-cache")
	ctx.JSON(http.StatusOK, gin.H{
		"device_code":               plainDeviceCode,
		"user_code":                 userCode,
		"verification_uri":          verificationURI,
		"verification_uri_complete": verificationURI + "?user_code=" + userCode,
		"expires_in":                int64(oauthDeviceCodeTTL / time.Second),
		"interval":                  authorization.IntervalSeconds,
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
	authorization, application, ok := h.oauthDeviceVerificationRequest(ctx, user, userCode)
	if !ok {
		return
	}
	ctx.JSON(http.StatusOK, gin.H{
		"application": oauthApplicationToResponse(application),
		"scope":       authorization.Scope,
		"userCode":    formatOAuthDeviceUserCode(userCode),
		"expiresAt":   authorization.ExpiresAt,
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
	var authorization model.OAuthDeviceAuthorization
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(
			&authorization,
			"user_code_hash = ?",
			hashToken(userCode),
		).Error; err != nil {
			return err
		}
		now := time.Now()
		if authorization.Status != "pending" || !authorization.ExpiresAt.After(now) {
			return errOAuthInvalidGrant
		}
		var application model.OAuthApplication
		if err := tx.First(
			&application,
			"id = ? and client_id = ? and revoked_at is null",
			authorization.ApplicationID,
			lunaCLIClientID,
		).Error; err != nil {
			return err
		}
		if !input.Approved {
			return tx.Model(&authorization).Updates(map[string]any{
				"status":     "denied",
				"denied_at":  now,
				"updated_at": now,
			}).Error
		}
		scope := authorization.Scope
		if scope == "" {
			scope = recommendedOAuthScope(user)
		}
		if scope == "" || !userCanCreateAccessTokenScope(user, scope) {
			return errOAuthInvalidScope
		}
		grant, err := ensureOAuthGrant(tx, authorization.ApplicationID, user.ID, scope, now)
		if err != nil {
			return err
		}
		authorization.GrantID = &grant.ID
		authorization.UserID = &user.ID
		authorization.Scope = scope
		return tx.Model(&authorization).Updates(map[string]any{
			"grant_id":    grant.ID,
			"user_id":     user.ID,
			"scope":       scope,
			"status":      "approved",
			"approved_at": now,
			"updated_at":  now,
		}).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, errOAuthInvalidGrant) {
		writeErrorCode(ctx, http.StatusNotFound, "oauth.device.invalid_code", "Device verification code is invalid or expired")
		return
	}
	if errors.Is(err, errOAuthInvalidScope) {
		writeErrorCode(ctx, http.StatusForbidden, "oauth.scope.forbidden", "Requested OAuth scope is not allowed")
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
	h.audit(user.ID, "oauth_device."+status, authorization.ID, true, authorization.Scope)
	ctx.JSON(http.StatusOK, gin.H{"status": status})
}

func (h *Handlers) exchangeOAuthDeviceCode(ctx *gin.Context, application model.OAuthApplication) {
	plainDeviceCode := strings.TrimSpace(ctx.PostForm("device_code"))
	if plainDeviceCode == "" {
		oauthError(ctx, http.StatusBadRequest, "invalid_request", "device_code is required")
		return
	}
	var response oauthTokenResponse
	protocolError := ""
	err := h.db.Transaction(func(tx *gorm.DB) error {
		var authorization model.OAuthDeviceAuthorization
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(
			&authorization,
			"device_code_hash = ?",
			hashToken(plainDeviceCode),
		).Error; err != nil {
			return err
		}
		now := time.Now()
		if authorization.ApplicationID != application.ID || authorization.ConsumedAt != nil {
			protocolError = "invalid_grant"
			return nil
		}
		if !authorization.ExpiresAt.After(now) {
			protocolError = "expired_token"
			return nil
		}
		if authorization.LastPolledAt != nil && now.Before(authorization.LastPolledAt.Add(time.Duration(authorization.IntervalSeconds)*time.Second)) {
			authorization.IntervalSeconds += 5
			if err := tx.Model(&authorization).Updates(map[string]any{
				"interval_seconds": authorization.IntervalSeconds,
				"last_polled_at":   now,
				"updated_at":       now,
			}).Error; err != nil {
				return err
			}
			protocolError = "slow_down"
			return nil
		}
		if err := tx.Model(&authorization).Updates(map[string]any{"last_polled_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		switch authorization.Status {
		case "pending":
			protocolError = "authorization_pending"
			return nil
		case "denied":
			protocolError = "access_denied"
			return nil
		case "approved":
		default:
			protocolError = "invalid_grant"
			return nil
		}
		if authorization.GrantID == nil || authorization.UserID == nil {
			protocolError = "invalid_grant"
			return nil
		}
		var grant model.OAuthGrant
		if err := tx.First(
			&grant,
			"id = ? and application_id = ? and user_id = ? and revoked_at is null",
			*authorization.GrantID,
			application.ID,
			*authorization.UserID,
		).Error; err != nil {
			return err
		}
		issued, err := issueOAuthTokens(tx, application, grant, now)
		if err != nil {
			return err
		}
		if err := tx.Model(&authorization).Updates(map[string]any{
			"status":      "consumed",
			"consumed_at": now,
			"updated_at":  now,
		}).Error; err != nil {
			return err
		}
		response = issued
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		oauthError(ctx, http.StatusBadRequest, "invalid_grant", "Device code is invalid")
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

func (h *Handlers) oauthDeviceVerificationRequest(ctx *gin.Context, user model.User, userCode string) (model.OAuthDeviceAuthorization, model.OAuthApplication, bool) {
	if len(userCode) != 8 {
		writeErrorCode(ctx, http.StatusNotFound, "oauth.device.invalid_code", "Device verification code is invalid or expired")
		return model.OAuthDeviceAuthorization{}, model.OAuthApplication{}, false
	}
	var authorization model.OAuthDeviceAuthorization
	if h.db.First(
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
	if h.db.First(&application, "id = ? and revoked_at is null", authorization.ApplicationID).Error != nil {
		writeErrorCode(ctx, http.StatusNotFound, "oauth.application.not_found", "OAuth application not found")
		return model.OAuthDeviceAuthorization{}, model.OAuthApplication{}, false
	}
	if authorization.Scope == "" {
		authorization.Scope = recommendedOAuthScope(user)
	}
	if authorization.Scope == "" || !userCanCreateAccessTokenScope(user, authorization.Scope) {
		writeErrorCode(ctx, http.StatusForbidden, "oauth.scope.forbidden", "Requested OAuth scope is not allowed")
		return model.OAuthDeviceAuthorization{}, model.OAuthApplication{}, false
	}
	return authorization, application, true
}

func ensureOAuthGrant(tx *gorm.DB, applicationID, userID, scope string, now time.Time) (model.OAuthGrant, error) {
	var grant model.OAuthGrant
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(
		&grant,
		"application_id = ? and user_id = ? and revoked_at is null",
		applicationID,
		userID,
	).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.OAuthGrant{}, err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		grant = model.OAuthGrant{ID: id.New("ogrt"), ApplicationID: applicationID, UserID: userID, Scope: scope}
		return grant, tx.Create(&grant).Error
	}
	if grant.Scope == scope {
		return grant, nil
	}
	if err := revokeOAuthGrant(tx, grant.ID, now); err != nil {
		return model.OAuthGrant{}, err
	}
	grant = model.OAuthGrant{ID: id.New("ogrt"), ApplicationID: applicationID, UserID: userID, Scope: scope}
	return grant, tx.Create(&grant).Error
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
