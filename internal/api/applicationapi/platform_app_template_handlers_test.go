package applicationapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/appstore"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type platformTemplateUnitHost struct {
	Host
	user   model.User
	hasUsr bool
	audits []string
}

func (host *platformTemplateUnitHost) SecretStore() secret.Store   { return secret.Store{} }
func (host *platformTemplateUnitHost) DBFor(*gin.Context) *gorm.DB { return nil }
func (host *platformTemplateUnitHost) CurrentUser(*gin.Context) (model.User, bool) {
	return host.user, host.hasUsr
}
func (host *platformTemplateUnitHost) AuditWithContext(_, action, resource string, _ bool, _ string, _ context.Context) {
	host.audits = append(host.audits, action+":"+resource)
}

func TestPlatformAppTemplateHandlersEnforceAdminAndValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 1. Non-admin is rejected with 403
	nonAdminHost := &platformTemplateUnitHost{
		user:   model.User{ID: "usr_member", Role: authz.PlatformRoleUser},
		hasUsr: true,
	}
	nonAdminHandler := New(nonAdminHost)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/configs/app-templates", nil)
	nonAdminHandler.ListPlatformAppTemplates(ctx)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin ListPlatformAppTemplates status = %d, want 403", rec.Code)
	}

	// 2. Admin rejects built-in slug collision (409) and plaintext secret default (400)
	adminHost := &platformTemplateUnitHost{
		user:   model.User{ID: "usr_admin", Role: authz.PlatformRoleAdmin},
		hasUsr: true,
	}
	adminHandler := New(adminHost)

	reservedBody, _ := json.Marshal(map[string]any{
		"slug":        "postgresql",
		"name":        "Custom PG",
		"category":    "database",
		"image":       "postgres:17",
		"servicePort": 5432,
	})
	rec = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/configs/app-templates", bytes.NewReader(reservedBody))
	ctx.Request.Header.Set("Content-Type", "application/json")
	adminHandler.CreatePlatformAppTemplate(ctx)
	if rec.Code != http.StatusConflict {
		t.Fatalf("reserved slug status = %d body=%s, want 409", rec.Code, rec.Body.String())
	}

	secretDefaultBody, _ := json.Marshal(map[string]any{
		"slug":        "custom-db",
		"name":        "Custom DB",
		"category":    "database",
		"image":       "postgres:17",
		"servicePort": 5432,
		"secretEnv":   map[string]string{"POSTGRES_PASSWORD": "{{pw}}"},
		"values": []map[string]any{
			{"key": "pw", "label": "Password", "secret": true, "default": "leaked"},
		},
	})
	rec = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/configs/app-templates", bytes.NewReader(secretDefaultBody))
	ctx.Request.Header.Set("Content-Type", "application/json")
	adminHandler.CreatePlatformAppTemplate(ctx)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("secret default status = %d body=%s, want 400", rec.Code, rec.Body.String())
	}

	// 3. Export built-in template strips secret defaults and records audit
	rec = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/configs/app-templates/mongo-express/export", nil)
	ctx.Params = gin.Params{{Key: "templateId", Value: "mongo-express"}}
	adminHandler.ExportPlatformAppTemplate(ctx)
	if rec.Code != http.StatusOK {
		t.Fatalf("export built-in status = %d body=%s, want 200", rec.Code, rec.Body.String())
	}
	var bundle appstore.PlatformTemplateBundle
	if err := json.Unmarshal(rec.Body.Bytes(), &bundle); err != nil {
		t.Fatalf("decode bundle: %v", err)
	}
	if bundle.SchemaVersion != appstore.PlatformTemplateBundleSchemaVersion || bundle.Slug != "mongo-express" {
		t.Fatalf("bundle = %#v", bundle)
	}
	for _, val := range bundle.Values {
		if val.Secret && val.Default != "" {
			t.Fatalf("exported bundle leaked secret default for %s", val.Key)
		}
	}
	if len(adminHost.audits) != 1 || adminHost.audits[0] != "platform_app_template.export:mongo-express" {
		t.Fatalf("audits = %v", adminHost.audits)
	}
}
