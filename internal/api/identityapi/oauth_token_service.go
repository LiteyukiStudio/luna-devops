package identityapi

import (
	"crypto/subtle"
	"errors"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// OAuth mutations use the application row as their linearization point. The
// remaining locks are always acquired after it, so refresh rotation cannot
// insert a credential after a family, grant, or application revoke returns.
func lockOAuthApplication(tx *gorm.DB, applicationID string, requireActive bool) (model.OAuthApplication, error) {
	var application model.OAuthApplication
	query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", strings.TrimSpace(applicationID))
	if requireActive {
		query = query.Where("revoked_at is null")
	}
	if err := query.First(&application).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.OAuthApplication{}, errOAuthInvalidGrant
		}
		return model.OAuthApplication{}, err
	}
	return application, nil
}

func lockOAuthClientApplication(tx *gorm.DB, authentication oauthClientAuthentication, allowPublic bool) (model.OAuthApplication, error) {
	application, err := lockOAuthApplication(tx, authentication.applicationID, true)
	if err != nil {
		return model.OAuthApplication{}, err
	}
	if application.ClientID != authentication.clientID {
		return model.OAuthApplication{}, errOAuthInvalidClient
	}
	if authentication.public {
		if allowPublic && application.ID == lunaCLIApplicationID && application.ClientID == lunaCLIClientID {
			return application, nil
		}
		return model.OAuthApplication{}, errOAuthInvalidClient
	}
	if authentication.clientSecretHash == "" || subtle.ConstantTimeCompare(
		[]byte(application.ClientSecretHash),
		[]byte(authentication.clientSecretHash),
	) != 1 {
		return model.OAuthApplication{}, errOAuthInvalidClient
	}
	return application, nil
}

func lockOAuthExchangeUser(tx *gorm.DB, application model.OAuthApplication, userID, scope string) (model.User, error) {
	scope = normalizeOAuthScopeForApplication(application, scope)
	if scope == "" {
		return model.User{}, errOAuthInvalidScope
	}
	var user model.User
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, "id = ?", strings.TrimSpace(userID)).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.User{}, errOAuthInvalidGrant
		}
		return model.User{}, err
	}
	if user.Disabled || (!isLunaCLIApplication(application) && !userCanAuthorizeOAuthScope(user, scope)) {
		return model.User{}, errOAuthInvalidGrant
	}
	return user, nil
}

func lockOAuthExchangeAuthority(tx *gorm.DB, applicationID, userID, scope string) (model.OAuthApplication, model.User, error) {
	application, err := lockOAuthApplication(tx, applicationID, true)
	if err != nil {
		return model.OAuthApplication{}, model.User{}, err
	}
	scope = normalizeOAuthScopeForApplication(application, scope)
	if scope == "" {
		return model.OAuthApplication{}, model.User{}, errOAuthInvalidScope
	}
	if !oauthApplicationAllowsScope(application, scope) {
		return model.OAuthApplication{}, model.User{}, errOAuthInvalidGrant
	}
	user, err := lockOAuthExchangeUser(tx, application, userID, scope)
	if err != nil {
		return model.OAuthApplication{}, model.User{}, err
	}
	return application, user, nil
}

func oauthApplicationAllowsScope(application model.OAuthApplication, scope string) bool {
	allowed := strings.TrimSpace(application.AllowedScopes)
	return allowed == "*" || oauthScopeSubset(scope, allowed)
}

func ensureOAuthGrant(tx *gorm.DB, application model.OAuthApplication, user model.User, approvedScope string, consentedAt time.Time) (model.OAuthGrant, error) {
	approvedScope = normalizeOAuthScopeForApplication(application, approvedScope)
	if approvedScope == "" || application.ID == "" || user.ID == "" || consentedAt.IsZero() ||
		!oauthApplicationAllowsScope(application, approvedScope) ||
		(!isLunaCLIApplication(application) && !userCanAuthorizeOAuthScope(user, approvedScope)) {
		return model.OAuthGrant{}, errOAuthInvalidGrant
	}
	var revokedGrant model.OAuthGrant
	revokedErr := tx.Where(
		"application_id = ? and user_id = ? and revoked_at is not null",
		application.ID,
		user.ID,
	).Order("revoked_at desc").First(&revokedGrant).Error
	if revokedErr != nil && !errors.Is(revokedErr, gorm.ErrRecordNotFound) {
		return model.OAuthGrant{}, revokedErr
	}
	if revokedErr == nil && revokedGrant.RevokedAt != nil && !consentedAt.After(*revokedGrant.RevokedAt) {
		return model.OAuthGrant{}, errOAuthInvalidGrant
	}

	var grant model.OAuthGrant
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(
		&grant,
		"application_id = ? and user_id = ? and revoked_at is null",
		application.ID,
		user.ID,
	).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return model.OAuthGrant{}, err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		grant = model.OAuthGrant{ID: id.New("ogrt"), ApplicationID: application.ID, UserID: user.ID, Scope: approvedScope}
		return grant, tx.Create(&grant).Error
	}
	if grant.Scope == approvedScope {
		return grant, nil
	}
	mergedScope := mergeOAuthScopes(grant.Scope, approvedScope)
	if mergedScope == "" {
		return model.OAuthGrant{}, errOAuthInvalidScope
	}
	if err := tx.Model(&grant).Update("scope", mergedScope).Error; err != nil {
		return model.OAuthGrant{}, err
	}
	grant.Scope = mergedScope
	return grant, nil
}

