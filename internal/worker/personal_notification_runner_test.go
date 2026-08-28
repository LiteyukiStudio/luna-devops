package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
	"github.com/LiteyukiStudio/devops/internal/platformevent"
	"github.com/LiteyukiStudio/devops/internal/platformmail"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

func TestPersonalEmailDeliveryUsesInternalUserAddressOnce(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "personal_notification_worker_test",
		Migrate:      migratePersonalNotificationRunnerTestModels,
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
	createPersonalNotificationRunnerDeliveries(t, db, delivery)

	var recipients []string
	runner := &Runner{
		db: db,
		personalEmailSender: func(_ context.Context, recipient string, _ notification.RenderedMessage) (notification.SendResult, error) {
			recipients = append(recipients, recipient)
			return notification.SendResult{StatusCode: 250, ResponseSnippet: "sent"}, nil
		},
		personalEmailCooldown: func(context.Context, *gorm.DB) (time.Duration, error) { return 0, nil },
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
		Migrate:      migratePersonalNotificationRunnerTestModels,
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
	createPersonalNotificationRunnerDeliveries(t, db, delivery)

	const privateSMTPDetail = "smtp.internal.example:2525"
	runner := &Runner{
		db: db,
		personalEmailSender: func(context.Context, string, notification.RenderedMessage) (notification.SendResult, error) {
			return notification.SendResult{}, errors.New("dial tcp " + privateSMTPDetail + ": connection refused")
		},
		personalEmailCooldown: func(context.Context, *gorm.DB) (time.Duration, error) { return 0, nil },
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

func TestPersonalEmailDeliveryMarksPermanentBatchFailureUniformly(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "personal_notification_batch_failure_test",
		Migrate:      migratePersonalNotificationRunnerTestModels,
	})
	user := model.User{ID: "usr_batch_failure", Email: "batch-failure@example.com", Name: "Recipient"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	deliveries := []model.NotificationDelivery{
		{
			ID: "ndl_batch_failure_first", RecipientUserID: user.ID, EventID: "evt_batch_failure_first", EventType: "build.failed",
			ChannelID: notification.UserEmailChannelID, AdapterKind: notification.AdapterKindSMTP,
			EventJSON: mustMarshalNotificationEvent(t, notification.Event{ID: "evt_batch_failure_first", Type: "build.failed"}),
			Status:    "pending", QueuedAt: time.Now().Add(-time.Second),
		},
		{
			ID: "ndl_batch_failure_second", RecipientUserID: user.ID, EventID: "evt_batch_failure_second", EventType: "release.failed",
			ChannelID: notification.UserEmailChannelID, AdapterKind: notification.AdapterKindSMTP,
			EventJSON: mustMarshalNotificationEvent(t, notification.Event{ID: "evt_batch_failure_second", Type: "release.failed"}),
			Status:    "pending", QueuedAt: time.Now(),
		},
	}
	createPersonalNotificationRunnerDeliveries(t, db, deliveries...)
	runner := &Runner{
		db:                    db,
		personalEmailCooldown: func(context.Context, *gorm.DB) (time.Duration, error) { return 0, nil },
		personalEmailSender: func(context.Context, string, notification.RenderedMessage) (notification.SendResult, error) {
			return notification.SendResult{}, platformmail.ErrInvalidSettings
		},
	}
	err := runner.handlePersonalEmailDelivery(t.Context(), deliveries[0])
	if !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("permanent batch failure error = %v, want SkipRetry", err)
	}
	var stored []model.NotificationDelivery
	if err := db.Order("id asc").Find(&stored).Error; err != nil {
		t.Fatalf("load failed batch deliveries: %v", err)
	}
	if len(stored) != 2 {
		t.Fatalf("stored deliveries = %d, want 2", len(stored))
	}
	for _, delivery := range stored {
		if delivery.Status != "failed" || delivery.AttemptCount != 1 || delivery.ErrorMessage != "notification.personal_email_not_configured" {
			t.Fatalf("delivery %s status=%q attempts=%d error=%q", delivery.ID, delivery.Status, delivery.AttemptCount, delivery.ErrorMessage)
		}
	}
}

func TestPersonalEmailDeliverySchedulesOneUserCooldownSecond(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "personal_notification_cooldown_test",
		Migrate:      migratePersonalNotificationRunnerTestModels,
	})
	user := model.User{ID: "usr_cooldown", Email: "cooldown@example.com", Name: "Cooldown"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	previousStartedAt := time.Now().Add(-10 * time.Second).UTC()
	previousFinishedAt := previousStartedAt.Add(time.Second)
	deliveries := []model.NotificationDelivery{
		{
			ID: "ndl_cooldown_previous", RecipientUserID: user.ID, EventID: "evt_previous", EventType: "build.failed",
			ChannelID: notification.UserEmailChannelID, AdapterKind: notification.AdapterKindSMTP,
			EventJSON: mustMarshalNotificationEvent(t, notification.Event{ID: "evt_previous", Type: "build.failed"}),
			Status:    "succeeded", StartedAt: &previousStartedAt, FinishedAt: &previousFinishedAt,
		},
		{
			ID: "ndl_cooldown_pending", RecipientUserID: user.ID, EventID: "evt_pending", EventType: "release.failed",
			ChannelID: notification.UserEmailChannelID, AdapterKind: notification.AdapterKindSMTP,
			EventJSON: mustMarshalNotificationEvent(t, notification.Event{ID: "evt_pending", Type: "release.failed"}),
			Status:    "pending", QueuedAt: time.Now(),
		},
	}
	createPersonalNotificationRunnerDeliveries(t, db, deliveries...)

	var scheduled []tasks.NotificationEmailDigestPayload
	runner := &Runner{
		db: db,
		personalEmailCooldown: func(context.Context, *gorm.DB) (time.Duration, error) {
			return time.Minute, nil
		},
		enqueueEmailDigest: func(_ context.Context, payload tasks.NotificationEmailDigestPayload) (*asynq.TaskInfo, error) {
			scheduled = append(scheduled, payload)
			return &asynq.TaskInfo{}, nil
		},
		personalEmailSender: func(context.Context, string, notification.RenderedMessage) (notification.SendResult, error) {
			t.Fatal("email must not be sent during the per-user cooldown")
			return notification.SendResult{}, nil
		},
	}
	if err := runner.handlePersonalEmailDelivery(t.Context(), deliveries[1]); err != nil {
		t.Fatalf("handle delivery during cooldown: %v", err)
	}
	if len(scheduled) != 1 || scheduled[0].RecipientUserID != user.ID {
		t.Fatalf("scheduled digest payloads = %#v", scheduled)
	}
	wantDueAt := previousStartedAt.Add(time.Minute).Truncate(time.Second)
	if previousStartedAt.Add(time.Minute).Nanosecond() != 0 {
		wantDueAt = wantDueAt.Add(time.Second)
	}
	if got := time.Unix(scheduled[0].DueAtUnix, 0); !got.Equal(wantDueAt) {
		t.Fatalf("digest due at = %s, want %s", got, wantDueAt)
	}
	var pending model.NotificationDelivery
	if err := db.First(&pending, "id = ?", deliveries[1].ID).Error; err != nil {
		t.Fatalf("load pending delivery: %v", err)
	}
	if pending.Status != "pending" || pending.AttemptCount != 0 {
		t.Fatalf("delivery status=%q attempts=%d, want untouched pending delivery", pending.Status, pending.AttemptCount)
	}
}

