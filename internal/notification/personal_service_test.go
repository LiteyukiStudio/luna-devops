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
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

type recordingNotificationEnqueuer struct {
	payloads      []tasks.NotificationDeliverPayload
	attempts      int
	failures      int
	enqueueErrors []error
	traceIDs      []trace.TraceID
}

func (e *recordingNotificationEnqueuer) EnqueueNotificationDeliver(ctx context.Context, payload tasks.NotificationDeliverPayload) (*asynq.TaskInfo, error) {
	e.attempts++
	e.traceIDs = append(e.traceIDs, trace.SpanContextFromContext(ctx).TraceID())
	if len(e.enqueueErrors) > 0 {
		err := e.enqueueErrors[0]
		e.enqueueErrors = e.enqueueErrors[1:]
		if err != nil {
			return nil, err
		}
	}
	if e.failures > 0 {
		e.failures--
		return nil, errors.New("notification queue unavailable")
	}
	e.payloads = append(e.payloads, payload)
	return &asynq.TaskInfo{}, nil
}

func TestNotificationFanoutPersistsAllTargetsBeforeEnqueueFailures(t *testing.T) {
	for _, tt := range []struct {
		name                string
		withSharedTarget    bool
		wantDeliveryCount   int
		wantFailedRecipient string
	}{
		{name: "shared failure does not block personal targets", withSharedTarget: true, wantDeliveryCount: 3, wantFailedRecipient: ""},
		{name: "actor failure does not block owner", wantDeliveryCount: 2, wantFailedRecipient: "usr_fanout_actor"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := testdb.Open(t, testdb.Options{
				SchemaPrefix: "notification_fanout_enqueue_failure_test",
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
				{ID: "usr_fanout_actor", Email: "actor@example.com", Name: "Actor"},
				{ID: "usr_fanout_owner", Email: "owner@example.com", Name: "Owner"},
			}
			if err := db.Create(&users).Error; err != nil {
				t.Fatalf("create fanout users: %v", err)
			}
			if tt.withSharedTarget {
				channel := model.NotificationChannel{
					ID: "nch_fanout_shared", Name: "shared", AdapterKind: AdapterKindWebhook,
					ConfigJSON: `{}`, SecretRefsJSON: `{}`, Enabled: true,
				}
				if err := db.Create(&channel).Error; err != nil {
					t.Fatalf("create shared channel: %v", err)
				}
				filterJSON, err := EncodeRuleFilter(RuleFilter{Scope: RuleScopeAll})
				if err != nil {
					t.Fatalf("encode shared rule filter: %v", err)
				}
				rule := model.NotificationRule{
					ID: "nrl_fanout_shared", Name: "shared", Enabled: true,
					EventTypesJSON: EncodeStringList([]string{"build.failed"}),
					FilterJSON:     filterJSON, ChannelIDsJSON: EncodeStringList([]string{channel.ID}),
				}
				if err := db.Create(&rule).Error; err != nil {
					t.Fatalf("create shared rule: %v", err)
				}
			}

			enqueuer := &recordingNotificationEnqueuer{failures: 1}
			service := Service{DB: db, Enqueuer: enqueuer}
			deliveries, err := service.Emit(t.Context(), Event{
				ID: "evt_fanout_failure", Type: "build.failed", Severity: SeverityError,
				Actor: ActorContext{ID: users[0].ID}, ResourceOwnerUserID: users[1].ID,
			})
			if err == nil {
				t.Fatal("fanout unexpectedly hid the first enqueue failure")
			}
			if len(deliveries) != tt.wantDeliveryCount || enqueuer.attempts != tt.wantDeliveryCount {
				t.Fatalf("deliveries=%d attempts=%d, want %d", len(deliveries), enqueuer.attempts, tt.wantDeliveryCount)
			}

			var stored []model.NotificationDelivery
			if err := db.Order("created_at asc, id asc").Find(&stored).Error; err != nil {
				t.Fatalf("load fanout deliveries: %v", err)
			}
			if len(stored) != tt.wantDeliveryCount {
				t.Fatalf("stored deliveries=%d, want %d", len(stored), tt.wantDeliveryCount)
			}
			var storedEvent model.PlatformEvent
			if err := db.First(&storedEvent, "id = ?", "evt_fanout_failure").Error; err != nil {
				t.Fatalf("load committed event after enqueue failure: %v", err)
			}
			if storedEvent.NotificationFanoutStatus != NotificationFanoutStatusCompleted {
				t.Fatalf("event fanout status = %q, want completed", storedEvent.NotificationFanoutStatus)
			}
			byRecipient := make(map[string]model.NotificationDelivery, len(stored))
			for _, delivery := range stored {
				byRecipient[delivery.RecipientUserID] = delivery
			}
			failed := byRecipient[tt.wantFailedRecipient]
			if failed.Status != "enqueue_failed" {
				t.Fatalf("first target status=%q, want enqueue_failed", failed.Status)
			}
			owner := byRecipient[users[1].ID]
			if owner.ID == "" || owner.Status != "pending" {
				t.Fatalf("owner delivery=%#v, want persisted pending delivery", owner)
			}
			ownerEnqueued := false
			for _, payload := range enqueuer.payloads {
				ownerEnqueued = ownerEnqueued || payload.DeliveryID == owner.ID
			}
			if !ownerEnqueued {
				t.Fatalf("owner delivery %q was not enqueued after the earlier target failed", owner.ID)
			}
		})
	}
}

