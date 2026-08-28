package platformmail

import (
	"net/mail"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
)

func TestValidateRequiresCompleteSMTPSettings(t *testing.T) {
	valid := model.PlatformMailSettings{
		Host:        " smtp.example.com ",
		Port:        587,
		Security:    "STARTTLS",
		Username:    "mailer",
		PasswordRef: "secret:stored",
		FromAddress: "noreply@example.com",
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
}

func TestSMTPConfigUsesExplicitSingleRecipient(t *testing.T) {
	settings := model.PlatformMailSettings{
		Host:        "smtp.example.com",
		Port:        465,
		Security:    "tls",
		Username:    "mailer",
		PasswordRef: "secret:stored",
		FromAddress: "noreply@example.com",
		FromName:    "Luna DevOps",
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
