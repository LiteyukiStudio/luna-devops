package api

import (
	"errors"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func exchangeOAuthAuthorizationCodeValue(
	db *gorm.DB,
	authentication oauthClientAuthentication,
	plainCode string,
	redirectURI string,
	verifier string,
	now time.Time,
) (oauthTokenResponse, error) {
	var response oauthTokenResponse
	err := db.Transaction(func(tx *gorm.DB) error {
		application, err := lockOAuthClientApplication(tx, authentication, false)
		if err != nil {
			return err
		}
		var snapshot model.OAuthAuthorizationCode
		if err := tx.First(&snapshot, "code_hash = ?", hashToken(strings.TrimSpace(plainCode))).Error; err != nil {
			return err
		}
		if !validOAuthAuthorizationCode(snapshot, application.ID, redirectURI, verifier, now) ||
			!oauthApplicationAllowsScope(application, snapshot.Scope) {
			return errOAuthInvalidGrant
		}
		user, err := lockOAuthExchangeUser(tx, snapshot.UserID, snapshot.Scope)
		if err != nil {
			return err
		}
		grant, err := ensureOAuthGrant(tx, application, user, snapshot.Scope, snapshot.CreatedAt)
		if err != nil {
			return err
		}
		var code model.OAuthAuthorizationCode
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(
			&code,
			"id = ? and code_hash = ?",
			snapshot.ID,
			snapshot.CodeHash,
		).Error; err != nil {
			return err
		}
		if !validOAuthAuthorizationCode(code, application.ID, redirectURI, verifier, now) ||
			code.UserID != snapshot.UserID || code.Scope != snapshot.Scope {
			return errOAuthInvalidGrant
		}
		issued, err := issueOAuthTokens(tx, application, grant, code.Scope, id.New("ofam"), now)
		if err != nil {
			return err
		}
		if err := tx.Model(&code).Updates(map[string]any{"grant_id": grant.ID, "consumed_at": now}).Error; err != nil {
			return err
		}
		response = issued
		return nil
	})
	return response, err
}

func validOAuthAuthorizationCode(
	code model.OAuthAuthorizationCode,
	applicationID string,
	redirectURI string,
	verifier string,
	now time.Time,
) bool {
	return code.ApplicationID == applicationID &&
		code.ConsumedAt == nil &&
		code.ExpiresAt.After(now) &&
		code.RedirectURI == redirectURI &&
		verifyPKCE(verifier, code.CodeChallenge)
}

func exchangeOAuthRefreshTokenValue(
	db *gorm.DB,
	authentication oauthClientAuthentication,
	plainRefreshToken string,
	now time.Time,
) (oauthTokenResponse, error) {
	var response oauthTokenResponse
	reused := false
	err := db.Transaction(func(tx *gorm.DB) error {
		application, err := lockOAuthClientApplication(tx, authentication, true)
		if err != nil {
			return err
		}
		var snapshot model.OAuthRefreshToken
		if err := tx.First(&snapshot, "token_hash = ?", hashToken(strings.TrimSpace(plainRefreshToken))).Error; err != nil {
			return err
		}
		if snapshot.ApplicationID != application.ID || snapshot.FamilyID == "" ||
			!oauthApplicationAllowsScope(application, snapshot.Scope) {
			return errOAuthInvalidGrant
		}
		if _, err := lockOAuthExchangeUser(tx, snapshot.UserID, snapshot.Scope); err != nil {
			return err
		}
		var grant model.OAuthGrant
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(
			&grant,
			"id = ? and application_id = ? and user_id = ? and revoked_at is null",
			snapshot.GrantID,
			application.ID,
			snapshot.UserID,
		).Error; err != nil {
			return err
		}
		var refreshToken model.OAuthRefreshToken
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(
			&refreshToken,
			"id = ? and token_hash = ?",
			snapshot.ID,
			snapshot.TokenHash,
		).Error; err != nil {
			return err
		}
		if refreshToken.ApplicationID != application.ID ||
			refreshToken.GrantID != grant.ID ||
			refreshToken.UserID != snapshot.UserID ||
			refreshToken.FamilyID == "" ||
			refreshToken.FamilyID != snapshot.FamilyID ||
			refreshToken.Scope != snapshot.Scope ||
			refreshToken.RevokedAt != nil ||
			!refreshToken.ExpiresAt.After(now) {
			return errOAuthInvalidGrant
		}
		if refreshToken.ConsumedAt != nil {
			if err := revokeLockedOAuthFamily(tx, grant.ID, refreshToken.FamilyID, now); err != nil {
				return err
			}
			reused = true
			return nil
		}
		if err := tx.Model(&refreshToken).Update("consumed_at", now).Error; err != nil {
			return err
		}
		issued, err := issueOAuthTokens(tx, application, grant, refreshToken.Scope, refreshToken.FamilyID, now)
		if err != nil {
			return err
		}
		response = issued
		return nil
	})
	if err != nil {
		return oauthTokenResponse{}, err
	}
	if reused {
		return oauthTokenResponse{}, errOAuthInvalidGrant
	}
	return response, nil
}

func exchangeOAuthDeviceCodeValue(
	db *gorm.DB,
	authentication oauthClientAuthentication,
	plainDeviceCode string,
	now time.Time,
) (oauthTokenResponse, string, error) {
	var response oauthTokenResponse
	protocolError := ""
	err := db.Transaction(func(tx *gorm.DB) error {
		application, err := lockOAuthClientApplication(tx, authentication, true)
		if err != nil {
			return err
		}
		var snapshot model.OAuthDeviceAuthorization
		if err := tx.First(&snapshot, "device_code_hash = ?", hashToken(strings.TrimSpace(plainDeviceCode))).Error; err != nil {
			return err
		}
		if snapshot.ApplicationID != application.ID || snapshot.ConsumedAt != nil {
			protocolError = "invalid_grant"
			return nil
		}
		if !snapshot.ExpiresAt.After(now) {
			protocolError = "expired_token"
			return nil
		}
		if snapshot.LastPolledAt != nil && now.Before(snapshot.LastPolledAt.Add(time.Duration(snapshot.IntervalSeconds)*time.Second)) {
			var authorization model.OAuthDeviceAuthorization
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&authorization, "id = ?", snapshot.ID).Error; err != nil {
				return err
			}
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
		switch snapshot.Status {
		case "pending", "denied":
			var authorization model.OAuthDeviceAuthorization
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&authorization, "id = ?", snapshot.ID).Error; err != nil {
				return err
			}
			if err := tx.Model(&authorization).Updates(map[string]any{"last_polled_at": now, "updated_at": now}).Error; err != nil {
				return err
			}
			if snapshot.Status == "pending" {
				protocolError = "authorization_pending"
			} else {
				protocolError = "access_denied"
			}
			return nil
		case "approved":
		default:
			protocolError = "invalid_grant"
			return nil
		}
		if snapshot.UserID == nil || snapshot.ApprovedAt == nil ||
			!oauthApplicationAllowsScope(application, snapshot.Scope) {
			return errOAuthInvalidGrant
		}
		user, err := lockOAuthExchangeUser(tx, *snapshot.UserID, snapshot.Scope)
		if err != nil {
			return err
		}
		grant, err := ensureOAuthGrant(tx, application, user, snapshot.Scope, *snapshot.ApprovedAt)
		if err != nil {
			return err
		}
		var authorization model.OAuthDeviceAuthorization
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(
			&authorization,
			"id = ? and device_code_hash = ?",
			snapshot.ID,
			snapshot.DeviceCodeHash,
		).Error; err != nil {
			return err
		}
		if authorization.ApplicationID != application.ID ||
			authorization.Status != "approved" ||
			authorization.ConsumedAt != nil ||
			authorization.UserID == nil ||
			*authorization.UserID != user.ID ||
			authorization.ApprovedAt == nil ||
			!authorization.ApprovedAt.Equal(*snapshot.ApprovedAt) ||
			authorization.Scope != snapshot.Scope ||
			!authorization.ExpiresAt.After(now) {
			return errOAuthInvalidGrant
		}
		issued, err := issueOAuthTokens(tx, application, grant, authorization.Scope, id.New("ofam"), now)
		if err != nil {
			return err
		}
		if err := tx.Model(&authorization).Updates(map[string]any{
			"grant_id":       grant.ID,
			"status":         "consumed",
			"last_polled_at": now,
			"consumed_at":    now,
			"updated_at":     now,
		}).Error; err != nil {
			return err
		}
		response = issued
		return nil
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return oauthTokenResponse{}, "invalid_grant", nil
		}
		return oauthTokenResponse{}, "", err
	}
	return response, protocolError, nil
}