func TestNotificationEmitLeavesPendingEventWhenFanoutPreparationFails(t *testing.T) {
	injectedErr := errors.New("injected notification fanout database failure")
	for _, tt := range []struct {
		name              string
		eventID           string
		actorID           string
		wantDeliveryCount int
		prepare           func(*testing.T, *gorm.DB)
		inject            func(*testing.T, *gorm.DB) func()
	}{
		{
			name:    "target resolution failure",
			eventID: "evt_atomic_target_failure",
			inject: func(t *testing.T, db *gorm.DB) func() {
				t.Helper()
				const callbackName = "test:fail_notification_rule_target_query"
				if err := db.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
					if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Table == "notification_rules" {
						tx.AddError(injectedErr)
					}
				}); err != nil {
					t.Fatalf("register target query failure: %v", err)
				}
				return func() {
					if err := db.Callback().Query().Remove(callbackName); err != nil {
						t.Fatalf("remove target query failure: %v", err)
					}
				}
			},
		},
		{
			name:              "delivery persistence failure",
			eventID:           "evt_atomic_delivery_failure",
			actorID:           "usr_atomic_delivery",
			wantDeliveryCount: 1,
			prepare: func(t *testing.T, db *gorm.DB) {
				t.Helper()
				if err := db.Create(&model.User{ID: "usr_atomic_delivery", Email: "atomic@example.com", Name: "Atomic"}).Error; err != nil {
					t.Fatalf("create atomic delivery user: %v", err)
				}
			},
			inject: func(t *testing.T, db *gorm.DB) func() {
				t.Helper()
				const callbackName = "test:fail_notification_delivery_create"
				if err := db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
					if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Table == "notification_deliveries" {
						tx.AddError(injectedErr)
					}
				}); err != nil {
					t.Fatalf("register delivery create failure: %v", err)
				}
				return func() {
					if err := db.Callback().Create().Remove(callbackName); err != nil {
						t.Fatalf("remove delivery create failure: %v", err)
					}
				}
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := testdb.Open(t, testdb.Options{
				SchemaPrefix: "notification_emit_atomicity_test",
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
			if tt.prepare != nil {
				tt.prepare(t, db)
			}
			removeFault := tt.inject(t, db)

			event := Event{ID: tt.eventID, Type: "build.failed", Severity: SeverityError, Actor: ActorContext{ID: tt.actorID}}
			service := Service{DB: db}
			if _, err := service.Emit(t.Context(), event); !errors.Is(err, injectedErr) {
				t.Fatalf("Emit error = %v, want injected fanout error", err)
			}

			var storedEvent model.PlatformEvent
			if err := db.First(&storedEvent, "id = ?", tt.eventID).Error; err != nil {
				t.Fatalf("load pending platform event: %v", err)
			}
			if storedEvent.NotificationFanoutStatus != NotificationFanoutStatusPending {
				t.Fatalf("event fanout status = %q, want pending", storedEvent.NotificationFanoutStatus)
			}
			var deliveryCount int64
			if err := db.Model(&model.NotificationDelivery{}).Where("event_id = ?", tt.eventID).Count(&deliveryCount).Error; err != nil {
				t.Fatalf("count rolled-back fanout deliveries: %v", err)
			}
			if deliveryCount != 0 {
				t.Fatalf("delivery count after failed materialization = %d, want 0", deliveryCount)
			}

			removeFault()
			deliveries, err := service.MaterializeEvent(t.Context(), storedEvent.ID)
			if err != nil {
				t.Fatalf("recover pending fanout: %v", err)
			}
			if len(deliveries) != tt.wantDeliveryCount {
				t.Fatalf("recovered deliveries = %d, want %d", len(deliveries), tt.wantDeliveryCount)
			}
			if err := db.First(&storedEvent, "id = ?", tt.eventID).Error; err != nil {
				t.Fatalf("reload recovered platform event: %v", err)
			}
			if storedEvent.NotificationFanoutStatus != NotificationFanoutStatusCompleted {
				t.Fatalf("recovered event fanout status = %q, want completed", storedEvent.NotificationFanoutStatus)
			}
		})
	}
}

