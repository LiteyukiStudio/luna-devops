package gitapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type webhookBuildTestHost struct {
	Host
	db          *gorm.DB
	allowed     bool
	billingCode string
	prepared    int
	queued      int
}

func (h *webhookBuildTestHost) DBFor(*gin.Context) *gorm.DB { return h.db }
func (h *webhookBuildTestHost) BuildRunActorName(user model.User) string {
	return user.Name
}
func (h *webhookBuildTestHost) DeploymentTargetMatchesBuildRun(model.DeploymentTarget, model.BuildRun) bool {
	return true
}
func (h *webhookBuildTestHost) ProjectRoleActionAllowed(_ context.Context, _ model.User, _ string, action authz.Action) (bool, error) {
	if action != authz.ActionBuildTrigger {
		return false, nil
	}
	return h.allowed, nil
}
func (h *webhookBuildTestHost) NewBuildBillingDenialCode(context.Context, string) (string, error) {
	return h.billingCode, nil
}
func (h *webhookBuildTestHost) PrepareBuildRunRequest(model.User, *model.BuildRun, context.Context) error {
	h.prepared++
	return nil
}
func (h *webhookBuildTestHost) QueueBuildRun(_ context.Context, _ model.User, run model.BuildRun) (model.BuildRun, error) {
	h.queued++
	return run, h.db.Create(&run).Error
}

func openWebhookBuildDB(t *testing.T, projectDeleteStatus string) (*gorm.DB, model.RepositoryBinding) {
	t.Helper()
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "webhook_build_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.User{}, &model.Project{}, &model.GitAccount{}, &model.DeploymentTarget{}, &model.BuildRun{}, &model.PlatformEvent{})
		},
	})
	binding := model.RepositoryBinding{ID: "rpb_hook", ProjectID: "prj_hook", ApplicationID: "app_hook", GitAccountID: "gac_hook"}
	for _, value := range []any{
		&model.User{ID: "usr_hook", Email: "hook@example.test", Name: "Hook Owner"},
		&model.Project{ID: binding.ProjectID, Name: "Hook", Identifier: "hook", DeleteStatus: projectDeleteStatus},
		&model.GitAccount{ID: binding.GitAccountID, UserID: "usr_hook", ProviderID: "gpr_hook", Username: "hook"},
		&model.DeploymentTarget{
			ID: "dplt_hook", ProjectID: binding.ProjectID, ApplicationID: binding.ApplicationID, Name: "Dev", Stage: "dev",
			KubernetesName: "hook-dev", Enabled: true, RepositoryBindingID: binding.ID, SourceType: "repository", DeleteStatus: "active",
		},
	} {
		if err := db.Create(value).Error; err != nil {
			t.Fatalf("seed %T: %v", value, err)
		}
	}
	return db, binding
}

func runWebhookBuild(handler *Handler, binding model.RepositoryBinding, deliveryID string) webhookBuildTriggerResult {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/git/webhooks/"+binding.ID, nil)
	return handler.enqueueBuildRunsForWebhook(ctx, binding, gitWebhookPushPayload{SourceBranch: "main", CommitSHA: "abc123"}, true, deliveryID)
}

func webhookRejectionCodes(t *testing.T, db *gorm.DB) []string {
	t.Helper()
	var events []model.PlatformEvent
	if err := db.Where("type = ?", webhookRejectedEventType).Find(&events).Error; err != nil {
		t.Fatalf("read webhook rejection events: %v", err)
	}
	codes := make([]string, 0, len(events))
	for _, event := range events {
		if event.Status != "failed" || event.DeploymentTargetID != "dplt_hook" || event.ActorID != "usr_hook" {
			t.Fatalf("rejection event = %+v", event)
		}
		codes = append(codes, event.Message)
	}
	return codes
}

func TestWebhookBuildRejectsWithManualTriggerGuards(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name          string
		projectStatus string
		allowed       bool
		billingCode   string
		wantCode      string
	}{
		{name: "actor lost build permission", projectStatus: "active", allowed: false, wantCode: webhookRejectActorForbidden},
		{name: "project deleting", projectStatus: "deleting", allowed: true, wantCode: webhookRejectProjectDeleting},
		{name: "billing blocked", projectStatus: "active", allowed: true, billingCode: "billing.insufficient_balance", wantCode: "billing.insufficient_balance"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, binding := openWebhookBuildDB(t, tc.projectStatus)
			host := &webhookBuildTestHost{db: db, allowed: tc.allowed, billingCode: tc.billingCode}
			result := runWebhookBuild(&Handler{host: host}, binding, "delivery-0001")
			if result.Matched != 1 || result.Failed != 1 || result.Queued != 0 || host.prepared != 0 || host.queued != 0 {
				t.Fatalf("result = %+v prepared=%d queued=%d", result, host.prepared, host.queued)
			}
			if codes := webhookRejectionCodes(t, db); len(codes) != 1 || codes[0] != tc.wantCode {
				t.Fatalf("rejection codes = %v, want [%s]", codes, tc.wantCode)
			}
		})
	}
}

func TestWebhookBuildDropsRedeliveredEvents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, binding := openWebhookBuildDB(t, "active")
	host := &webhookBuildTestHost{db: db, allowed: true}
	handler := &Handler{host: host}

	first := runWebhookBuild(handler, binding, "delivery-0002")
	if first.Queued != 1 || first.Duplicate != 0 {
		t.Fatalf("first delivery = %+v", first)
	}
	second := runWebhookBuild(handler, binding, "delivery-0002")
	if second.Queued != 0 || second.Duplicate != 1 || second.IgnoredReason != "duplicate_delivery" || host.queued != 1 {
		t.Fatalf("redelivery = %+v queued=%d", second, host.queued)
	}
	third := runWebhookBuild(handler, binding, "delivery-0003")
	if third.Queued != 1 || host.queued != 2 {
		t.Fatalf("new delivery = %+v queued=%d", third, host.queued)
	}
	var run model.BuildRun
	if err := db.First(&run, "idempotency_key = ?", webhookBuildIdempotencyKey(binding.ID, "delivery-0002", "dplt_hook")).Error; err != nil || run.CreatedBy != "usr_hook" {
		t.Fatalf("idempotent webhook run = %+v, %v", run, err)
	}
}

func TestGitWebhookDeliveryIDRejectsMalformedValues(t *testing.T) {
	header := http.Header{}
	header.Set("X-Gitea-Delivery", "9f0c5c1e-1111-4222-8333-444455556666")
	if got := gitWebhookDeliveryID(header); got != "9f0c5c1e-1111-4222-8333-444455556666" {
		t.Fatalf("delivery id = %q", got)
	}
	header.Set("X-Gitea-Delivery", "short")
	if got := gitWebhookDeliveryID(header); got != "" {
		t.Fatalf("malformed delivery id = %q, want empty", got)
	}
}