func TestPersonalEmailDeliverySerializesAndAggregatesUserEventsPostgres(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix:       "personal_notification_digest_concurrency_test",
		MaxOpenConnections: 4,
		Migrate:            migratePersonalNotificationRunnerTestModels,
	})
	user := model.User{ID: "usr_digest_concurrent", Email: "digest@example.com", Name: "Digest", Language: "zh-CN"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	deliveries := []model.NotificationDelivery{
		{
			ID: "ndl_digest_first", RecipientUserID: user.ID, EventID: "evt_digest_first", EventType: "build.failed",
			ChannelID: notification.UserEmailChannelID, AdapterKind: notification.AdapterKindSMTP,
			EventJSON: mustMarshalNotificationEvent(t, notification.Event{ID: "evt_digest_first", Type: "build.failed", Message: "first"}),
			Status:    "pending", QueuedAt: time.Now().Add(-time.Second),
		},
		{
			ID: "ndl_digest_second", RecipientUserID: user.ID, EventID: "evt_digest_second", EventType: "release.failed",
			ChannelID: notification.UserEmailChannelID, AdapterKind: notification.AdapterKindSMTP,
			EventJSON: mustMarshalNotificationEvent(t, notification.Event{ID: "evt_digest_second", Type: "release.failed", Message: "second"}),
			Status:    "pending", QueuedAt: time.Now(),
		},
	}
	createPersonalNotificationRunnerDeliveries(t, db, deliveries...)

	firstSendEntered := make(chan struct{})
	releaseFirstSend := make(chan struct{})
	var releaseFirstSendOnce sync.Once
	t.Cleanup(func() { releaseFirstSendOnce.Do(func() { close(releaseFirstSend) }) })
	var sendMu sync.Mutex
	sendCount := 0
	runner := &Runner{
		db:                    db,
		personalEmailCooldown: func(context.Context, *gorm.DB) (time.Duration, error) { return 0, nil },
		personalEmailSender: func(context.Context, string, notification.RenderedMessage) (notification.SendResult, error) {
			sendMu.Lock()
			sendCount++
			current := sendCount
			sendMu.Unlock()
			if current == 1 {
				close(firstSendEntered)
				<-releaseFirstSend
			}
			return notification.SendResult{StatusCode: 250, ResponseSnippet: "sent"}, nil
		},
	}

	errs := make(chan error, 2)
	go func() { errs <- runner.handlePersonalEmailDelivery(t.Context(), deliveries[0]) }()
	select {
	case <-firstSendEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("first email send did not start")
	}
	go func() { errs <- runner.handlePersonalEmailDelivery(t.Context(), deliveries[1]) }()
	time.Sleep(100 * time.Millisecond)
	sendMu.Lock()
	gotWhileLocked := sendCount
	sendMu.Unlock()
	if gotWhileLocked != 1 {
		t.Fatalf("send count while first user transaction held lock = %d, want 1", gotWhileLocked)
	}
	releaseFirstSendOnce.Do(func() { close(releaseFirstSend) })
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatalf("handle concurrent personal email delivery: %v", err)
		}
	}
	sendMu.Lock()
	gotSendCount := sendCount
	sendMu.Unlock()
	if gotSendCount != 1 {
		t.Fatalf("send count = %d, want one aggregated email", gotSendCount)
	}
	var stored []model.NotificationDelivery
	if err := db.Order("id asc").Find(&stored).Error; err != nil {
		t.Fatalf("load stored deliveries: %v", err)
	}
	if len(stored) != 2 {
		t.Fatalf("stored deliveries = %d, want 2", len(stored))
	}
	for _, delivery := range stored {
		if delivery.Status != "succeeded" || delivery.AttemptCount != 1 {
			t.Fatalf("delivery %s status=%q attempts=%d", delivery.ID, delivery.Status, delivery.AttemptCount)
		}
	}
}

