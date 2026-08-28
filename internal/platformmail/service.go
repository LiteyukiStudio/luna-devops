package platformmail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
	"gorm.io/gorm"
)

const (
	SettingsID                          = "default"
	DefaultPersonalEmailCooldownSeconds = 60
	MaxPersonalEmailCooldownSeconds     = 3600
)

var (
	ErrInvalidSettings  = errors.New("platform mail settings are invalid")
	ErrInvalidRecipient = errors.New("platform mail recipient is invalid")
)

func DefaultSettings() model.PlatformMailSettings {
	return model.PlatformMailSettings{
		ID:                           SettingsID,
		Port:                         587,
		Security:                     "starttls",
		FromName:                     "Luna DevOps",
		PersonalEmailCooldownSeconds: DefaultPersonalEmailCooldownSeconds,
	}
}

func Normalize(settings model.PlatformMailSettings) model.PlatformMailSettings {
	settings.ID = SettingsID
	settings.Host = strings.TrimSpace(settings.Host)
	settings.Security = strings.ToLower(strings.TrimSpace(settings.Security))
	settings.Username = strings.TrimSpace(settings.Username)
	settings.PasswordRef = strings.TrimSpace(settings.PasswordRef)
	settings.FromAddress = strings.TrimSpace(settings.FromAddress)
	settings.FromName = strings.TrimSpace(settings.FromName)
	if settings.Port == 0 {
		settings.Port = 587
	}
	if settings.Security == "" {
		settings.Security = "starttls"
	}
	if settings.FromName == "" {
		settings.FromName = "Luna DevOps"
	}
	return settings
}

func Validate(settings model.PlatformMailSettings, passwordProvided bool) error {
	settings = Normalize(settings)
	if settings.Host == "" {
		return errors.New("SMTP host is required")
	}
	if settings.Port < 1 || settings.Port > 65535 {
		return errors.New("SMTP port must be between 1 and 65535")
	}
	switch settings.Security {
	case "none", "starttls", "tls":
	default:
		return errors.New("SMTP security must be none, starttls, or tls")
	}
	if settings.FromAddress == "" {
		return errors.New("SMTP sender address is required")
	}
	address, err := mail.ParseAddress(settings.FromAddress)
	if err != nil || !strings.EqualFold(address.Address, settings.FromAddress) {
		return errors.New("SMTP sender address is invalid")
	}
	if settings.Username != "" && settings.PasswordRef == "" && !passwordProvided {
		return errors.New("SMTP password is required when username is set")
	}
	if err := validatePersonalEmailCooldownSeconds(settings.PersonalEmailCooldownSeconds); err != nil {
		return err
	}
	return nil
}

func PersonalEmailAggregationCooldown(ctx context.Context, db *gorm.DB) (time.Duration, error) {
	settings, err := Get(ctx, db)
	if err != nil {
		return 0, err
	}
	if err := validatePersonalEmailCooldownSeconds(settings.PersonalEmailCooldownSeconds); err != nil {
		return 0, err
	}
	return time.Duration(settings.PersonalEmailCooldownSeconds) * time.Second, nil
}

func validatePersonalEmailCooldownSeconds(seconds int) error {
	if seconds < 0 || seconds > MaxPersonalEmailCooldownSeconds {
		return fmt.Errorf("personal email cooldown must be between 0 and %d seconds", MaxPersonalEmailCooldownSeconds)
	}
	return nil
}

func Get(ctx context.Context, db *gorm.DB) (model.PlatformMailSettings, error) {
	if db == nil {
		return model.PlatformMailSettings{}, errors.New("platform mail database is required")
	}
	settings := DefaultSettings()
	if err := db.WithContext(ctx).FirstOrCreate(&settings, "id = ?", SettingsID).Error; err != nil {
		return model.PlatformMailSettings{}, err
	}
	return Normalize(settings), nil
}

func Save(ctx context.Context, db *gorm.DB, settings model.PlatformMailSettings) (model.PlatformMailSettings, error) {
	if db == nil {
		return model.PlatformMailSettings{}, errors.New("platform mail database is required")
	}
	settings = Normalize(settings)
	if err := Validate(settings, false); err != nil {
		return model.PlatformMailSettings{}, err
	}
	if err := db.WithContext(ctx).Save(&settings).Error; err != nil {
		return model.PlatformMailSettings{}, err
	}
	return settings, nil
}

func Send(
	ctx context.Context,
	db *gorm.DB,
	secretResolver notification.SecretResolver,
	recipient string,
	message notification.RenderedMessage,
) (notification.SendResult, error) {
	settings, err := Get(ctx, db)
	if err != nil {
		return notification.SendResult{}, err
	}
	if err := Validate(settings, false); err != nil {
		return notification.SendResult{}, fmt.Errorf("%w: %v", ErrInvalidSettings, err)
	}
	cfg, err := smtpConfig(settings, recipient)
	if err != nil {
		return notification.SendResult{}, fmt.Errorf("%w: %v", ErrInvalidRecipient, err)
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return notification.SendResult{}, err
	}
	return (notification.SMTPAdapter{}).Send(ctx, raw, nil, message, secretResolver)
}

func smtpConfig(settings model.PlatformMailSettings, recipient string) (notification.SMTPConfig, error) {
	settings = Normalize(settings)
	recipient = strings.TrimSpace(recipient)
	address, err := mail.ParseAddress(recipient)
	if err != nil || !strings.EqualFold(address.Address, recipient) {
		return notification.SMTPConfig{}, errors.New("mail recipient is invalid")
	}
	from := settings.FromAddress
	if settings.FromName != "" {
		from = (&mail.Address{Name: settings.FromName, Address: settings.FromAddress}).String()
	}
	return notification.SMTPConfig{
		Host:      settings.Host,
		Port:      settings.Port,
		Security:  settings.Security,
		Username:  settings.Username,
		From:      from,
		To:        []string{recipient},
		Timeout:   15,
		SecretRef: settings.PasswordRef,
	}, nil
}