func TestNotificationDeliveryReenqueuesPendingReplayAndAcceptsDuplicateTask(t *testing.T) {
	for _, tt := range []struct {
		name       string
		enqueueErr error
	}{
		{name: "pending replay is enqueued"},
		{name: "duplicate task means pending already has a task", enqueueErr: asynq.ErrDuplicateTask},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := testdb.Open(t, testdb.Options{
				SchemaPrefix: "notification_pending_replay_test",
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
			user := model.User{ID: "usr_pending_replay", Email: "pending@example.com", Name: "Pending"}
			if err := db.Create(&user).Error; err != nil {
				t.Fatalf("create pending replay user: %v", err)
			}
			event := Event{
				ID: "evt_pending_replay", Type: "build.failed", Severity: SeverityError,
				Actor: ActorContext{ID: user.ID}, DedupKey: "build:pending-replay",
			}
			initial, err := (Service{DB: db}).Emit(t.Context(), event)
			if err != nil || len(initial) != 1 || initial[0].Status != "pending" {
				t.Fatalf("initial pending delivery=%#v err=%v", initial, err)
			}

			enqueuer := &recordingNotificationEnqueuer{enqueueErrors: []error{tt.enqueueErr}}
			replayed := event
			replayed.ID = "evt_pending_replay_duplicate"
			deliveries, err := (Service{DB: db, Enqueuer: enqueuer}).Emit(t.Context(), replayed)
			if err != nil {
				t.Fatalf("replay pending delivery: %v", err)
			}
			if len(deliveries) != 1 || deliveries[0].ID != initial[0].ID || enqueuer.attempts != 1 {
				t.Fatalf("replay deliveries=%#v attempts=%d", deliveries, enqueuer.attempts)
			}
			var stored model.NotificationDelivery
			if err := db.First(&stored, "id = ?", initial[0].ID).Error; err != nil {
				t.Fatalf("load replayed pending delivery: %v", err)
			}
			if stored.Status != "pending" || stored.ErrorMessage != "" {
				t.Fatalf("replayed pending status=%q error=%q", stored.Status, stored.ErrorMessage)
			}
		})
	}
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
	if failed.ErrorMessage != PersonalDeliveryEnqueueFailedCode {
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
	if len(duplicateDeliveries) != 3 || len(enqueuer.payloads) != 5 {
		t.Fatalf("duplicate created %d deliveries and %d total tasks", len(duplicateDeliveries), len(enqueuer.payloads))
	}
	replayedChannels := make(map[string]bool, len(duplicateDeliveries))
	for _, duplicateDelivery := range duplicateDeliveries {
		replayedChannels[duplicateDelivery.ChannelID] = true
		var authoritativeReplay Event
		if err := json.Unmarshal([]byte(duplicateDelivery.EventJSON), &authoritativeReplay); err != nil {
			t.Fatalf("decode duplicate delivery event: %v", err)
		}
		if authoritativeReplay.Type != event.Type || authoritativeReplay.Project.ID != event.Project.ID ||
			authoritativeReplay.Actor.ID != event.Actor.ID || authoritativeReplay.Message != event.Message {
			t.Fatalf("duplicate delivery used replay input instead of recorded event: %#v", authoritativeReplay)
		}
	}
	if !replayedChannels[UserEmailChannelID] || !replayedChannels["nch_actor"] || !replayedChannels[lateChannel.ID] || replayedChannels["nch_other"] {
		t.Fatalf("duplicate replay channels = %#v", replayedChannels)
	}
	if err := db.Model(&model.UserNotificationPreference{}).Create(map[string]any{
		"user_id":          "usr_other",
		"email_enabled":    false,
		"event_types_json": EncodeStringList([]string{"build.failed"}),
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
	if len(enqueuer.payloads) != 6 {
		t.Fatalf("tasks after email-disabled event = %d, want 6", len(enqueuer.payloads))
	}

	outsider := event
	outsider.ID = "evt_external_failure"
	outsider.DedupKey = "build:external-failure"
	outsider.Actor = ActorContext{ID: "external_actor", Email: "actor-internal@example.com"}
	outsiderDeliveries, err := service.Emit(t.Context(), outsider)
	if err != nil {
		t.Fatalf("emit external actor event: %v", err)
	}
	if len(outsiderDeliveries) != 0 || len(enqueuer.payloads) != 6 {
		t.Fatalf("external actor created %d deliveries and %d total tasks", len(outsiderDeliveries), len(enqueuer.payloads))
	}
}