func TestPersonalEmailDeliveryUsesOneRetryLeaderAcrossSiblingTasksPostgres(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "personal_notification_retry_leader_test",
		Migrate:      migratePersonalNotificationRunnerTestModels,
	})
	user := model.User{ID: "usr_retry_leader", Email: "retry-leader@example.com", Name: "Retry Leader", Language: "en-US"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create retry leader user: %v", err)
	}

	queuedAt := time.Now().Add(-time.Minute)
	deliveries := []model.NotificationDelivery{
		{
			ID: "ndl_retry_leader_enqueue_failed", RecipientUserID: user.ID,
			EventID: "evt_retry_leader_enqueue_failed", EventType: "build.failed",
			ChannelID: notification.UserEmailChannelID, AdapterKind: notification.AdapterKindSMTP,
			EventJSON: mustMarshalNotificationEvent(t, notification.Event{
				ID: "evt_retry_leader_enqueue_failed", Type: "build.failed", Message: "enqueue failed event",
			}),
			Status: "enqueue_failed", QueuedAt: queuedAt,
		},
	}
	for index := range 6 {
		eventID := fmt.Sprintf("evt_retry_leader_%d", index)
		deliveries = append(deliveries, model.NotificationDelivery{
			ID:              fmt.Sprintf("ndl_retry_leader_%d", index),
			RecipientUserID: user.ID,
			EventID:         eventID,
			EventType:       "build.failed",
			ChannelID:       notification.UserEmailChannelID,
			AdapterKind:     notification.AdapterKindSMTP,
			EventJSON: mustMarshalNotificationEvent(t, notification.Event{
				ID: eventID, Type: "build.failed", Message: fmt.Sprintf("sibling event %d", index),
			}),
			Status:   "pending",
			QueuedAt: queuedAt.Add(time.Duration(index+1) * time.Millisecond),
		})
	}
	createPersonalNotificationRunnerDeliveries(t, db, deliveries...)

	tasksByDeliveryID := make(map[string]*asynq.Task, len(deliveries)-1)
	for _, delivery := range deliveries[1:] {
		task, err := tasks.NewNotificationDeliverTask(tasks.NotificationDeliverPayload{DeliveryID: delivery.ID})
		if err != nil {
			t.Fatalf("create notification delivery task %s: %v", delivery.ID, err)
		}
		tasksByDeliveryID[delivery.ID] = task
	}

	sendCount := 0
	runner := &Runner{
		db:                    db,
		personalEmailCooldown: func(context.Context, *gorm.DB) (time.Duration, error) { return 0, nil },
		personalEmailSender: func(context.Context, string, notification.RenderedMessage) (notification.SendResult, error) {
			sendCount++
			if sendCount == 1 {
				return notification.SendResult{}, errors.New("temporary SMTP failure")
			}
			return notification.SendResult{StatusCode: 250, ResponseSnippet: "sent"}, nil
		},
	}
	leaderID := deliveries[1].ID
	if err := runner.handleNotificationDeliver(t.Context(), tasksByDeliveryID[leaderID]); err == nil || errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("first leader attempt error = %v, want retryable SMTP failure", err)
	}
	assertPersonalEmailRetryLeaderDeliveries(t, db, deliveries, map[string]string{
		deliveries[0].ID: "enqueue_failed",
	}, "pending", 1)

	for _, sibling := range deliveries[2:] {
		if err := runner.handleNotificationDeliver(t.Context(), tasksByDeliveryID[sibling.ID]); err != nil {
			t.Fatalf("sibling delivery %s returned error: %v", sibling.ID, err)
		}
	}
	if sendCount != 1 {
		t.Fatalf("send count after sibling tasks = %d, want 1", sendCount)
	}
	assertPersonalEmailRetryLeaderDeliveries(t, db, deliveries, map[string]string{
		deliveries[0].ID: "enqueue_failed",
	}, "pending", 1)

	if err := runner.handleNotificationDeliver(t.Context(), tasksByDeliveryID[leaderID]); err != nil {
		t.Fatalf("retry leader delivery: %v", err)
	}
	if sendCount != 2 {
		t.Fatalf("send count after leader retry = %d, want 2", sendCount)
	}
	assertPersonalEmailRetryLeaderDeliveries(t, db, deliveries, nil, "succeeded", 2)
}

