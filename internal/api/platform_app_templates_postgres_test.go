package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	transportapi "github.com/LiteyukiStudio/devops/internal/api/transport"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/database"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
)

func performJSONBearerRequest(t *testing.T, router http.Handler, method, path, token string, payload any) *httptest.ResponseRecorder {
	t.Helper()
	body := ""
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal %s %s payload: %v", method, path, err)
		}
		body = string(encoded)
	}
	return performBearerRequest(router, method, path, token, body)
}

func TestPlatformAppTemplatesLifecycleMarketplaceAndInstall(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("REDIS_ADDR", "")

	db := testdb.OpenDatabase(t, testdb.Options{
		SchemaPrefix: "platform_app_templates_test",
		Migrate: func(db *gorm.DB) error {
			return database.MigrateContext(context.Background(), db)
		},
	})

	admin := model.User{
		ID:       "usr_tpl_admin",
		Email:    "tpl-admin@example.com",
		Name:     "Template Admin",
		Role:     authz.PlatformRoleAdmin,
		Language: "zh-CN",
	}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatal(err)
	}
	accessToken := "pat_tpl_admin_full"
	if err := db.Create(&model.AccessToken{
		ID:        "pat_tpl_admin",
		UserID:    admin.ID,
		Name:      "Template Admin Full Scope",
		Scope:     "*",
		TokenHash: transportapi.HashToken(accessToken),
		Source:    "personal",
	}).Error; err != nil {
		t.Fatal(err)
	}

	project := model.Project{
		ID:                  "prj_tpl_suite",
		Identifier:          "tpl-suite",
		KubernetesNamespace: "luna-tpl-suite",
		Name:                "Template Suite Project",
		BillingOwnerUserID:  admin.ID,
		DeleteStatus:        "active",
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ProjectMember{
		ID:        "pm_tpl_admin",
		ProjectID: project.ID,
		UserID:    admin.ID,
		Role:      authz.ProjectRoleOwner,
	}).Error; err != nil {
		t.Fatal(err)
	}
	cluster := model.RuntimeCluster{
		ID:           "clu_tpl_suite",
		Name:         "dev-cluster",
		Scope:        "global",
		DeleteStatus: "active",
	}
	if err := db.Create(&cluster).Error; err != nil {
		t.Fatal(err)
	}

	router := NewRouter(db, mustTestConfig(t))

	// 1. Create a multi-component platform template through the full router
	createPayload := map[string]any{
		"slug":             "internal-suite",
		"name":             "Internal Suite",
		"description":      "Multi-component internal portal and API",
		"category":         "collaboration",
		"icon":             "layers",
		"version":          "1.2.0",
		"enabled":          true,
		"featured":         true,
		"popularityWeight": 88,
		"components": []map[string]any{
			{
				"key":         "web",
				"name":        "Web Portal",
				"image":       "ghcr.io/liteyuki/suite-web:1.2.0",
				"servicePort": 3000,
				"env":         map[string]string{"API_BASE": "http://{{components.api.host}}:8080"},
			},
			{
				"key":         "api",
				"name":        "Suite API",
				"image":       "ghcr.io/liteyuki/suite-api:1.2.0",
				"servicePort": 8080,
				"secretEnv":   map[string]string{"API_SECRET": "{{apiSecret}}"},
			},
		},
		"values": []map[string]any{
			{"key": "apiSecret", "label": "API Secret", "secret": true, "autoGenerate": true},
		},
	}
	rec := performJSONBearerRequest(t, router, http.MethodPost, "/api/v1/configs/app-templates", accessToken, createPayload)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create platform template = %d %s", rec.Code, rec.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	createdID, _ := created["id"].(string)
	if createdID == "" || created["isMultiComponent"] != true {
		t.Fatalf("created = %#v", created)
	}

	// 2. Marketplace ListAppTemplates & GetAppTemplate include enabled platform template
	rec = performJSONBearerRequest(t, router, http.MethodGet, "/api/v1/app-templates?query=internal-suite", accessToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list marketplace = %d %s", rec.Code, rec.Body.String())
	}
	var listed []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0]["id"] != createdID || listed[0]["source"] != "platform" {
		t.Fatalf("listed = %#v", listed)
	}
	rec = performJSONBearerRequest(t, router, http.MethodGet, "/api/v1/app-templates/internal-suite", accessToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get marketplace template by slug = %d %s", rec.Code, rec.Body.String())
	}

	// 3. Install the multi-component platform template (installNow=false so task queue is not required)
	rec = performJSONBearerRequest(t, router, http.MethodPost, "/api/v1/projects/"+project.ID+"/app-templates/"+createdID+"/install", accessToken, map[string]any{
		"applicationName":       "Portal",
		"applicationIdentifier": "portal",
		"deploymentName":        "prod",
		"stage":                 "prod",
		"clusterId":             cluster.ID,
		"installNow":            false,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("install platform template = %d %s", rec.Code, rec.Body.String())
	}
	var bindingCount int64
	if err := db.Model(&model.ServiceBinding{}).Where("project_id = ?", project.ID).Count(&bindingCount).Error; err != nil {
		t.Fatal(err)
	}
	if bindingCount != 1 {
		t.Fatalf("service bindings created = %d, want 1 (web -> api)", bindingCount)
	}

	// 4. Export & Import under a new slug, then disable original template and verify marketplace hides it
	rec = performJSONBearerRequest(t, router, http.MethodGet, "/api/v1/configs/app-templates/"+createdID+"/export", accessToken, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("export platform template = %d %s", rec.Code, rec.Body.String())
	}
	var exported map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &exported); err != nil {
		t.Fatal(err)
	}
	exported["slug"] = "internal-suite-copy"
	exported["name"] = "Internal Suite Copy"
	rec = performJSONBearerRequest(t, router, http.MethodPost, "/api/v1/configs/app-templates/import", accessToken, map[string]any{
		"overwrite": false,
		"bundle":    exported,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("import platform template = %d %s", rec.Code, rec.Body.String())
	}
	var imported map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &imported); err != nil {
		t.Fatal(err)
	}
	importedID, _ := imported["id"].(string)

	createPayload["enabled"] = false
	rec = performJSONBearerRequest(t, router, http.MethodPut, "/api/v1/configs/app-templates/"+createdID, accessToken, createPayload)
	if rec.Code != http.StatusOK {
		t.Fatalf("disable platform template = %d %s", rec.Code, rec.Body.String())
	}

	rec = performJSONBearerRequest(t, router, http.MethodGet, "/api/v1/app-templates/"+createdID, accessToken, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("disabled template GetAppTemplate status = %d, want 404", rec.Code)
	}

	// 5. Delete imported copy and verify AuditLogs
	rec = performJSONBearerRequest(t, router, http.MethodDelete, "/api/v1/configs/app-templates/"+importedID, accessToken, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete platform template = %d %s", rec.Code, rec.Body.String())
	}

	for _, action := range []string{
		"platform_app_template.create",
		"platform_app_template.export",
		"platform_app_template.import",
		"platform_app_template.disable",
		"platform_app_template.delete",
	} {
		var count int64
		if err := db.Model(&model.AuditLog{}).Where("action = ?", action).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count == 0 {
			t.Fatalf("expected audit log for action %q", action)
		}
	}
}
