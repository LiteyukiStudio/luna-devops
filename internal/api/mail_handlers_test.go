package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin/binding"
)

func TestPlatformMailSettingsResponseDoesNotExposePassword(t *testing.T) {
	response := platformMailSettingsResponseFor(model.PlatformMailSettings{
		Host:                         "smtp.example.com",
		Port:                         587,
		Security:                     "starttls",
		Username:                     "mailer",
		PasswordRef:                  "secret:private-marker",
		FromAddress:                  "noreply@example.com",
		PersonalEmailCooldownSeconds: 120,
	})
	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	if strings.Contains(string(data), "private-marker") || strings.Contains(string(data), "passwordRef") || strings.Contains(string(data), `"password"`) {
		t.Fatalf("mail settings response exposed a password value or reference: %s", data)
	}
	if !response.PasswordSet {
		t.Fatal("passwordSet = false, want true")
	}
	if response.PersonalEmailCooldownSeconds != 120 {
		t.Fatalf("personalEmailCooldownSeconds = %d, want 120", response.PersonalEmailCooldownSeconds)
	}
}

func TestPlatformMailSettingsBlankPasswordPreservesStoredReference(t *testing.T) {
	existing := model.PlatformMailSettings{PasswordRef: "secret:stored"}
	cooldown := 0
	settings, password := platformMailSettingsFromInput(existing, platformMailSettingsInput{
		Host:                         "smtp.example.com",
		Port:                         587,
		Security:                     "starttls",
		Username:                     "mailer",
		Password:                     "   ",
		FromAddress:                  "noreply@example.com",
		FromName:                     "Luna DevOps",
		PersonalEmailCooldownSeconds: &cooldown,
	})
	if password != "" {
		t.Fatalf("password = %q, want empty", password)
	}
	if settings.PasswordRef != existing.PasswordRef {
		t.Fatalf("PasswordRef = %q, want %q", settings.PasswordRef, existing.PasswordRef)
	}
	if settings.PersonalEmailCooldownSeconds != 0 {
		t.Fatalf("PersonalEmailCooldownSeconds = %d, want 0", settings.PersonalEmailCooldownSeconds)
	}
}

func TestPlatformMailSettingsSeparatesNewPasswordFromPersistedModel(t *testing.T) {
	cooldown := 60
	settings, password := platformMailSettingsFromInput(model.PlatformMailSettings{}, platformMailSettingsInput{
		Host:                         "smtp.example.com",
		Port:                         587,
		Security:                     "starttls",
		Username:                     "mailer",
		Password:                     "plain-secret",
		FromAddress:                  "noreply@example.com",
		PersonalEmailCooldownSeconds: &cooldown,
	})
	if password != "plain-secret" {
		t.Fatalf("password = %q", password)
	}
	if settings.PasswordRef != "" {
		t.Fatalf("PasswordRef = %q before Secret Store write, want empty", settings.PasswordRef)
	}
}

func TestPlatformMailSettingsInputRequiresCooldownWhileAllowingZero(t *testing.T) {
	if err := binding.Validator.ValidateStruct(platformMailSettingsInput{}); err == nil {
		t.Fatal("mail settings input accepted a missing personalEmailCooldownSeconds")
	}
	zero := 0
	if err := binding.Validator.ValidateStruct(platformMailSettingsInput{PersonalEmailCooldownSeconds: &zero}); err != nil {
		t.Fatalf("mail settings input rejected an explicit zero cooldown: %v", err)
	}
}