func TestSucceededPersonalEmailTaskRetriesRemainderScheduling(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "personal_notification_remainder_retry_test",
		Migrate:      migratePersonalNotificationRunnerTestModels,
	})
	user := model.User{ID: "usr_remainder_retry", Email: "remainder@example.com", Name: "Remainder"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create remainder user: %v", err)
	}
	startedAt := time.Now().Add(-10 * time.Second)
	finishedAt := startedAt.Add(time.Second)
	deliveries := []model.NotificationDelivery{
		{
			ID: "ndl_remainder_trigger", RecipientUserID: "usr_remainder_retry", EventID: "evt_remainder_trigger", EventType: "build.failed",
			ChannelID: notification.UserEmailChannelID, AdapterKind: notification.AdapterKindSMTP,
			EventJSON: mustMarshalNotificationEvent(t, notification.Event{ID: "evt_remainder_trigger", Type: "build.failed"}),
			Status:    "succeeded", StartedAt: &startedAt, FinishedAt: &finishedAt, QueuedAt: startedAt,
		},
		{
			ID: "ndl_remainder_waiting", RecipientUserID: "usr_remainder_retry", EventID: "evt_remainder_waiting", EventType: "release.failed",
			ChannelID: notification.UserEmailChannelID, AdapterKind: notification.AdapterKindSMTP,
			EventJSON: mustMarshalNotificationEvent(t, notification.Event{ID: "evt_remainder_waiting", Type: "release.failed"}),
			Status:    "enqueue_failed", QueuedAt: time.Now(),
		},
	}
	createPersonalNotificationRunnerDeliveries(t, db, deliveries...)
	task, err := tasks.NewNotificationDeliverTask(tasks.NotificationDeliverPayload{DeliveryID: deliveries[0].ID})
	if err != nil {
		t.Fatalf("create notification delivery task: %v", err)
	}

	enqueueCalls := 0
	runner := &Runner{
		db: db,
		personalEmailCooldown: func(context.Context, *gorm.DB) (time.Duration, error) {
			return time.Minute, nil
		},
		enqueueEmailDigest: func(_ context.Context, payload tasks.NotificationEmailDigestPayload) (*asynq.TaskInfo, error) {
			enqueueCalls++
			if payload.RecipientUserID != deliveries[0].RecipientUserID {
				t.Fatalf("digest recipient = %q", payload.RecipientUserID)
			}
			if enqueueCalls == 1 {
				return nil, errors.New("temporary queue failure")
			}
			return &asynq.TaskInfo{}, nil
		},
		personalEmailSender: func(context.Context, string, notification.RenderedMessage) (notification.SendResult, error) {
			t.Fatal("remainder must stay inside the current cooldown")
			return notification.SendResult{}, nil
		},
	}
	if err := runner.handleNotificationDeliver(t.Context(), task); err == nil {
		t.Fatal("first remainder scheduling unexpectedly succeeded")
	}
	if err := runner.handleNotificationDeliver(t.Context(), task); err != nil {
		t.Fatalf("retry remainder scheduling: %v", err)
	}
	if enqueueCalls != 2 {
		t.Fatalf("remainder enqueue calls = %d, want 2", enqueueCalls)
	}
}

