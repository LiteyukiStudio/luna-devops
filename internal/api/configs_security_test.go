package api

import (
	"net/http"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
)

func TestFullConfigPayloadWithUnchangedStepUpValuesDoesNotRequireAssertion(t *testing.T) {
	db := newMFAIntegrationDB(t)
	limitMFAIntegrationConnections(t, db, 1)
	now := time.Now()
	user := model.User{ID: "usr_config_admin", Email: "config-admin@example.com", Name: "Config Admin", AuthType: "local", Role: "platform_admin", Language: "en-US"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	sessionToken := "sess_config_admin"
	if err := db.Create(&model.UserSession{ID: "ses_config_admin", UserID: user.ID, TokenHash: hashToken(sessionToken), ExpiresAt: now.Add(time.Hour)}).Error; err != nil {
		t.Fatal(err)
	}
	handlers := &Handlers{db: db, configs: newConfigCache(db), mode: "development"}
	recorder, ctx := newMFAIntegrationContext(http.MethodPut, "/api/v1/configs", map[string]any{"values": map[string]any{
		"site.title":                                "Updated Luna DevOps",
		"security.stepUpMfa.enabled":                "false",
		"security.stepUpMfa.idleTimeoutMinutes":     "10",
		"security.stepUpMfa.absoluteTimeoutMinutes": "60",
	}}, sessionToken)
	handlers.UpdateConfigs(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("full config update = %d %s", recorder.Code, recorder.Body.String())
	}
	assertAppConfigValue(t, db, "site.title", "Updated Luna DevOps")
}

func TestStepUpSecurityConfigReadsSharedDatabase(t *testing.T) {
	db := newMFAIntegrationDB(t)
	firstReplica := newConfigCache(db)
	secondReplica := newConfigCache(db)
	if configBool(firstReplica.get([]string{"security.stepUpMfa.enabled"})["security.stepUpMfa.enabled"]) {
		t.Fatal("step-up MFA should use the disabled default before the external update")
	}

	if err := upsertConfigValues(db, map[string]string{
		"security.stepUpMfa.enabled":                "true",
		"security.stepUpMfa.idleTimeoutMinutes":     "7",
		"security.stepUpMfa.absoluteTimeoutMinutes": "23",
	}); err != nil {
		t.Fatalf("update shared security config: %v", err)
	}

	values := secondReplica.get([]string{
		"security.stepUpMfa.enabled",
		"security.stepUpMfa.idleTimeoutMinutes",
		"security.stepUpMfa.absoluteTimeoutMinutes",
	})
	if values["security.stepUpMfa.enabled"] != "true" ||
		values["security.stepUpMfa.idleTimeoutMinutes"] != "7" ||
		values["security.stepUpMfa.absoluteTimeoutMinutes"] != "23" {
		t.Fatalf("second replica did not observe shared update without reload: %#v", values)
	}
}

func TestConfigBatchValidationAndTransactionAreAtomic(t *testing.T) {
	db := newMFAIntegrationDB(t)
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
