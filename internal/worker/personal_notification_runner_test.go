package worker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"gorm.io/gorm"
)

func TestPersonalEmailDeliveryUsesInternalUserAddressOnce(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "personal_notification_worker_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.User{}, &model.NotificationDelivery{})
		},
	})
	user := model.User{ID: "usr_recipient", Email: "authoritative@example.com", Name: "Recipient"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	event := notification.Event{
		ID:       "evt_personal_email",
		Type:     "build.failed",
		Severity: notification.SeverityError,
		Actor: notification.ActorContext{
			ID:    user.ID,
			Email: "spoofed@example.com",
		},
		Message: "build failed",
	}
	delivery := model.NotificationDelivery{
		ID:              "ndl_personal_email",
		RecipientUserID: user.ID,
		EventID:         event.ID,
		EventType:       event.Type,
		Severity:        event.Severity,
		ChannelID:       notification.UserEmailChannelID,
		AdapterKind:     notification.AdapterKindSMTP,
		EventJSON:       mustMarshalNotificationEvent(t, event),
		Status:          "pending",
	}
	if err := db.Create(&delivery).Error; err != nil {
		t.Fatalf("create delivery: %v", err)
	}

	var recipients []string
	runner := &Runner{
		db: db,
		personalEmailSender: func(_ context.Context, recipient string, _ notification.RenderedMessage) (notification.SendResult, error) {
			recipients = append(recipients, recipient)
			return notification.SendResult{StatusCode: 250, ResponseSnippet: "sent"}, nil
		},
	}
	if err := runner.handlePersonalEmailDelivery(t.Context(), delivery); err != nil {
		t.Fatalf("handle personal email delivery: %v", err)
	}
	if len(recipients) != 1 || recipients[0] != user.Email {
		t.Fatalf("recipients = %#v, want one authoritative user email", recipients)
	}

	var stored model.NotificationDelivery
	if err := db.First(&stored, "id = ?", delivery.ID).Error; err != nil {
		t.Fatalf("load delivery: %v", err)
	}
	if stored.Status != "succeeded" || stored.AttemptCount != 1 {
		t.Fatalf("stored delivery status=%q attempts=%d", stored.Status, stored.AttemptCount)
	}
}

func TestPersonalEmailDeliveryDoesNotPersistSMTPErrorDetails(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "personal_notification_error_redaction_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.User{}, &model.NotificationDelivery{})
		},
	})
	user := model.User{ID: "usr_redacted", Email: "redacted@example.com", Name: "Recipient"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	event := notification.Event{ID: "evt_redacted", Type: "build.failed", Actor: notification.ActorContext{ID: user.ID}}
	delivery := model.NotificationDelivery{
		ID:              "ndl_redacted",
		RecipientUserID: user.ID,
		EventID:         event.ID,
		EventType:       event.Type,
		ChannelID:       notification.UserEmailChannelID,
		AdapterKind:     notification.AdapterKindSMTP,
		EventJSON:       mustMarshalNotificationEvent(t, event),
		Status:          "pending",
	}
	if err := db.Create(&delivery).Error; err != nil {
		t.Fatalf("create delivery: %v", err)
	}

	const privateSMTPDetail = "smtp.internal.example:2525"
	runner := &Runner{
		db: db,
		personalEmailSender: func(context.Context, string, notification.RenderedMessage) (notification.SendResult, error) {
			return notification.SendResult{}, errors.New("dial tcp " + privateSMTPDetail + ": connection refused")
		},
	}
	if err := runner.handlePersonalEmailDelivery(t.Context(), delivery); err == nil {
		t.Fatal("personal email delivery unexpectedly succeeded")
	}
	var stored model.NotificationDelivery
	if err := db.First(&stored, "id = ?", delivery.ID).Error; err != nil {
		t.Fatalf("load failed delivery: %v", err)
	}
	if stored.ErrorMessage != "notification.personal_email_send_failed" || strings.Contains(stored.ErrorMessage, privateSMTPDetail) {
		t.Fatalf("stored personal delivery error = %q", stored.ErrorMessage)
	}
}

func TestPersonalWebhookDeliveryStorageUsesStableErrorsAndNoResponseBody(t *testing.T) {
	delivery := model.NotificationDelivery{RecipientUserID: "usr_personal", AdapterKind: notification.AdapterKindWebhook}
	storedErr := notificationDeliveryErrorForStorage(delivery, "notification.personal_webhook_send_failed", errors.New("https://hooks.example/path-secret-marker"))
	if storedErr.Error() != "notification.personal_webhook_send_failed" {
		t.Fatalf("stored personal webhook error = %q", storedErr)
	}
	if snippet := notificationDeliveryResponseSnippetForStorage(delivery, "response-secret-marker"); snippet != "" {
		t.Fatalf("stored personal webhook response snippet = %q", snippet)
	}

	shared := model.NotificationDelivery{}
	sharedErr := errors.New("shared diagnostic")
	if got := notificationDeliveryErrorForStorage(shared, "unused", sharedErr); !errors.Is(got, sharedErr) {
		t.Fatalf("shared delivery error = %v", got)
	}
	if got := notificationDeliveryResponseSnippetForStorage(shared, "shared response"); got != "shared response" {
		t.Fatalf("shared response snippet = %q", got)
	}
}

func mustMarshalNotificationEvent(t *testing.T, event notification.Event) string {
	t.Helper()
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal notification event: %v", err)
	}
	return string(data)
}
