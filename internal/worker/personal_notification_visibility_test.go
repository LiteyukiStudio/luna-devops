package worker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

func TestPersonalEmailDigestUsesAuthoritativeEventsAndRejectsUnrelatedEntry(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "personal_notification_authority_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(
				&model.User{},
				&model.ProjectMember{},
				&model.PlatformEvent{},
				&model.UserNotificationPreference{},
				&model.NotificationDelivery{},
			)
		},
	})
	user := model.User{ID: "usr_authoritative_recipient", Email: "recipient@example.com", Name: "Recipient", Role: authz.PlatformRoleUser}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create recipient: %v", err)
	}
	validEvent := notification.Event{
		ID: "evt_authoritative_valid", Type: "build.failed", Severity: notification.SeverityError,
		Actor: notification.ActorContext{ID: user.ID}, Message: "authoritative visible failure",
	}
	invalidEvent := notification.Event{
		ID: "evt_authoritative_invalid", Type: "build.failed", Severity: notification.SeverityError,
		Actor: notification.ActorContext{ID: "usr_someone_else"}, Message: "unrelated private failure",
	}
	platformEvents := []model.PlatformEvent{
		platformEventForNotificationTest(t, validEvent),
		platformEventForNotificationTest(t, invalidEvent),
	}
	if err := db.Create(&platformEvents).Error; err != nil {
		t.Fatalf("create authoritative events: %v", err)
	}
	spoofed := validEvent
	spoofed.Message = "spoofed delivery snapshot"
	deliveries := []model.NotificationDelivery{
		{
			ID: "ndl_authoritative_valid", RecipientUserID: user.ID, EventID: validEvent.ID, EventType: validEvent.Type,
			ChannelID: notification.UserEmailChannelID, AdapterKind: notification.AdapterKindSMTP,
			EventJSON: mustMarshalNotificationEvent(t, spoofed), Status: "pending", QueuedAt: time.Now().Add(-time.Second),
		},
		{
			ID: "ndl_authoritative_invalid", RecipientUserID: user.ID, EventID: invalidEvent.ID, EventType: invalidEvent.Type,
			ChannelID: notification.UserEmailChannelID, AdapterKind: notification.AdapterKindSMTP,
			EventJSON: mustMarshalNotificationEvent(t, invalidEvent), Status: "pending", QueuedAt: time.Now(),
		},
	}
	if err := db.Create(&deliveries).Error; err != nil {
		t.Fatalf("create personal deliveries: %v", err)
	}

	var sent notification.RenderedMessage
	runner := &Runner{
		db:                    db,
		personalEmailCooldown: func(context.Context, *gorm.DB) (time.Duration, error) { return 0, nil },
		personalEmailSender: func(_ context.Context, recipient string, message notification.RenderedMessage) (notification.SendResult, error) {
			if recipient != user.Email {
				t.Fatalf("recipient = %q, want %q", recipient, user.Email)
			}
			sent = message
			return notification.SendResult{StatusCode: 250}, nil
		},
	}
	if err := runner.handlePersonalEmailDelivery(t.Context(), deliveries[0]); err != nil {
		t.Fatalf("send authoritative digest: %v", err)
	}
	if !strings.Contains(sent.Body, validEvent.Message) || strings.Contains(sent.Body, spoofed.Message) || strings.Contains(sent.Body, invalidEvent.Message) {
		t.Fatalf("personal digest body did not use only authoritative visible event: %q", sent.Body)
	}

	var stored []model.NotificationDelivery
	if err := db.Order("id asc").Find(&stored).Error; err != nil {
		t.Fatalf("load personal deliveries: %v", err)
	}
	statuses := make(map[string]model.NotificationDelivery, len(stored))
	for _, delivery := range stored {
		statuses[delivery.ID] = delivery
	}
	if statuses[deliveries[0].ID].Status != "succeeded" {
		t.Fatalf("valid delivery status = %q", statuses[deliveries[0].ID].Status)
	}
	if got := statuses[deliveries[1].ID]; got.Status != "failed" || got.ErrorMessage != notification.PersonalRecipientNotRelatedCode {
		t.Fatalf("unrelated delivery status=%q error=%q", got.Status, got.ErrorMessage)
	}
}

