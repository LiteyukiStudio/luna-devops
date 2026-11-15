package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	transportapi "github.com/LiteyukiStudio/devops/internal/api/transport"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/repository"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/gin-gonic/gin"
)

func TestPlatformAppTemplatesLifecycleMarketplaceAndInstall(t *testing.T) {
	db := authIntegrationDB(t)
	now := time.Now().UTC()

	admin := model.User{ID: "usr_tpl_admin", Email: "tpl-admin@example.com", Name: "Template Admin", Role: authz.PlatformRoleAdmin, Language: "zh-CN"}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatal(err)
	}
	sessionToken := "sess_tpl_admin"
	if err := db.Create(&model.UserSession{
		ID:        "ses_tpl_admin",
		UserID:    admin.ID,
		TokenHash: transportapi.HashToken(sessionToken),
		ExpiresAt: now.Add(time.Hour),
	}).Error; err != nil {
		t.Fatal(err)
	}

	project := model.Project{
		ID:                  "prj_tpl_suite",
		Identifier:          "tpl-suite",
		KubernetesNamespace: "luna-tpl-suite",
		Name:                "Template Suite Project",
		BillingOwnerUserID:  admin.ID,
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
		Scope:        "platform",
		DeleteStatus: "active",
	}
	if err := db.Create(&cluster).Error; err != nil {
		t.Fatal(err)
	}

	codec, err := secret.NewCodec("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	handlers := &Handlers{
		db:       db,
		configs:  newConfigCache(db),
		mode:     "development",
		projects: repository.NewProjectRepository(db),
	}
	handlers.secrets = secret.NewStore(db, func(ctx context.Context, userID, action, resource string, success bool, message string) {
		handlers.auditWithContext(userID, action, resource, success, message, ctx)
	}, codec)
	handlers.domains = newDomainHandlers(handlers)

	// 1. Create a multi-component platform template
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
	rec, ctx := newAPIIntegrationContext(http.MethodPost, "/api/v1/configs/app-templates", createPayload, sessionToken)
	handlers.domains.application.CreatePlatformAppTemplate(ctx)
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

	// 2. Marketplace ListAppTemplates & GetAppTemplate & aitool include enabled platform template
	rec, ctx = newAPIIntegrationContext(http.MethodGet, "/api/v1/app-templates?query=internal-suite", nil, sessionToken)
	handlers.domains.application.ListAppTemplates(ctx)
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

	aiSvc := aitool.NewService(db)
	aiList, err := aiSvc.Execute(context.Background(), aitool.Request{
		OperationID: "listAppTemplates",
		Arguments:   map[string]any{"query": "internal-suite"},
	})
	if err != nil {
		t.Fatalf("aitool listAppTemplates: %v", err)
	}
	aiItems := aiList.Value.(map[string]any)["items"].([]map[string]any)
	if len(aiItems) != 1 || aiItems[0]["id"] != createdID {
		t.Fatalf("aiItems = %#v", aiItems)
	}
	if _, err := aiSvc.Execute(context.Background(), aitool.Request{
		OperationID: "getAppTemplate",
		Arguments:   map[string]any{"slug": "internal-suite"},
	}); err != nil {
		t.Fatalf("aitool getAppTemplate by slug: %v", err)
	}

	// 3. Install the multi-component platform template (installNow=false so task queue is not required)
	installNow := false
	rec, ctx = newAPIIntegrationContext(http.MethodPost, "/api/v1/projects/"+project.ID+"/app-templates/"+createdID+"/install", map[string]any{
		"applicationName":       "Portal",
		"applicationIdentifier": "portal",
		"deploymentName":        "prod",
		"stage":                 "production",
		"clusterId":             cluster.ID,
		"installNow":            installNow,
	}, sessionToken)
	ctx.Params = gin.Params{
		{Key: "projectId", Value: project.ID},
		{Key: "templateId", Value: createdID},
	}
	handlers.domains.application.InstallAppTemplate(ctx)
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
	rec, ctx = newAPIIntegrationContext(http.MethodGet, "/api/v1/configs/app-templates/"+createdID+"/export", nil, sessionToken)
	ctx.Params = gin.Params{{Key: "templateId", Value: createdID}}
	handlers.domains.application.ExportPlatformAppTemplate(ctx)
	if rec.Code != http.StatusOK {
		t.Fatalf("export platform template = %d %s", rec.Code, rec.Body.String())
	}
	var exported map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &exported); err != nil {
		t.Fatal(err)
	}
	exported["slug"] = "internal-suite-copy"
	exported["name"] = "Internal Suite Copy"
	rec, ctx = newAPIIntegrationContext(http.MethodPost, "/api/v1/configs/app-templates/import", map[string]any{
		"overwrite": false,
		"bundle":    exported,
	}, sessionToken)
	handlers.domains.application.ImportPlatformAppTemplate(ctx)
	if rec.Code != http.StatusCreated {
		t.Fatalf("import platform template = %d %s", rec.Code, rec.Body.String())
	}
	var imported map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &imported); err != nil {
		t.Fatal(err)
	}
	importedID, _ := imported["id"].(string)

	createPayload["enabled"] = false
	rec, ctx = newAPIIntegrationContext(http.MethodPut, "/api/v1/configs/app-templates/"+createdID, createPayload, sessionToken)
	ctx.Params = gin.Params{{Key: "templateId", Value: createdID}}
	handlers.domains.application.UpdatePlatformAppTemplate(ctx)
	if rec.Code != http.StatusOK {
		t.Fatalf("disable platform template = %d %s", rec.Code, rec.Body.String())
	}

	rec, ctx = newAPIIntegrationContext(http.MethodGet, "/api/v1/app-templates/"+createdID, nil, sessionToken)
	ctx.Params = gin.Params{{Key: "templateId", Value: createdID}}
	handlers.domains.application.GetAppTemplate(ctx)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("disabled template GetAppTemplate status = %d, want 404", rec.Code)
	}

	// 5. Delete imported copy and verify AuditLogs
	rec, ctx = newAPIIntegrationContext(http.MethodDelete, "/api/v1/configs/app-templates/"+importedID, nil, sessionToken)
	ctx.Params = gin.Params{{Key: "templateId", Value: importedID}}
	handlers.domains.application.DeletePlatformAppTemplate(ctx)
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
			t.Fatalf("expected audit log for %s", action)
		}
	}
}