func TestPersonalEmailDeliverySchedulesEnqueueFailedEventsBeyondDigestLimit(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "personal_notification_digest_limit_test",
		Migrate:      migratePersonalNotificationRunnerTestModels,
	})
	user := model.User{ID: "usr_digest_limit", Email: "digest-limit@example.com", Name: "Digest Limit", Language: "en-US"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	queuedAt := time.Now().Add(-time.Minute)
	deliveries := make([]model.NotificationDelivery, 0, personalEmailDigestMaxEvents+1)
	for index := range personalEmailDigestMaxEvents + 1 {
		eventID := fmt.Sprintf("evt_digest_limit_%02d", index)
		status := "pending"
		if index == personalEmailDigestMaxEvents {
			status = "enqueue_failed"
		}
		deliveries = append(deliveries, model.NotificationDelivery{
			ID:              fmt.Sprintf("ndl_digest_limit_%02d", index),
			RecipientUserID: user.ID,
			EventID:         eventID,
			EventType:       "build.failed",
			ChannelID:       notification.UserEmailChannelID,
			AdapterKind:     notification.AdapterKindSMTP,
			EventJSON: mustMarshalNotificationEvent(t, notification.Event{
				ID: eventID, Type: "build.failed", Message: fmt.Sprintf("failure %d", index),
			}),
			Status:   status,
			QueuedAt: queuedAt.Add(time.Duration(index) * time.Millisecond),
		})
	}
	createPersonalNotificationRunnerDeliveries(t, db, deliveries...)

	var scheduled []tasks.NotificationEmailDigestPayload
	sendCount := 0
	runner := &Runner{
		db: db,
		personalEmailCooldown: func(context.Context, *gorm.DB) (time.Duration, error) {
			return time.Minute, nil
		},
		enqueueEmailDigest: func(_ context.Context, payload tasks.NotificationEmailDigestPayload) (*asynq.TaskInfo, error) {
			scheduled = append(scheduled, payload)
			return &asynq.TaskInfo{}, nil
		},
		personalEmailSender: func(context.Context, string, notification.RenderedMessage) (notification.SendResult, error) {
			sendCount++
			return notification.SendResult{StatusCode: 250, ResponseSnippet: "sent"}, nil
		},
	}
	if err := runner.handlePersonalEmailDelivery(t.Context(), deliveries[0]); err != nil {
		t.Fatalf("send first bounded digest: %v", err)
	}
	if sendCount != 1 {
		t.Fatalf("first digest send count = %d, want 1", sendCount)
	}
	var succeeded int64
	if err := db.Model(&model.NotificationDelivery{}).Where("status = ?", "succeeded").Count(&succeeded).Error; err != nil {
		t.Fatalf("count succeeded deliveries: %v", err)
	}
	if succeeded != personalEmailDigestMaxEvents {
		t.Fatalf("succeeded deliveries = %d, want %d", succeeded, personalEmailDigestMaxEvents)
	}
	if sendCount != 1 || len(scheduled) != 1 || scheduled[0].RecipientUserID != user.ID {
		t.Fatalf("send count=%d scheduled=%#v, want one later user digest", sendCount, scheduled)
	}
	var remaining model.NotificationDelivery
	if err := db.First(&remaining, "id = ?", deliveries[len(deliveries)-1].ID).Error; err != nil {
		t.Fatalf("load remaining delivery: %v", err)
	}
	if remaining.Status != "enqueue_failed" || remaining.AttemptCount != 0 {
		t.Fatalf("remaining delivery status=%q attempts=%d", remaining.Status, remaining.AttemptCount)
	}
}

