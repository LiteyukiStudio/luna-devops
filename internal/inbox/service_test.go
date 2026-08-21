package inbox

import (
	"context"
	"errors"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
)

func TestNewMessageValidatesInternalDeepLink(t *testing.T) {
	t.Parallel()
	valid := []string{"", "/projects", "/projects/prj_demo?tab=billing", "/inbox#pending"}
	for _, deepLink := range valid {
		deepLink := deepLink
		t.Run("valid_"+deepLink, func(t *testing.T) {
			t.Parallel()
			message, err := newMessage(validPublishInput(deepLink))
			if err != nil {
				t.Fatalf("new message: %v", err)
			}
			if message.DeepLink != deepLink {
				t.Fatalf("deep link = %q", message.DeepLink)
			}
		})
	}

	invalid := []string{
		"https://example.com/projects",
		"//example.com/projects",
		`/projects\settings`,
		"/projects/%5Csettings",
		"projects",
		"/projects\nsettings",
	}
	for _, deepLink := range invalid {
		deepLink := deepLink
		t.Run("invalid_"+deepLink, func(t *testing.T) {
			t.Parallel()
			_, err := newMessage(validPublishInput(deepLink))
			if !errors.Is(err, ErrInvalidDeepLink) {
				t.Fatalf("error = %v, want invalid deep link", err)
			}
		})
	}
}

func TestNewMessageRejectsInvalidEnumsAndJSON(t *testing.T) {
	t.Parallel()
	input := validPublishInput("/projects")
	input.Category = "unknown"
	if _, err := newMessage(input); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("category error = %v", err)
	}
	input = validPublishInput("/projects")
	input.Priority = "urgent"
	if _, err := newMessage(input); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("priority error = %v", err)
	}
	input = validPublishInput("/projects")
	input.Params = map[string]any{"invalid": make(chan struct{})}
	if _, err := newMessage(input); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("params error = %v", err)
	}
}

func TestNewActionRequestUsesFrozenInitialState(t *testing.T) {
	t.Parallel()
	request, err := newActionRequest(CreateActionRequestInput{
		Type: ActionRequestTypeBillingOwnerTransfer, RequesterUserID: "usr_owner", RecipientUserID: "usr_target",
		ProjectID: "prj_demo", Payload: map[string]any{"billingOwnerUserId": "usr_owner"},
	})
	if err != nil {
		t.Fatalf("new action request: %v", err)
	}
	if request.Status != ActionRequestStatusPending || request.RowVersion != 1 {
		t.Fatalf("initial request state = %s/%d", request.Status, request.RowVersion)
	}
	if request.PayloadJSON != `{"billingOwnerUserId":"usr_owner"}` {
		t.Fatalf("payload = %s", request.PayloadJSON)
	}
}

