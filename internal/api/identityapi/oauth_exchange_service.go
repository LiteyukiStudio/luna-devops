package identityapi

import (
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
		if !oauthApplicationAllowsGrantType(application, "authorization_code") {
			return errOAuthUnauthorizedClient
		}
		var snapshot model.OAuthAuthorizationCode
		if err := tx.First(&snapshot, "code_hash = ?", hashToken(strings.TrimSpace(plainCode))).Error; err != nil {
			return err
		}
		if !validOAuthAuthorizationCode(snapshot, application.ID, redirectURI, verifier, now) ||
			!oauthApplicationAllowsScope(application, snapshot.Scope) {
			return errOAuthInvalidGrant
		}
		user, err := lockOAuthExchangeUser(tx, application, snapshot.UserID, snapshot.Scope)
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
	requestedScope string,
	now time.Time,
) (oauthTokenResponse, error) {
	var response oauthTokenResponse
	reused := false
	err := db.Transaction(func(tx *gorm.DB) error {
		application, err := lockOAuthClientApplication(tx, authentication, true)
		if err != nil {
			return err
		}
		if !oauthApplicationAllowsGrantType(application, "refresh_token") {
			return errOAuthUnauthorizedClient
		}
		var snapshot model.OAuthRefreshToken
		if err := tx.First(&snapshot, "token_hash = ?", hashToken(strings.TrimSpace(plainRefreshToken))).Error; err != nil {
			return err
		}
		if snapshot.ApplicationID != application.ID || snapshot.FamilyID == "" ||
			!oauthApplicationAllowsScope(application, snapshot.Scope) {
			return errOAuthInvalidGrant
		}
		if _, err := lockOAuthExchangeUser(tx, application, snapshot.UserID, snapshot.Scope); err != nil {
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
		accessScope := refreshToken.Scope
		if rawScope := strings.TrimSpace(requestedScope); rawScope != "" {
			normalized := normalizeOAuthScope(rawScope)
			if normalized == "" || !oauthScopeSubset(normalized, refreshToken.Scope) {
				return errOAuthInvalidScope
			}
			accessScope = normalized
		}
		if err := tx.Model(&refreshToken).Update("consumed_at", now).Error; err != nil {
			return err
		}
		issued, err := issueOAuthTokensWithRefreshScope(tx, application, grant, accessScope, refreshToken.Scope, refreshToken.FamilyID, now)
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
