package platformmail

import (
	"net/mail"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
)

func TestDefaultSettingsUseOneMinutePersonalEmailCooldown(t *testing.T) {
	settings := DefaultSettings()
	if settings.PersonalEmailCooldownSeconds != DefaultPersonalEmailCooldownSeconds {
		t.Fatalf("personal email cooldown = %d, want %d", settings.PersonalEmailCooldownSeconds, DefaultPersonalEmailCooldownSeconds)
	}

	settings.PersonalEmailCooldownSeconds = 0
	if normalized := Normalize(settings); normalized.PersonalEmailCooldownSeconds != 0 {
		t.Fatalf("Normalize() changed disabled cooldown to %d", normalized.PersonalEmailCooldownSeconds)
	}
}

func TestValidateRequiresCompleteSMTPSettings(t *testing.T) {
	valid := model.PlatformMailSettings{
		Host:                         " smtp.example.com ",
		Port:                         587,
		Security:                     "STARTTLS",
		Username:                     "mailer",
		PasswordRef:                  "secret:stored",
		FromAddress:                  "noreply@example.com",
		PersonalEmailCooldownSeconds: DefaultPersonalEmailCooldownSeconds,
	}
	if err := Validate(valid, false); err != nil {
		t.Fatalf("Validate() rejected valid settings: %v", err)
	}

	missingPassword := valid
	missingPassword.PasswordRef = ""
	if err := Validate(missingPassword, false); err == nil {
		t.Fatal("Validate() accepted authenticated SMTP without a password")
	}
	if err := Validate(missingPassword, true); err != nil {
		t.Fatalf("Validate() rejected a password supplied with the update: %v", err)
	}

	invalidSender := valid
	invalidSender.FromAddress = "Luna <noreply@example.com>"
	if err := Validate(invalidSender, false); err == nil {
		t.Fatal("Validate() accepted a display name in fromAddress")
	}

	disabledCooldown := valid
	disabledCooldown.PersonalEmailCooldownSeconds = 0
	if err := Validate(disabledCooldown, false); err != nil {
		t.Fatalf("Validate() rejected a disabled personal email cooldown: %v", err)
	}
	for _, seconds := range []int{-1, MaxPersonalEmailCooldownSeconds + 1} {
		invalidCooldown := valid
		invalidCooldown.PersonalEmailCooldownSeconds = seconds
		if err := Validate(invalidCooldown, false); err == nil {
			t.Fatalf("Validate() accepted personal email cooldown %d", seconds)
		}
	}
}

func TestPersonalEmailAggregationCooldownReadsSavedSetting(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "platform_mail_cooldown_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.PlatformMailSettings{})
		},
	})

	cooldown, err := PersonalEmailAggregationCooldown(t.Context(), db)
	if err != nil {
		t.Fatalf("read default personal email cooldown: %v", err)
	}
	if cooldown != time.Minute {
		t.Fatalf("default cooldown = %s, want %s", cooldown, time.Minute)
	}
	if err := db.Model(&model.PlatformMailSettings{}).
		Where("id = ?", SettingsID).
		Update("personal_email_cooldown_seconds", 0).Error; err != nil {
		t.Fatalf("disable personal email cooldown: %v", err)
	}
	cooldown, err = PersonalEmailAggregationCooldown(t.Context(), db)
	if err != nil {
		t.Fatalf("read disabled personal email cooldown: %v", err)
	}
	if cooldown != 0 {
		t.Fatalf("disabled cooldown = %s, want 0", cooldown)
	}
}

func TestSMTPConfigUsesExplicitSingleRecipient(t *testing.T) {
	settings := model.PlatformMailSettings{
		Host:                         "smtp.example.com",
		Port:                         465,
		Security:                     "tls",
		Username:                     "mailer",
		PasswordRef:                  "secret:stored",
		FromAddress:                  "noreply@example.com",
		FromName:                     "Luna DevOps",
		PersonalEmailCooldownSeconds: DefaultPersonalEmailCooldownSeconds,
	}
	cfg, err := smtpConfig(settings, "operator@example.com")
	if err != nil {
		t.Fatalf("smtpConfig() error = %v", err)
	}
	if len(cfg.To) != 1 || cfg.To[0] != "operator@example.com" || len(cfg.Cc) != 0 || len(cfg.Bcc) != 0 {
		t.Fatalf("SMTP recipients = to:%v cc:%v bcc:%v", cfg.To, cfg.Cc, cfg.Bcc)
	}
	from, err := mail.ParseAddress(cfg.From)
	if err != nil || from.Name != settings.FromName || from.Address != settings.FromAddress || cfg.SecretRef != settings.PasswordRef {
		t.Fatalf("SMTP config = %#v", cfg)
	}
}

func TestSMTPConfigRejectsDisplayNameRecipient(t *testing.T) {
	if _, err := smtpConfig(DefaultSettings(), "Operator <operator@example.com>"); err == nil {
		t.Fatal("smtpConfig() accepted a recipient display name")
	}
}