func TestPersonalWebhookRejectsUnrelatedRecipientBeforeChannelLookup(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "personal_webhook_authority_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(
				&model.User{},
				&model.ProjectMember{},
				&model.PlatformEvent{},
				&model.UserNotificationPreference{},
				&model.NotificationChannel{},
				&model.NotificationDelivery{},
			)
		},
	})
	user := model.User{ID: "usr_webhook_unrelated", Email: "unrelated@example.com", Name: "Unrelated", Role: authz.PlatformRoleUser}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create unrelated user: %v", err)
	}
	event := notification.Event{ID: "evt_webhook_authority", Type: "build.failed", Actor: notification.ActorContext{ID: "usr_actor"}}
	if err := db.Create(&[]model.PlatformEvent{platformEventForNotificationTest(t, event)}).Error; err != nil {
		t.Fatalf("create authoritative webhook event: %v", err)
	}
	delivery := model.NotificationDelivery{
		ID: "ndl_webhook_authority", RecipientUserID: user.ID, EventID: event.ID, EventType: event.Type,
		ChannelID: "nch_missing_on_purpose", AdapterKind: notification.AdapterKindWebhook,
		EventJSON: mustMarshalNotificationEvent(t, event), Status: "pending", QueuedAt: time.Now(),
	}
	if err := db.Create(&delivery).Error; err != nil {
		t.Fatalf("create webhook delivery: %v", err)
	}
	task, err := tasks.NewNotificationDeliverTask(tasks.NotificationDeliverPayload{DeliveryID: delivery.ID})
	if err != nil {
		t.Fatalf("create webhook delivery task: %v", err)
	}
	runner := &Runner{db: db}
	if err := runner.handleNotificationDeliver(t.Context(), task); !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("unrelated webhook error = %v, want SkipRetry", err)
	}
	var stored model.NotificationDelivery
	if err := db.First(&stored, "id = ?", delivery.ID).Error; err != nil {
		t.Fatalf("load rejected webhook delivery: %v", err)
	}
	if stored.Status != "failed" || stored.ErrorMessage != notification.PersonalRecipientNotRelatedCode {
		t.Fatalf("webhook status=%q error=%q", stored.Status, stored.ErrorMessage)
	}
}

func TestPersonalEmailPreferenceChangeIsAppliedBeforeCooldown(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "personal_email_preference_recheck_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.User{}, &model.UserNotificationPreference{}, &model.NotificationDelivery{})
		},
	})
	user := model.User{ID: "usr_email_disabled", Email: "disabled@example.com", Name: "Disabled", Role: authz.PlatformRoleUser}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create email-disabled user: %v", err)
	}
	if err := db.Model(&model.UserNotificationPreference{}).Create(map[string]any{
		"user_id":          user.ID,
		"email_enabled":    false,
		"event_types_json": notification.EncodeStringList([]string{"build.failed"}),
	}).Error; err != nil {
		t.Fatalf("create disabled email preference: %v", err)
	}
	delivery := model.NotificationDelivery{
		ID: "ndl_email_disabled", RecipientUserID: user.ID, EventID: "evt_email_disabled", EventType: "build.failed",
		ChannelID: notification.UserEmailChannelID, AdapterKind: notification.AdapterKindSMTP,
		EventJSON: `{}`, Status: "pending", QueuedAt: time.Now(),
	}
	if err := db.Create(&delivery).Error; err != nil {
		t.Fatalf("create disabled email delivery: %v", err)
	}
	runner := &Runner{
		db: db,
		personalEmailCooldown: func(context.Context, *gorm.DB) (time.Duration, error) {
			t.Fatal("cooldown must not be evaluated after the user disabled personal email")
			return 0, nil
		},
		personalEmailSender: func(context.Context, string, notification.RenderedMessage) (notification.SendResult, error) {
			t.Fatal("disabled personal email must not be sent")
			return notification.SendResult{}, nil
		},
	}
	if err := runner.handlePersonalEmailDelivery(t.Context(), delivery); err != nil {
		t.Fatalf("apply changed personal email preference: %v", err)
	}
	var stored model.NotificationDelivery
	if err := db.First(&stored, "id = ?", delivery.ID).Error; err != nil {
		t.Fatalf("load disabled email delivery: %v", err)
	}
	if stored.Status != "failed" || stored.ErrorMessage != notification.PersonalEmailDisabledCode {
		t.Fatalf("disabled email status=%q error=%q", stored.Status, stored.ErrorMessage)
	}
}

func platformEventForNotificationTest(t *testing.T, event notification.Event) model.PlatformEvent {
	t.Helper()
	detail, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal authoritative notification event: %v", err)
	}
	return model.PlatformEvent{
		ID: event.ID, Type: event.Type, Category: "build", Severity: event.Severity, Status: "failed",
		ProjectID: event.Project.ID, ActorID: event.Actor.ID, ResourceOwnerUserID: event.ResourceOwnerUserID,
		DetailJSON: string(detail), OccurredAt: time.Now(), CreatedAt: time.Now(),
	}
}