func TestInboxServicePostgresLifecycleAndIsolation(t *testing.T) {
	db := openInboxTestDB(t)
	if err := db.AutoMigrate(&model.InboxActionRequest{}, &model.InboxMessage{}); err != nil {
		t.Fatalf("migrate inbox tables: %v", err)
	}
	service := NewService(db)
	ctx := context.Background()

	request, err := service.CreateActionRequest(ctx, CreateActionRequestInput{
		Type: ActionRequestTypeBillingOwnerTransfer, RequesterUserID: "usr_owner", RecipientUserID: "usr_target",
		ProjectID: "prj_demo", ResourceType: "project", ResourceID: "prj_demo",
		Payload: map[string]any{"previousBillingOwnerUserId": "usr_owner"},
	})
	if err != nil {
		t.Fatalf("create action request: %v", err)
	}
	input := validPublishInput("/projects/prj_demo")
	input.RecipientUserID = "usr_target"
	input.ActionRequestID = request.ID
	input.DedupKey = "billing-transfer:prj_demo:usr_target"
	message, created, err := service.Publish(ctx, input)
	if err != nil || !created {
		t.Fatalf("publish: created=%v err=%v", created, err)
	}
	existing, created, err := service.Publish(ctx, input)
	if err != nil || created || existing.ID != message.ID {
		t.Fatalf("deduplicate: message=%s created=%v err=%v", existing.ID, created, err)
	}

	if count, countErr := service.UnreadCount(ctx, "usr_target"); countErr != nil || count != 1 {
		t.Fatalf("target unread count = %d, err=%v", count, countErr)
	}
	if count, countErr := service.UnreadCount(ctx, "usr_other"); countErr != nil || count != 0 {
		t.Fatalf("other unread count = %d, err=%v", count, countErr)
	}
	if _, getErr := service.Get(ctx, "usr_other", message.ID); !errors.Is(getErr, ErrNotFound) {
		t.Fatalf("cross-user get error = %v", getErr)
	}
	if markErr := service.MarkRead(ctx, "usr_other", message.ID); !errors.Is(markErr, ErrNotFound) {
		t.Fatalf("cross-user mark-read error = %v", markErr)
	}
	if archiveErr := service.Archive(ctx, "usr_other", message.ID); !errors.Is(archiveErr, ErrNotFound) {
		t.Fatalf("cross-user archive error = %v", archiveErr)
	}
	if _, getErr := service.GetActionRequest(ctx, "usr_other", request.ID); !errors.Is(getErr, ErrNotFound) {
		t.Fatalf("cross-user action request error = %v", getErr)
	}

	result, err := service.List(ctx, ListInput{UserID: "usr_target", Filter: "action"})
	if err != nil || result.Total != 1 || len(result.Items) != 1 {
		t.Fatalf("action list = %#v, err=%v", result, err)
	}
	if err := db.Model(&model.InboxActionRequest{}).Where("id = ?", request.ID).Update("status", ActionRequestStatusCompleted).Error; err != nil {
		t.Fatalf("complete action request: %v", err)
	}
	result, err = service.List(ctx, ListInput{UserID: "usr_target", Filter: "action"})
	if err != nil || result.Total != 0 || len(result.Items) != 0 {
		t.Fatalf("completed action request remained actionable: %#v, err=%v", result, err)
	}
	requests, err := service.GetActionRequests(ctx, "usr_target", []string{request.ID, request.ID, "iar_missing"})
	if err != nil || len(requests) != 1 || requests[request.ID].RecipientUserID != "usr_target" {
		t.Fatalf("action request map = %#v, err=%v", requests, err)
	}
	if err := service.MarkRead(ctx, "usr_target", message.ID); err != nil {
		t.Fatalf("mark read: %v", err)
	}
	if count, countErr := service.UnreadCount(ctx, "usr_target"); countErr != nil || count != 0 {
		t.Fatalf("unread after mark = %d, err=%v", count, countErr)
	}
	if err := service.Archive(ctx, "usr_target", message.ID); err != nil {
		t.Fatalf("archive: %v", err)
	}
	result, err = service.List(ctx, ListInput{UserID: "usr_target"})
	if err != nil || result.Total != 0 {
		t.Fatalf("list after archive = %#v, err=%v", result, err)
	}
}

func TestMarkAllReadOnlyTouchesRecipient(t *testing.T) {
	db := openInboxTestDB(t)
	if err := db.AutoMigrate(&model.InboxMessage{}); err != nil {
		t.Fatalf("migrate inbox table: %v", err)
	}
	service := NewService(db)
	for _, userID := range []string{"usr_one", "usr_two"} {
		input := validPublishInput("")
		input.RecipientUserID = userID
		input.DedupKey = "announcement:" + userID
		if _, _, err := service.Publish(context.Background(), input); err != nil {
			t.Fatalf("publish for %s: %v", userID, err)
		}
	}
	if err := service.MarkAllRead(context.Background(), "usr_one"); err != nil {
		t.Fatalf("mark all read: %v", err)
	}
	if count, err := service.UnreadCount(context.Background(), "usr_one"); err != nil || count != 0 {
		t.Fatalf("usr_one unread = %d, err=%v", count, err)
	}
	if count, err := service.UnreadCount(context.Background(), "usr_two"); err != nil || count != 1 {
		t.Fatalf("usr_two unread = %d, err=%v", count, err)
	}
}

func validPublishInput(deepLink string) PublishInput {
	return PublishInput{
		RecipientUserID: "usr_target",
		Type:            "system.announcement",
		Category:        CategorySystem,
		Priority:        PriorityNormal,
		TitleKey:        "inbox.messages.system_announcement.title",
		ContentKey:      "inbox.messages.system_announcement.content",
		Params:          map[string]any{"announcementId": "announcement_demo"},
		DeepLink:        deepLink,
	}
}

func openInboxTestDB(t *testing.T) *gorm.DB {
	return testdb.Open(t, testdb.Options{SchemaPrefix: "inbox_test"})
}
