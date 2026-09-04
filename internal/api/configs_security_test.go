package api

import (
	transportapi "github.com/LiteyukiStudio/devops/internal/api/transport"
	"net/http"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
)

func TestFullConfigPayloadUpdatesKnownValues(t *testing.T) {
	db := authIntegrationDB(t)
	now := time.Now()
	user := model.User{ID: "usr_config_admin", Email: "config-admin@example.com", Name: "Config Admin", Role: authz.PlatformRoleAdmin, Language: "en-US"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	sessionToken := "sess_config_admin"
	if err := db.Create(&model.UserSession{ID: "ses_config_admin", UserID: user.ID, TokenHash: transportapi.HashToken(sessionToken), ExpiresAt: now.Add(time.Hour)}).Error; err != nil {
		t.Fatal(err)
	}
	handlers := &Handlers{db: db, configs: newConfigCache(db), mode: "development"}
	recorder, ctx := newAPIIntegrationContext(http.MethodPut, "/api/v1/configs", map[string]any{"values": map[string]any{
		"site.title": "Updated Luna DevOps",
	}}, sessionToken)
	handlers.UpdateConfigs(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("full config update = %d %s", recorder.Code, recorder.Body.String())
	}
	assertAppConfigValue(t, db, "site.title", "Updated Luna DevOps")
}

func TestAIConfigUpdatePersistsTransactionAudit(t *testing.T) {
	db := authIntegrationDB(t)
	now := time.Now()
	user := model.User{ID: "usr_ai_config_admin", Email: "ai-config-admin@example.com", Name: "AI Config Admin", Role: authz.PlatformRoleAdmin, Language: "en-US"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	sessionToken := "sess_ai_config_admin"
	if err := db.Create(&model.UserSession{ID: "ses_ai_config_admin", UserID: user.ID, TokenHash: transportapi.HashToken(sessionToken), ExpiresAt: now.Add(time.Hour)}).Error; err != nil {
		t.Fatal(err)
	}
	handlers := &Handlers{db: db, configs: newConfigCache(db), mode: "development"}
	recorder, ctx := newAPIIntegrationContext(http.MethodPut, "/api/v1/configs", map[string]any{"values": map[string]any{
		"ai.assistant.enabled": true,
	}}, sessionToken)

	handlers.UpdateConfigs(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("AI config update = %d %s", recorder.Code, recorder.Body.String())
	}
	assertAppConfigValue(t, db, "ai.assistant.enabled", "true")
	var audit model.AuditLog
	if err := db.First(&audit, "action = ?", "ai.settings_update").Error; err != nil {
		t.Fatalf("load AI settings audit: %v", err)
	}
	if audit.ID == "" || audit.UserID != user.ID || audit.Resource != "ai.settings" || !audit.Success {
		t.Fatalf("unexpected AI settings audit: %+v", audit)
	}
}

func TestConfigBatchValidationAndTransactionAreAtomic(t *testing.T) {
	db := authIntegrationDB(t)
	if err := db.Create(&model.AppConfig{Key: "site.title", Value: "before"}).Error; err != nil {
		t.Fatalf("seed config: %v", err)
	}

	if _, err := validateConfigValues(map[string]any{
		"site.title":  "after-validation",
		"unknown.key": "rejected",
	}); err == nil {
		t.Fatal("expected the complete batch to be rejected before writing")
	}
	assertAppConfigValue(t, db, "site.title", "before")

	if err := db.Exec(`
		CREATE FUNCTION reject_test_config() RETURNS trigger AS $$
		BEGIN
			IF NEW.key = 'site.logoUrl' THEN
				RAISE EXCEPTION 'forced config write failure';
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql
	`).Error; err != nil {
		t.Fatalf("create failure function: %v", err)
	}
	if err := db.Exec(`
		CREATE TRIGGER reject_test_config_write
		BEFORE INSERT OR UPDATE ON app_configs
		FOR EACH ROW EXECUTE FUNCTION reject_test_config()
	`).Error; err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}

	err := upsertConfigValues(db, map[string]string{
		"site.title":   "after-transaction",
		"site.logoUrl": "https://example.com/logo.png",
	})
	if err == nil {
		t.Fatal("expected the config transaction to fail")
	}
	assertAppConfigValue(t, db, "site.title", "before")
}

func assertAppConfigValue(t *testing.T, db *gorm.DB, key, expected string) {
	t.Helper()
	var row model.AppConfig
	if err := db.First(&row, "key = ?", key).Error; err != nil {
		t.Fatalf("load config %s: %v", key, err)
	}
	if row.Value != expected {
		t.Fatalf("config %s = %q, want %q", key, row.Value, expected)
	}
}
