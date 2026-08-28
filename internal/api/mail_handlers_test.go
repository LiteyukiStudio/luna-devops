package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
)

func TestPlatformMailSettingsResponseDoesNotExposePassword(t *testing.T) {
	response := platformMailSettingsResponseFor(model.PlatformMailSettings{
		Host:        "smtp.example.com",
		Port:        587,
		Security:    "starttls",
		Username:    "mailer",
		PasswordRef: "secret:private-marker",
		FromAddress: "noreply@example.com",
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
}

func TestPlatformMailSettingsBlankPasswordPreservesStoredReference(t *testing.T) {
	existing := model.PlatformMailSettings{PasswordRef: "secret:stored"}
	settings, password := platformMailSettingsFromInput(existing, platformMailSettingsInput{
		Host:        "smtp.example.com",
		Port:        587,
		Security:    "starttls",
		Username:    "mailer",
		Password:    "   ",
		FromAddress: "noreply@example.com",
		FromName:    "Luna DevOps",
	})
	if password != "" {
		t.Fatalf("password = %q, want empty", password)
	}
	if settings.PasswordRef != existing.PasswordRef {
		t.Fatalf("PasswordRef = %q, want %q", settings.PasswordRef, existing.PasswordRef)
	}
}

func TestPlatformMailSettingsSeparatesNewPasswordFromPersistedModel(t *testing.T) {
	settings, password := platformMailSettingsFromInput(model.PlatformMailSettings{}, platformMailSettingsInput{
		Host:        "smtp.example.com",
		Port:        587,
		Security:    "starttls",
		Username:    "mailer",
		Password:    "plain-secret",
		FromAddress: "noreply@example.com",
	})
	if password != "plain-secret" {
		t.Fatalf("password = %q", password)
	}
	if settings.PasswordRef != "" {
		t.Fatalf("PasswordRef = %q before Secret Store write, want empty", settings.PasswordRef)
	}
}
