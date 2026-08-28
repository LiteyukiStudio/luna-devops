package notification

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

type recordingNotificationEnqueuer struct {
	payloads []tasks.NotificationDeliverPayload
	attempts int
	failures int
}

func (e *recordingNotificationEnqueuer) EnqueueNotificationDeliver(_ context.Context, payload tasks.NotificationDeliverPayload) (*asynq.TaskInfo, error) {
	e.attempts++
	if e.failures > 0 {
		e.failures--
		return nil, errors.New("notification queue unavailable")
	}
	e.payloads = append(e.payloads, payload)
	return &asynq.TaskInfo{}, nil
}

func TestNotificationDeliveryRetriesEnqueueFailureOnEventReplay(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "personal_notification_enqueue_retry_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(
				&model.User{},
				&model.PlatformEvent{},
				&model.UserNotificationPreference{},
				&model.NotificationChannel{},
				&model.NotificationRule{},
				&model.NotificationDelivery{},
			)
		},
	})
	user := model.User{ID: "usr_retry", Email: "retry@example.com", Name: "Retry User"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create retry user: %v", err)
	}
	enqueuer := &recordingNotificationEnqueuer{failures: 1}
	service := Service{DB: db, Enqueuer: enqueuer}
	event := Event{
		ID:       "evt_retry",
		Type:     "build.failed",
		Severity: SeverityError,
		Actor:    ActorContext{ID: user.ID},
		DedupKey: "build:retry",
		Message:  "build failed",
	}
	if _, err := service.Emit(t.Context(), event); err == nil {
		t.Fatal("first emit unexpectedly succeeded while the queue was unavailable")
	}
	var failed model.NotificationDelivery
	if err := db.First(&failed, "event_id = ? and channel_id = ?", event.ID, UserEmailChannelID).Error; err != nil {
		t.Fatalf("load enqueue-failed delivery: %v", err)
	}
	if failed.Status != "enqueue_failed" {
		t.Fatalf("delivery status = %q, want enqueue_failed", failed.Status)
	}
	if failed.ErrorMessage != "notification.personal_delivery_enqueue_failed" {
		t.Fatalf("delivery error = %q, want stable personal enqueue error", failed.ErrorMessage)
	}

	replayed := event
	replayed.ID = "evt_retry_replay"
	deliveries, err := service.Emit(t.Context(), replayed)
	if err != nil {
		t.Fatalf("replay enqueue-failed event: %v", err)
	}
	if len(deliveries) != 1 || deliveries[0].ID != failed.ID || enqueuer.attempts != 2 || len(enqueuer.payloads) != 1 {
		t.Fatalf("replay deliveries=%#v attempts=%d payloads=%#v", deliveries, enqueuer.attempts, enqueuer.payloads)
	}
	var recovered model.NotificationDelivery
	if err := db.First(&recovered, "id = ?", failed.ID).Error; err != nil {
		t.Fatalf("load recovered delivery: %v", err)
	}
	if recovered.Status != "pending" || recovered.ErrorMessage != "" {
		t.Fatalf("recovered delivery status=%q error=%q", recovered.Status, recovered.ErrorMessage)
	}
}

