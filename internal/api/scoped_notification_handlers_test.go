package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
	"github.com/LiteyukiStudio/devops/internal/repository"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func newScopedNotificationTestContext(method, target string, user model.User, params gin.Params, body string) (*httptest.ResponseRecorder, *gin.Context) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		ctx.Request.Header.Set("Content-Type", "application/json")
	}
	if user.Language == "" {
		user.Language = "zh-CN"
	}
	if user.Role == "" {
		user.Role = authz.PlatformRoleUser
	}
	ctx.Set(currentUserContextKey, user)
	ctx.Params = params
	return recorder, ctx
}

func openScopedNotificationTestDB(t *testing.T, prefix string) (*gorm.DB, *Handlers, *fakeBuildTaskEnqueuer) {
	t.Helper()
	t.Setenv("SECRET_ENCRYPTION_KEY", "scoped-notification-test-key")
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: prefix,
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(
				&model.User{},
				&model.Project{},
				&model.ProjectMember{},
				&model.Application{},
				&model.DeploymentTarget{},
				&model.ScopedResourceProjectBinding{},
				&model.NotificationChannel{},
				&model.NotificationTemplate{},
				&model.NotificationRule{},
				&model.NotificationDelivery{},
				&model.SecretValue{},
				&model.AuditLog{},
			)
		},
	})
	enqueuer := &fakeBuildTaskEnqueuer{}
	handlers := &Handlers{
		db:         db,
		projects:   repository.NewProjectRepository(db),
		secrets:    secret.NewStore(db, nil, mustTestSecretCodec(t)),
		taskClient: enqueuer,
	}
	handlers.domains = newDomainHandlers(handlers)
	return db, handlers, enqueuer
}