func mergeOAuthScopes(existing, approved string) string {
	if strings.TrimSpace(existing) == "" {
		return normalizeOAuthScope(approved)
	}
	return normalizeOAuthScope(existing + "," + approved)
}

func recordOAuthAuthorizationConsent(tx *gorm.DB, code *model.OAuthAuthorizationCode) error {
	if code == nil {
		return errOAuthInvalidGrant
	}
	application, _, err := lockOAuthExchangeAuthority(tx, code.ApplicationID, code.UserID, code.Scope)
	if err != nil {
		return err
	}
	if !exactRedirectURIAllowed(application, code.RedirectURI) {
		return errOAuthInvalidGrant
	}
	if code.CreatedAt.IsZero() {
		code.CreatedAt = time.Now()
	}
	return tx.Create(code).Error
}

func decideOAuthDeviceConsent(tx *gorm.DB, authorizationID, userID string, approved bool) (model.OAuthDeviceAuthorization, error) {
	var snapshot model.OAuthDeviceAuthorization
	if err := tx.First(&snapshot, "id = ?", authorizationID).Error; err != nil {
		return model.OAuthDeviceAuthorization{}, err
	}
	application, err := lockOAuthApplication(tx, snapshot.ApplicationID, true)
	if err != nil {
		return model.OAuthDeviceAuthorization{}, err
	}
	now := time.Now()
	var authorization model.OAuthDeviceAuthorization
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(
		&authorization,
		"id = ? and application_id = ?",
		authorizationID,
		application.ID,
	).Error; err != nil {
		return model.OAuthDeviceAuthorization{}, err
	}
	if authorization.Status != "pending" || !authorization.ExpiresAt.After(now) {
		return model.OAuthDeviceAuthorization{}, errOAuthInvalidGrant
	}
	if !approved {
		authorization.Status = "denied"
		authorization.DeniedAt = &now
		return authorization, tx.Model(&authorization).Updates(map[string]any{
			"status": "denied", "denied_at": now, "updated_at": now,
		}).Error
	}
	if !isLunaCLIApplication(application) {
		return model.OAuthDeviceAuthorization{}, errOAuthInvalidGrant
	}
	user, err := lockOAuthExchangeUser(tx, application, userID, lunaCLIFullAccessScope)
	if err != nil {
		return model.OAuthDeviceAuthorization{}, err
	}
	authorization.UserID = &user.ID
	authorization.Status = "approved"
	authorization.ApprovedAt = &now
	return authorization, tx.Model(&authorization).Updates(map[string]any{
		"user_id": user.ID, "status": "approved", "approved_at": now, "updated_at": now,
	}).Error
}

func issueOAuthTokens(tx *gorm.DB, application model.OAuthApplication, grant model.OAuthGrant, scope, familyID string, now time.Time) (oauthTokenResponse, error) {
	scope = normalizeOAuthScopeForApplication(application, scope)
	if scope == "" || familyID == "" || !oauthScopeSubset(scope, grant.Scope) || !oauthApplicationAllowsScope(application, scope) {
		return oauthTokenResponse{}, errOAuthInvalidGrant
	}
	plainAccessToken := "lyo_" + randomHex(32)
	accessToken := model.AccessToken{
		ID: id.New("tok"), UserID: grant.UserID, Name: application.Name, Scope: scope,
		TokenHash: hashToken(plainAccessToken), Source: "oauth", OAuthApplicationID: application.ID, OAuthGrantID: grant.ID,
		OAuthFamilyID: familyID,
	}
	response := oauthTokenResponse{AccessToken: plainAccessToken, TokenType: "Bearer"}
	if !isLunaCLIApplication(application) {
		response.Scope = oauthScopeText(scope)
	}
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
		FamilyID: familyID, TokenHash: hashToken(plainRefreshToken), Scope: scope, ExpiresAt: now.Add(oauthRefreshTokenTTL),
	}
	if err := tx.Create(&refreshToken).Error; err != nil {
		return oauthTokenResponse{}, err
	}
	response.RefreshToken = plainRefreshToken
	return response, nil
}

func invalidateOAuthPendingConsent(tx *gorm.DB, applicationID, userID string, now time.Time) error {
	if err := tx.Model(&model.OAuthAuthorizationCode{}).
		Where("application_id = ? and user_id = ? and consumed_at is null", applicationID, userID).
		Update("consumed_at", now).Error; err != nil {
		return err
	}
	return tx.Model(&model.OAuthDeviceAuthorization{}).
		Where("application_id = ? and user_id = ? and consumed_at is null and status = ?", applicationID, userID, "approved").
		Updates(map[string]any{"status": "denied", "denied_at": now, "consumed_at": now, "updated_at": now}).Error
}