func TestPersonalEmailFailureDeliveryIDsUsePersistentAttempts(t *testing.T) {
	maxAttempts := tasks.PolicyForType(tasks.TypeNotificationEmailDigest).MaxRetry + 1
	failed, pending, enqueueFailed := personalEmailFailureDeliveryIDs([]model.NotificationDelivery{
		{ID: "ndl_exhausted", AttemptCount: maxAttempts - 1},
		{ID: "ndl_retryable", AttemptCount: maxAttempts - 2},
		{ID: "ndl_enqueue_failed", Status: "enqueue_failed", AttemptCount: maxAttempts - 2},
	})
	if len(failed) != 1 || failed[0] != "ndl_exhausted" {
		t.Fatalf("failed delivery ids = %#v", failed)
	}
	if len(pending) != 1 || pending[0] != "ndl_retryable" {
		t.Fatalf("pending delivery ids = %#v", pending)
	}
	if len(enqueueFailed) != 1 || enqueueFailed[0] != "ndl_enqueue_failed" {
		t.Fatalf("enqueue-failed delivery ids = %#v", enqueueFailed)
	}
}

func TestPersonalEmailDeliveryStopsAtPersistentAttemptLimit(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "personal_notification_attempt_limit_test",
		Migrate:      migratePersonalNotificationRunnerTestModels,
	})
	user := model.User{ID: "usr_attempt_limit", Email: "attempt-limit@example.com", Name: "Attempt Limit"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	maxAttempts := tasks.PolicyForType(tasks.TypeNotificationEmailDigest).MaxRetry + 1
	event := notification.Event{ID: "evt_attempt_limit", Type: "build.failed", Message: "failed"}
	delivery := model.NotificationDelivery{
		ID:              "ndl_attempt_limit",
		RecipientUserID: user.ID,
		EventID:         event.ID,
		EventType:       event.Type,
		ChannelID:       notification.UserEmailChannelID,
		AdapterKind:     notification.AdapterKindSMTP,
		EventJSON:       mustMarshalNotificationEvent(t, event),
		Status:          "pending",
		AttemptCount:    maxAttempts - 1,
		QueuedAt:        time.Now(),
	}
	createPersonalNotificationRunnerDeliveries(t, db, delivery)
	runner := &Runner{
		db:                    db,
		personalEmailCooldown: func(context.Context, *gorm.DB) (time.Duration, error) { return time.Hour, nil },
		personalEmailSender: func(context.Context, string, notification.RenderedMessage) (notification.SendResult, error) {
			return notification.SendResult{}, errors.New("temporary SMTP failure")
		},
	}
	err := runner.handlePersonalEmailDelivery(t.Context(), delivery)
	if !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("attempt-limit delivery error = %v, want SkipRetry", err)
	}
	var stored model.NotificationDelivery
	if err := db.First(&stored, "id = ?", delivery.ID).Error; err != nil {
		t.Fatalf("load attempt-limit delivery: %v", err)
	}
	if stored.Status != "failed" || stored.AttemptCount != maxAttempts {
		t.Fatalf("attempt-limit delivery status=%q attempts=%d, want failed/%d", stored.Status, stored.AttemptCount, maxAttempts)
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

func migratePersonalNotificationRunnerTestModels(db *gorm.DB) error {
	return db.AutoMigrate(
		&model.User{},
		&model.PlatformEvent{},
		&model.UserNotificationPreference{},
		&model.NotificationDelivery{},
	)
}

func createPersonalNotificationRunnerDeliveries(t *testing.T, db *gorm.DB, deliveries ...model.NotificationDelivery) {
	t.Helper()
	for index := range deliveries {
		var event notification.Event
		if err := json.Unmarshal([]byte(deliveries[index].EventJSON), &event); err != nil {
			t.Fatalf("decode delivery event fixture: %v", err)
		}
		event.ID = deliveries[index].EventID
		event.Type = deliveries[index].EventType
		event.Project.ID = deliveries[index].ProjectID
		if event.Actor.ID == "" {
			event.Actor.ID = deliveries[index].RecipientUserID
		}
		deliveries[index].EventJSON = mustMarshalNotificationEvent(t, event)
		storedEvent := platformevent.NewRecord(platformevent.RecordInput{
			ID:                  event.ID,
			Type:                event.Type,
			Severity:            firstNonEmpty(event.Severity, deliveries[index].Severity),
			ProjectID:           event.Project.ID,
			ActorID:             event.Actor.ID,
			ResourceOwnerUserID: event.ResourceOwnerUserID,
			Detail:              event,
		})
		if err := db.Create(&storedEvent).Error; err != nil {
			t.Fatalf("create authoritative platform event: %v", err)
		}
	}
	if err := db.Create(&deliveries).Error; err != nil {
		t.Fatalf("create deliveries: %v", err)
	}
}

func assertPersonalEmailRetryLeaderDeliveries(
	t *testing.T,
	db *gorm.DB,
	deliveries []model.NotificationDelivery,
	statusOverrides map[string]string,
	defaultStatus string,
	wantAttemptCount int,
) {
	t.Helper()
	var stored []model.NotificationDelivery
	if err := db.Order("queued_at asc, id asc").Find(&stored).Error; err != nil {
		t.Fatalf("load retry leader deliveries: %v", err)
	}
	if len(stored) != len(deliveries) {
		t.Fatalf("stored retry leader deliveries = %d, want %d", len(stored), len(deliveries))
	}
	for _, delivery := range stored {
		wantStatus := defaultStatus
		if override, ok := statusOverrides[delivery.ID]; ok {
			wantStatus = override
		}
		if delivery.Status != wantStatus || delivery.AttemptCount != wantAttemptCount {
			t.Fatalf(
				"delivery %s status=%q attempts=%d, want %q/%d",
				delivery.ID,
				delivery.Status,
				delivery.AttemptCount,
				wantStatus,
				wantAttemptCount,
			)
		}
	}
}
