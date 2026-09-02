package identityapi

import (
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func lockOwnedActiveOAuthApplication(tx *gorm.DB, applicationID, ownerUserID string) (model.OAuthApplication, error) {
	var application model.OAuthApplication
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(
		&application,
		"id = ? and owner_user_id = ? and revoked_at is null",
		strings.TrimSpace(applicationID),
		strings.TrimSpace(ownerUserID),
	).Error
	return application, err
}

func updateOwnedOAuthApplication(db *gorm.DB, applicationID, ownerUserID string, next model.OAuthApplication) (model.OAuthApplication, error) {
	var application model.OAuthApplication
	err := db.Transaction(func(tx *gorm.DB) error {
		locked, err := lockOwnedActiveOAuthApplication(tx, applicationID, ownerUserID)
		if err != nil {
			return err
		}
		now := time.Now()
		scopesChanged := locked.AllowedScopes != next.AllowedScopes
		if err := tx.Model(&locked).Updates(map[string]any{
			"name":                       next.Name,
			"description":                next.Description,
			"homepage_url":               next.HomepageURL,
			"logo_url":                   next.LogoURL,
			"redirect_uris":              next.RedirectURIs,
			"allowed_scopes":             next.AllowedScopes,
			"access_token_lifetime_days": next.AccessTokenLifetimeDays,
			"updated_at":                 now,
		}).Error; err != nil {
			return err
		}
		locked.Name = next.Name
		locked.Description = next.Description
		locked.HomepageURL = next.HomepageURL
		locked.LogoURL = next.LogoURL
		locked.RedirectURIs = next.RedirectURIs
		locked.AllowedScopes = next.AllowedScopes
		locked.AccessTokenLifetimeDays = next.AccessTokenLifetimeDays
		locked.UpdatedAt = now
		if scopesChanged {
			if err := revokeOAuthApplication(tx, locked.ID, now); err != nil {
				return err
			}
		}
		application = locked
		return nil
	})
	return application, err
}

func rotateOwnedOAuthApplicationSecret(db *gorm.DB, applicationID, ownerUserID, clientSecretHash string) (model.OAuthApplication, error) {
	var application model.OAuthApplication
	err := db.Transaction(func(tx *gorm.DB) error {
		locked, err := lockOwnedActiveOAuthApplication(tx, applicationID, ownerUserID)
		if err != nil {
			return err
		}
		now := time.Now()
		if err := tx.Model(&locked).Updates(map[string]any{
			"client_secret_hash": clientSecretHash,
			"updated_at":         now,
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.OAuthRefreshToken{}).
			Where("application_id = ? and revoked_at is null", locked.ID).
			Update("revoked_at", now).Error; err != nil {
			return err
		}
		locked.ClientSecretHash = clientSecretHash
		locked.UpdatedAt = now
		application = locked
		return nil
	})
	return application, err
}

func deleteOwnedOAuthApplication(db *gorm.DB, applicationID, ownerUserID string) (model.OAuthApplication, error) {
	var application model.OAuthApplication
	err := db.Transaction(func(tx *gorm.DB) error {
		locked, err := lockOwnedActiveOAuthApplication(tx, applicationID, ownerUserID)
		if err != nil {
			return err
		}
		now := time.Now()
		if err := tx.Model(&locked).Updates(map[string]any{
			"revoked_at": now,
			"updated_at": now,
		}).Error; err != nil {
			return err
		}
		if err := revokeOAuthApplication(tx, locked.ID, now); err != nil {
			return err
		}
		locked.RevokedAt = &now
		locked.UpdatedAt = now
		application = locked
		return nil
	})
	return application, err
}