func invalidateOAuthApplicationPendingConsent(tx *gorm.DB, applicationID string, now time.Time) error {
	if err := tx.Model(&model.OAuthAuthorizationCode{}).
		Where("application_id = ? and consumed_at is null", applicationID).
		Update("consumed_at", now).Error; err != nil {
		return err
	}
	return tx.Model(&model.OAuthDeviceAuthorization{}).
		Where("application_id = ? and consumed_at is null", applicationID).
		Updates(map[string]any{"status": "denied", "denied_at": now, "consumed_at": now, "updated_at": now}).Error
}

func revokeLockedOAuthFamily(tx *gorm.DB, grantID, familyID string, now time.Time) error {
	if err := tx.Model(&model.AccessToken{}).
		Where("oauth_grant_id = ? and oauth_family_id = ? and revoked_at is null", grantID, familyID).
		Update("revoked_at", now).Error; err != nil {
		return err
	}
	return tx.Model(&model.OAuthRefreshToken{}).
		Where("grant_id = ? and family_id = ? and revoked_at is null", grantID, familyID).
		Update("revoked_at", now).Error
}

func lockOAuthGrantForMutation(tx *gorm.DB, grantID, expectedApplicationID string) (model.OAuthGrant, error) {
	var snapshot model.OAuthGrant
	if err := tx.First(&snapshot, "id = ?", strings.TrimSpace(grantID)).Error; err != nil {
		return model.OAuthGrant{}, err
	}
	applicationID := snapshot.ApplicationID
	if expectedApplicationID != "" && applicationID != expectedApplicationID {
		return model.OAuthGrant{}, errOAuthInvalidGrant
	}
	if _, err := lockOAuthApplication(tx, applicationID, false); err != nil {
		return model.OAuthGrant{}, err
	}
	var grant model.OAuthGrant
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(
		&grant,
		"id = ? and application_id = ?",
		grantID,
		applicationID,
	).Error; err != nil {
		return model.OAuthGrant{}, err
	}
	return grant, nil
}

func revokeOAuthFamily(tx *gorm.DB, grantID, familyID string, now time.Time) error {
	grantID = strings.TrimSpace(grantID)
	familyID = strings.TrimSpace(familyID)
	if grantID == "" || familyID == "" {
		return errOAuthInvalidGrant
	}
	if _, err := lockOAuthGrantForMutation(tx, grantID, ""); err != nil {
		return err
	}
	return revokeLockedOAuthFamily(tx, grantID, familyID, now)
}

func revokeOAuthTokenFamily(tx *gorm.DB, applicationID, tokenHash string, now time.Time) error {
	grantID := ""
	familyID := ""
	var accessToken model.AccessToken
	if err := tx.First(
		&accessToken,
		"token_hash = ? and oauth_application_id = ?",
		tokenHash,
		applicationID,
	).Error; err == nil {
		grantID = accessToken.OAuthGrantID
		familyID = accessToken.OAuthFamilyID
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if grantID == "" {
		var refreshToken model.OAuthRefreshToken
		if err := tx.First(
			&refreshToken,
			"token_hash = ? and application_id = ?",
			tokenHash,
			applicationID,
		).Error; err == nil {
			grantID = refreshToken.GrantID
			familyID = refreshToken.FamilyID
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
	}
	if grantID == "" || familyID == "" {
		return nil
	}
	if _, err := lockOAuthGrantForMutation(tx, grantID, applicationID); err != nil {
		return err
	}
	return revokeLockedOAuthFamily(tx, grantID, familyID, now)
}

func revokeLockedOAuthGrant(tx *gorm.DB, grant model.OAuthGrant, now time.Time, invalidatePending bool) error {
	if invalidatePending {
		if err := invalidateOAuthPendingConsent(tx, grant.ApplicationID, grant.UserID, now); err != nil {
			return err
		}
	}
	if err := tx.Model(&grant).Where("revoked_at is null").Update("revoked_at", now).Error; err != nil {
		return err
	}
	if err := tx.Model(&model.AccessToken{}).Where("oauth_grant_id = ? and revoked_at is null", grant.ID).Update("revoked_at", now).Error; err != nil {
		return err
	}
	return tx.Model(&model.OAuthRefreshToken{}).Where("grant_id = ? and revoked_at is null", grant.ID).Update("revoked_at", now).Error
}

func revokeOAuthGrant(tx *gorm.DB, grantID string, now time.Time) error {
	grant, err := lockOAuthGrantForMutation(tx, grantID, "")
	if err != nil {
		return err
	}
	return revokeLockedOAuthGrant(tx, grant, now, true)
}

func revokeOAuthApplication(tx *gorm.DB, applicationID string, now time.Time) error {
	application, err := lockOAuthApplication(tx, applicationID, false)
	if err != nil {
		return err
	}
	var grants []model.OAuthGrant
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("application_id = ? and revoked_at is null", application.ID).
		Order("id").Find(&grants).Error; err != nil {
		return err
	}
	if err := invalidateOAuthApplicationPendingConsent(tx, application.ID, now); err != nil {
		return err
	}
	for _, grant := range grants {
		if err := revokeLockedOAuthGrant(tx, grant, now, false); err != nil {
			return err
		}
	}
	return nil
}
