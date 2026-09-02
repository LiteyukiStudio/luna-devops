package identityapi

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// initialAdminAdvisoryLockID is the ASCII representation of "LUNAADMI".
// It serializes first-user creation across API replicas.
const initialAdminAdvisoryLockID int64 = 0x4c554e4141444d49

var (
	ErrInitialAdminConfigInvalid       = errors.New("initial administrator configuration is invalid")
	ErrInitialAdminDatabaseUnavailable = errors.New("initial administrator database is unavailable")
	ErrInitialAdminRecoveryRequired    = errors.New("database contains users but no active platform administrator")
)

// EnsureInitialAdmin creates the configured administrator only when the user
// table has never contained an account. Existing active administrators are
// authoritative and are never reconciled from process configuration.
func EnsureInitialAdmin(ctx context.Context, db *gorm.DB, mode string, input InitialAdminConfig) (err error) {
	ctx, end := telemetry.StartOperation(ctx, "auth", "initial_admin.ensure")
	defer func() { end(err) }()
	if db == nil {
		return ErrInitialAdminDatabaseUnavailable
	}

	var created *model.User
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", initialAdminAdvisoryLockID).Error; err != nil {
			return err
		}
		exists, err := platformAdminExists(tx)
		if err != nil {
			return err
		}
		if exists {
			return nil
		}

		var userCount int64
		if err := tx.Unscoped().Model(&model.User{}).Count(&userCount).Error; err != nil {
			return err
		}
		if userCount != 0 {
			return ErrInitialAdminRecoveryRequired
		}

		user, err := initialAdminUser(input)
		if err != nil {
			return err
		}
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		if err := createDefaultUserProject(tx, user); err != nil {
			return err
		}
		if err := tx.Create(&model.AuditLog{
			ID:        id.New("aud"),
			UserID:    user.ID,
			Action:    "auth.initial_admin_create",
			Resource:  user.ID,
			Success:   true,
			Message:   "",
			CreatedAt: time.Now(),
		}).Error; err != nil {
			return err
		}
		created = &user
		return nil
	})
	if err != nil {
		return err
	}
	if mode == "development" && created != nil {
		ensureDevelopmentAdminWallet(ctx, db.WithContext(ctx), *created, input.FreeQuotaCredits)
	}
	return nil
}

func initialAdminUser(input InitialAdminConfig) (model.User, error) {
	email := strings.ToLower(strings.TrimSpace(input.Email))
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email {
		return model.User{}, fmt.Errorf("%w: INITIAL_ADMIN_EMAIL must be a valid bare email address", ErrInitialAdminConfigInvalid)
	}
	if len(input.Password) < 8 || len(input.Password) > 72 {
		return model.User{}, fmt.Errorf("%w: INITIAL_ADMIN_PASSWORD must contain 8 to 72 bytes", ErrInitialAdminConfigInvalid)
	}
	language := strings.TrimSpace(input.Language)
	if language == "" {
		language = "zh-CN"
	}
	if language != "zh-CN" && language != "en-US" {
		return model.User{}, fmt.Errorf("%w: INITIAL_ADMIN_LANGUAGE must be zh-CN or en-US", ErrInitialAdminConfigInvalid)
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return model.User{}, fmt.Errorf("%w: INITIAL_ADMIN_PASSWORD cannot be hashed", ErrInitialAdminConfigInvalid)
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = email
	}
	return model.User{
		ID:       id.New("usr"),
		Email:    email,
		Name:     name,
		Role:     authz.PlatformRoleAdmin,
		Language: language,
		Password: string(passwordHash),
	}, nil
}