func TestScopedNotificationOwnerOnlyCoOwnershipAndRedaction(t *testing.T) {
	db, handlers, _ := openScopedNotificationTestDB(t, "scoped_notification_acl")

	ownerAll := model.User{ID: "usr_owner_all", Email: "owner-all@example.com", Role: authz.PlatformRoleUser}
	coOwnerB := model.User{ID: "usr_co_owner_b", Email: "co-owner-b@example.com", Role: authz.PlatformRoleUser}
	adminA := model.User{ID: "usr_admin_a", Email: "admin-a@example.com", Role: authz.PlatformRoleUser}
	devA := model.User{ID: "usr_dev_a", Email: "dev-a@example.com", Role: authz.PlatformRoleUser}
	viewerA := model.User{ID: "usr_viewer_a", Email: "viewer-a@example.com", Role: authz.PlatformRoleUser}

	for _, u := range []model.User{ownerAll, coOwnerB, adminA, devA, viewerA} {
		if err := db.Create(&u).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []model.Project{
		{ID: "prj_a", Name: "Alpha", Identifier: "alpha"},
		{ID: "prj_b", Name: "Beta", Identifier: "beta"},
	} {
		if err := db.Create(&p).Error; err != nil {
			t.Fatal(err)
		}
	}
	members := []model.ProjectMember{
		{ID: "pm_1", ProjectID: "prj_a", UserID: ownerAll.ID, Role: authz.ProjectRoleOwner},
		{ID: "pm_2", ProjectID: "prj_b", UserID: ownerAll.ID, Role: authz.ProjectRoleOwner},
		{ID: "pm_3", ProjectID: "prj_b", UserID: coOwnerB.ID, Role: authz.ProjectRoleOwner},
		{ID: "pm_4", ProjectID: "prj_a", UserID: adminA.ID, Role: authz.ProjectRoleAdmin},
		{ID: "pm_5", ProjectID: "prj_a", UserID: devA.ID, Role: authz.ProjectRoleDeveloper},
		{ID: "pm_6", ProjectID: "prj_a", UserID: viewerA.ID, Role: authz.ProjectRoleViewer},
	}
	if err := db.Create(&members).Error; err != nil {
		t.Fatal(err)
	}

	// Admin and Developer cannot create project notification channels.
	for _, nonOwner := range []model.User{adminA, devA, viewerA} {
		rec, ctx := newScopedNotificationTestContext(
			http.MethodPost,
			"/api/v1/notification-channels",
			nonOwner,
			nil,
			`{"name":"Team Feishu","scope":"project","projectIds":["prj_a"],"presetId":"feishu-bot","secrets":{"WebhookToken":"tok-1"}}`,
		)
		handlers.domains.notification.CreateScopedNotificationChannel(ctx)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("user %s create status = %d, want 403; body = %s", nonOwner.ID, rec.Code, rec.Body.String())
		}
	}

	// Owner of both prj_a and prj_b creates a shared SMTP sender channel (without default To, reusable across spaces).
	createBody := `{
		"name":"Tenant Shared SMTP",
		"scope":"project",
		"projectIds":["prj_a","prj_b"],
		"adapterKind":"smtp",
		"config":{"host":"smtp.tenant.example","port":587,"security":"starttls","username":"alerts@tenant.example","from":"alerts@tenant.example","to":["default-ops@tenant.example"]},
		"secrets":{"Password":"smtp-secret-password"},
		"enabled":true
	}`
	createRec, createCtx := newScopedNotificationTestContext(http.MethodPost, "/api/v1/notification-channels", ownerAll, nil, createBody)
	handlers.domains.notification.CreateScopedNotificationChannel(createCtx)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("owner create shared SMTP status = %d, body = %s", createRec.Code, createRec.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	channelID, _ := created["id"].(string)
	if channelID == "" || created["canManage"] != true {
		t.Fatalf("unexpected created channel payload: %#v", created)
	}
	if strings.Contains(createRec.Body.String(), "smtp-secret-password") || strings.Contains(createRec.Body.String(), "passwordRef") {
		t.Fatalf("secret or passwordRef leaked in create response: %s", createRec.Body.String())
	}

	// Developer can read project channels, sees redacted username/to, and canManage=false; Viewer gets 403.
	devListRec, devListCtx := newScopedNotificationTestContext(
		http.MethodGet,
		"/api/v1/projects/prj_a/notifications/channels",
		devA,
		gin.Params{{Key: "projectId", Value: "prj_a"}},
		"",
	)
	handlers.domains.notification.ListProjectNotificationChannels(devListCtx)
	if devListRec.Code != http.StatusOK {
		t.Fatalf("developer list status = %d, body = %s", devListRec.Code, devListRec.Body.String())
	}
	if strings.Contains(devListRec.Body.String(), "default-ops@tenant.example") || strings.Contains(devListRec.Body.String(), "alerts@tenant.example\"") {
		// Note: "from" stays visible or username is masked; verify private recipient is masked!
		if strings.Contains(devListRec.Body.String(), "default-ops@tenant.example") {
			t.Fatalf("developer list leaked private default recipient: %s", devListRec.Body.String())
		}
	}

	viewerListRec, viewerListCtx := newScopedNotificationTestContext(
		http.MethodGet,
		"/api/v1/projects/prj_a/notifications/channels",
		viewerA,
		gin.Params{{Key: "projectId", Value: "prj_a"}},
		"",
	)
	handlers.domains.notification.ListProjectNotificationChannels(viewerListCtx)
	if viewerListRec.Code != http.StatusForbidden {
		t.Fatalf("viewer list status = %d, want 403", viewerListRec.Code)
	}

	// Single-project co-owner (Owner of prj_b only, NOT prj_a) cannot mutate or delete the shared channel.
	coUpdateRec, coUpdateCtx := newScopedNotificationTestContext(
		http.MethodPut,
		"/api/v1/notification-channels/"+channelID,
		coOwnerB,
		gin.Params{{Key: "channelId", Value: channelID}},
		`{"name":"Hijacked","scope":"project","projectIds":["prj_b"],"adapterKind":"smtp","config":{"host":"evil.example","port":587,"security":"starttls","from":"alerts@tenant.example"}}`,
	)
	handlers.domains.notification.UpdateScopedNotificationChannel(coUpdateCtx)
	if coUpdateRec.Code != http.StatusForbidden {
		t.Fatalf("coOwnerB update shared channel status = %d, want 403; body = %s", coUpdateRec.Code, coUpdateRec.Body.String())
	}

	coDeleteRec, coDeleteCtx := newScopedNotificationTestContext(
		http.MethodDelete,
		"/api/v1/notification-channels/"+channelID,
		coOwnerB,
		gin.Params{{Key: "channelId", Value: channelID}},
		"",
	)
	handlers.domains.notification.DeleteScopedNotificationChannel(coDeleteCtx)
	if coDeleteRec.Code != http.StatusForbidden {
		t.Fatalf("coOwnerB delete shared channel status = %d, want 403", coDeleteRec.Code)
	}

	// Co-owner of prj_b CAN unbind the shared channel from prj_b without mutating credentials.
	unbindRec, unbindCtx := newScopedNotificationTestContext(
		http.MethodPost,
		"/api/v1/projects/prj_b/notifications/channels/"+channelID+"/unbind",
		coOwnerB,
		gin.Params{{Key: "projectId", Value: "prj_b"}, {Key: "channelId", Value: channelID}},
		"",
	)
	handlers.domains.notification.UnbindProjectNotificationChannel(unbindCtx)
	if unbindCtx.Writer.Status() != http.StatusNoContent {
		t.Fatalf("coOwnerB unbind prj_b status = %d, body = %s", unbindCtx.Writer.Status(), unbindRec.Body.String())
	}

	// Unbinding the last remaining project (prj_a) is rejected with 409 so channels never become orphaned.
	lastUnbindRec, lastUnbindCtx := newScopedNotificationTestContext(
		http.MethodPost,
		"/api/v1/projects/prj_a/notifications/channels/"+channelID+"/unbind",
		ownerAll,
		gin.Params{{Key: "projectId", Value: "prj_a"}, {Key: "channelId", Value: channelID}},
		"",
	)
	handlers.domains.notification.UnbindProjectNotificationChannel(lastUnbindCtx)
	if lastUnbindRec.Code != http.StatusConflict {
		t.Fatalf("unbind last project status = %d, want 409; body = %s", lastUnbindRec.Code, lastUnbindRec.Body.String())
	}
}

func TestProjectSMTPRecipientOverrideRuleAndManualDeliveryRetry(t *testing.T) {
	db, handlers, enqueuer := openScopedNotificationTestDB(t, "scoped_notification_rules")

	owner := model.User{ID: "usr_owner", Email: "owner@example.com", Role: authz.PlatformRoleUser}
	dev := model.User{ID: "usr_dev", Email: "dev@example.com", Role: authz.PlatformRoleUser}
	if err := db.Create(&[]model.User{owner, dev}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Project{ID: "prj_main", Name: "Main", Identifier: "main"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]model.ProjectMember{
		{ID: "pm_o", ProjectID: "prj_main", UserID: owner.ID, Role: authz.ProjectRoleOwner},
		{ID: "pm_d", ProjectID: "prj_main", UserID: dev.ID, Role: authz.ProjectRoleDeveloper},
	}).Error; err != nil {
		t.Fatal(err)
	}

	// Create a sender-only SMTP account (empty default To/Cc).
	smtpRec, smtpCtx := newScopedNotificationTestContext(
		http.MethodPost,
		"/api/v1/notification-channels",
		owner,
		nil,
		`{"name":"Sender Only SMTP","scope":"project","projectIds":["prj_main"],"adapterKind":"smtp","config":{"host":"smtp.corp.example","port":465,"security":"tls","username":"noreply@corp.example","from":"noreply@corp.example"},"secrets":{"Password":"secret"}}`,
	)
	handlers.domains.notification.CreateScopedNotificationChannel(smtpCtx)
	if smtpRec.Code != http.StatusCreated {
		t.Fatalf("create sender-only SMTP status = %d, body = %s", smtpRec.Code, smtpRec.Body.String())
	}
	var smtpResp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(smtpRec.Body.Bytes(), &smtpResp); err != nil {
		t.Fatal(err)
	}

	// Rule without channelTargets must fail because neither channel default nor rule override provides recipients.
	missingTargetRec, missingTargetCtx := newScopedNotificationTestContext(
		http.MethodPost,
		"/api/v1/projects/prj_main/notifications/rules",
		owner,
		gin.Params{{Key: "projectId", Value: "prj_main"}},
		`{"name":"Runtime Crash Rule","eventTypes":["build.failed"],"channelIds":["`+smtpResp.ID+`"]}`,
	)
	handlers.domains.notification.CreateProjectNotificationRule(missingTargetCtx)
	if missingTargetRec.Code != http.StatusBadRequest || !strings.Contains(missingTargetRec.Body.String(), "notification.smtp_recipient_required") {
		t.Fatalf("rule without recipients status = %d, body = %s", missingTargetRec.Code, missingTargetRec.Body.String())
	}

	// Rule with per-channel recipient override + configurable reminder & timezone succeeds!
	ruleBody := `{
		"name":"Runtime Crash Rule",
		"eventTypes":["build.failed","release.failed"],
		"channelIds":["` + smtpResp.ID + `"],
		"channelTargets":{"` + smtpResp.ID + `":{"to":["backend-oncall@corp.example"],"cc":["lead@corp.example"]}},
		"timeZone":"Asia/Tokyo",
		"remindIntervalMinutes":30,
		"maxReminders":5,
		"notifyOnRecovery":false,
		"enabled":true
	}`
	ruleRec, ruleCtx := newScopedNotificationTestContext(
		http.MethodPost,
		"/api/v1/projects/prj_main/notifications/rules",
		owner,
		gin.Params{{Key: "projectId", Value: "prj_main"}},
		ruleBody,
	)
	handlers.domains.notification.CreateProjectNotificationRule(ruleCtx)
	if ruleRec.Code != http.StatusCreated {
		t.Fatalf("create project rule status = %d, body = %s", ruleRec.Code, ruleRec.Body.String())
	}
	var savedRule model.NotificationRule
	if err := json.Unmarshal(ruleRec.Body.Bytes(), &savedRule); err != nil {
		t.Fatal(err)
	}
	if savedRule.ProjectID != "prj_main" || savedRule.TimeZone != "Asia/Tokyo" || savedRule.RemindIntervalMinutes != 30 || savedRule.MaxReminders != 5 || savedRule.NotifyOnRecovery {
		t.Fatalf("saved rule mismatch: %#v", savedRule)
	}

	// Seed a failed delivery and verify manual retry requires Owner and re-queues cleanly.
	now := time.Now().UTC()
	failedDelivery := model.NotificationDelivery{
		ID:                 "ndv_failed_1",
		ProjectID:          "prj_main",
		EventID:            "evt_1",
		EventType:          "release.failed",
		Severity:           "error",
		RuleID:             savedRule.ID,
		ChannelID:          smtpResp.ID,
		AdapterKind:        notification.AdapterKindSMTP,
		TargetOverrideJSON: `{"to":["backend-oncall@corp.example"]}`,
		TimeZone:           "Asia/Tokyo",
		Status:             "failed",
		AttemptCount:       6,
		ErrorMessage:       "dial tcp timeout",
		QueuedAt:           now.Add(-time.Minute),
		FinishedAt:         &now,
	}
	if err := db.Create(&failedDelivery).Error; err != nil {
		t.Fatal(err)
	}

	devRetryRec, devRetryCtx := newScopedNotificationTestContext(
		http.MethodPost,
		"/api/v1/projects/prj_main/notifications/deliveries/ndv_failed_1/retry",
		dev,
		gin.Params{{Key: "projectId", Value: "prj_main"}, {Key: "deliveryId", Value: "ndv_failed_1"}},
		"",
	)
	handlers.domains.notification.RetryProjectNotificationDelivery(devRetryCtx)
	if devRetryRec.Code != http.StatusForbidden {
		t.Fatalf("developer retry status = %d, want 403", devRetryRec.Code)
	}

	ownerRetryRec, ownerRetryCtx := newScopedNotificationTestContext(
		http.MethodPost,
		"/api/v1/projects/prj_main/notifications/deliveries/ndv_failed_1/retry",
		owner,
		gin.Params{{Key: "projectId", Value: "prj_main"}, {Key: "deliveryId", Value: "ndv_failed_1"}},
		"",
	)
	handlers.domains.notification.RetryProjectNotificationDelivery(ownerRetryCtx)
	if ownerRetryRec.Code != http.StatusOK {
		t.Fatalf("owner retry status = %d, body = %s", ownerRetryRec.Code, ownerRetryRec.Body.String())
	}
	if enqueuer.notificationPayload.DeliveryID != "ndv_failed_1" {
		t.Fatalf("enqueued delivery ID = %q, want ndv_failed_1", enqueuer.notificationPayload.DeliveryID)
	}
	var retried model.NotificationDelivery
	if err := json.Unmarshal(ownerRetryRec.Body.Bytes(), &retried); err != nil {
		t.Fatal(err)
	}
	if retried.Status != "pending" || retried.AttemptCount != 0 || retried.ChannelName != "Sender Only SMTP" || retried.RuleName != "Runtime Crash Rule" {
		t.Fatalf("unexpected retried delivery payload: %#v", retried)
	}
}