func TestPersonalNotificationFanoutUsesActorAndDefaultsWithDeduplication(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "personal_notification_service_test",
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(
				&model.User{},
				&model.PlatformEvent{},
				&model.UserNotificationPreference{},
				&model.NotificationChannel{},
				&model.NotificationRule{},
				&model.NotificationDelivery{},
			)
		},
	})
	users := []model.User{
		{ID: "usr_actor", Email: "actor-internal@example.com", Name: "Actor"},
		{ID: "usr_other", Email: "other@example.com", Name: "Other"},
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatalf("create users: %v", err)
	}
	channels := []model.NotificationChannel{
		{ID: "nch_actor", OwnerUserID: "usr_actor", Name: "actor webhook", AdapterKind: AdapterKindWebhook, ConfigJSON: `{}`, SecretRefsJSON: `{}`, Enabled: true},
		{ID: "nch_other", OwnerUserID: "usr_other", Name: "other webhook", AdapterKind: AdapterKindWebhook, ConfigJSON: `{}`, SecretRefsJSON: `{}`, Enabled: true},
	}
	if err := db.Create(&channels).Error; err != nil {
		t.Fatalf("create channels: %v", err)
	}

	enqueuer := &recordingNotificationEnqueuer{}
	service := Service{DB: db, Enqueuer: enqueuer}
	event := Event{
		ID:       "evt_actor_failure_1",
		Type:     "build.failed",
		Severity: SeverityError,
		Actor: ActorContext{
			ID:    "usr_actor",
			Email: "spoofed-external@example.com",
		},
		DedupKey: "build:actor-failure",
		Message:  "build failed",
	}
	deliveries, err := service.Emit(t.Context(), event)
	if err != nil {
		t.Fatalf("emit actor event: %v", err)
	}
	if len(deliveries) != 2 {
		t.Fatalf("delivery count = %d, want email and actor webhook: %#v", len(deliveries), deliveries)
	}
	channelsByID := map[string]bool{}
	for _, delivery := range deliveries {
		if delivery.RecipientUserID != "usr_actor" {
			t.Fatalf("delivery recipient = %q, want actor", delivery.RecipientUserID)
		}
		channelsByID[delivery.ChannelID] = true
	}
	if !channelsByID[UserEmailChannelID] || !channelsByID["nch_actor"] || channelsByID["nch_other"] {
		t.Fatalf("personal delivery channels = %#v", channelsByID)
	}
	if len(enqueuer.payloads) != 2 {
		t.Fatalf("enqueued deliveries = %d, want 2", len(enqueuer.payloads))
	}
	lateChannel := model.NotificationChannel{
		ID:             "nch_actor_late",
		OwnerUserID:    "usr_actor",
		Name:           "late actor webhook",
		AdapterKind:    AdapterKindWebhook,
		ConfigJSON:     `{}`,
		SecretRefsJSON: `{}`,
		Enabled:        true,
	}
	if err := db.Create(&lateChannel).Error; err != nil {
		t.Fatalf("create late actor channel: %v", err)
	}

	duplicate := event
	duplicate.ID = "evt_actor_failure_duplicate"
	duplicate.Type = "release.failed"
	duplicate.Project = EntityRef{ID: "prj_spoofed"}
	duplicate.Actor = ActorContext{ID: "usr_other", Email: "other@example.com"}
	duplicate.Message = "spoofed replay message"
	duplicateDeliveries, err := service.Emit(t.Context(), duplicate)
	if err != nil {
		t.Fatalf("emit duplicate event: %v", err)
	}
	if len(duplicateDeliveries) != 1 || duplicateDeliveries[0].ChannelID != lateChannel.ID || len(enqueuer.payloads) != 3 {
		t.Fatalf("duplicate created %d deliveries and %d total tasks", len(duplicateDeliveries), len(enqueuer.payloads))
	}
	var authoritativeReplay Event
	if err := json.Unmarshal([]byte(duplicateDeliveries[0].EventJSON), &authoritativeReplay); err != nil {
		t.Fatalf("decode duplicate delivery event: %v", err)
	}
	if authoritativeReplay.Type != event.Type || authoritativeReplay.Project.ID != event.Project.ID ||
		authoritativeReplay.Actor.ID != event.Actor.ID || authoritativeReplay.Message != event.Message {
		t.Fatalf("duplicate delivery used replay input instead of recorded event: %#v", authoritativeReplay)
	}
	if err := db.Select("user_id", "email_enabled", "event_types_json").Create(&model.UserNotificationPreference{
		UserID:         "usr_other",
		EmailEnabled:   false,
		EventTypesJSON: EncodeStringList([]string{"build.failed"}),
	}).Error; err != nil {
		t.Fatalf("create email-disabled preference: %v", err)
	}
	emailDisabled := event
	emailDisabled.ID = "evt_email_disabled"
	emailDisabled.DedupKey = "build:email-disabled"
	emailDisabled.Actor = ActorContext{ID: "usr_other", Email: "spoofed-other@example.com"}
	emailDisabledDeliveries, err := service.Emit(t.Context(), emailDisabled)
	if err != nil {
		t.Fatalf("emit email-disabled event: %v", err)
	}
	if len(emailDisabledDeliveries) != 1 || emailDisabledDeliveries[0].ChannelID != "nch_other" {
		t.Fatalf("email-disabled deliveries = %#v, want personal webhook only", emailDisabledDeliveries)
	}
	if len(enqueuer.payloads) != 4 {
		t.Fatalf("tasks after email-disabled event = %d, want 4", len(enqueuer.payloads))
	}

	outsider := event
	outsider.ID = "evt_external_failure"
	outsider.DedupKey = "build:external-failure"
	outsider.Actor = ActorContext{ID: "external_actor", Email: "actor-internal@example.com"}
	outsiderDeliveries, err := service.Emit(t.Context(), outsider)
	if err != nil {
		t.Fatalf("emit external actor event: %v", err)
	}
	if len(outsiderDeliveries) != 0 || len(enqueuer.payloads) != 4 {
		t.Fatalf("external actor created %d deliveries and %d total tasks", len(outsiderDeliveries), len(enqueuer.payloads))
	}
}
