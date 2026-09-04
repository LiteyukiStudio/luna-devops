package api

import (
	"encoding/json"
	"github.com/LiteyukiStudio/devops/internal/api/notificationapi"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func TestNotificationRuleInputRequiresExplicitExistingProjectScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "notification_rule_scope_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.Project{}, &model.NotificationChannel{})
		},
	})
	project := model.Project{ID: "prj_notification_scope", Identifier: "notification-scope", Name: "Notification scope"}
	channel := model.NotificationChannel{ID: "nch_notification_scope", Name: "Shared", AdapterKind: notification.AdapterKindWebhook, ConfigJSON: `{}`, SecretRefsJSON: `{}`, Enabled: true}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}

	tests := []struct {
		name     string
		input    notificationapi.NotificationRuleInput
		wantOK   bool
		wantCode string
	}{
		{
			name:   "selected project",
			input:  notificationapi.NotificationRuleInput{Name: "Project failures", EventTypes: []string{"build.failed", "build.failed"}, Filter: json.RawMessage(`{"scope":"projects","projectIds":["prj_notification_scope"]}`), ChannelIDs: []string{channel.ID, channel.ID}},
			wantOK: true,
		},
		{
			name:   "explicit all",
			input:  notificationapi.NotificationRuleInput{Name: "All failures", EventTypes: []string{"build.failed"}, Filter: json.RawMessage(`{"scope":"all"}`), ChannelIDs: []string{channel.ID}},
			wantOK: true,
		},
		{
			name:     "legacy empty filter",
			input:    notificationapi.NotificationRuleInput{Name: "Legacy", EventTypes: []string{"build.failed"}, Filter: json.RawMessage(`{}`), ChannelIDs: []string{channel.ID}},
			wantCode: "notification.rule_filter_invalid",
		},
		{
			name:     "unknown filter field",
			input:    notificationapi.NotificationRuleInput{Name: "Unknown", EventTypes: []string{"build.failed"}, Filter: json.RawMessage(`{"scope":"all","unknown":true}`), ChannelIDs: []string{channel.ID}},
			wantCode: "notification.rule_filter_invalid",
		},
		{
			name:     "missing project",
			input:    notificationapi.NotificationRuleInput{Name: "Missing", EventTypes: []string{"build.failed"}, Filter: json.RawMessage(`{"scope":"projects","projectIds":["prj_missing"]}`), ChannelIDs: []string{channel.ID}},
			wantCode: "notification.rule_project_not_found",
		},
		{
			name:     "empty event types",
			input:    notificationapi.NotificationRuleInput{Name: "No events", Filter: json.RawMessage(`{"scope":"all"}`), ChannelIDs: []string{channel.ID}},
			wantCode: "notification.rule_required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/notifications/rules", nil)
			rule, ok := notificationapi.New(notificationHost{domainHost: domainHost{handlers: &Handlers{db: db}}}).NotificationRuleFromInput(ctx, tt.input, model.NotificationRule{ID: "nrl_test"})
			if ok != tt.wantOK {
				t.Fatalf("ok = %t, status=%d body=%s", ok, recorder.Code, recorder.Body.String())
			}
			if tt.wantOK {
				if recorder.Code != http.StatusOK || rule.FilterJSON == "" {
					t.Fatalf("valid rule status=%d rule=%#v", recorder.Code, rule)
				}
				return
			}
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, body=%s", recorder.Code, recorder.Body.String())
			}
			var response map[string]any
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if response["code"] != tt.wantCode {
				t.Fatalf("error code = %#v, want %q", response["code"], tt.wantCode)
			}
		})
	}
}
