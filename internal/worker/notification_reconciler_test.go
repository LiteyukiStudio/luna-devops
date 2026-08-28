package worker

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
	"github.com/LiteyukiStudio/devops/internal/platformevent"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"github.com/hibiken/asynq"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

type recordingNotificationReconcileEnqueuer struct {
	deliveryIDs []string
	errors      []error
	traceIDs    []trace.TraceID
}

func (e *recordingNotificationReconcileEnqueuer) EnqueueNotificationDeliver(
	ctx context.Context,
	payload tasks.NotificationDeliverPayload,
) (*asynq.TaskInfo, error) {
	e.deliveryIDs = append(e.deliveryIDs, payload.DeliveryID)
	e.traceIDs = append(e.traceIDs, trace.SpanContextFromContext(ctx).TraceID())
	if len(e.errors) == 0 {
		return &asynq.TaskInfo{}, nil
	}
	err := e.errors[0]
	e.errors = e.errors[1:]
	if err != nil {
		return nil, err
	}
	return &asynq.TaskInfo{}, nil
}

func TestNotificationReconcileRequeuesStalePendingAndEnqueueFailed(t *testing.T) {
	db := openNotificationReconcilerTestDB(t, "notification_reconcile_requeue_test")
	now := time.Now().UTC().Truncate(time.Microsecond)
	deliveryTimeout := tasks.PolicyForType(tasks.TypeNotificationDeliver).Timeout
	staleStartedAt := now.Add(-deliveryTimeout - time.Minute)
	recentStartedAt := now.Add(-time.Minute)
	deliveries := []model.NotificationDelivery{
		newNotificationReconcileDelivery("ndl_enqueue_failed", "enqueue_failed", now.Add(-10*time.Minute), now, "legacy queue detail"),
		newNotificationReconcileDelivery("ndl_stale_pending", "pending", now.Add(-9*time.Minute), now.Add(-notificationReconcilePendingAge-time.Minute), ""),
		newNotificationReconcileDelivery("ndl_stale_retry_pending", notificationDeliveryRetryPendingStatus, now.Add(-17*time.Minute/2), now.Add(-notificationReconcilePendingAge-time.Minute), "temporary send failure"),
		newNotificationReconcileDelivery("ndl_stale_sending", "sending", now.Add(-8*time.Minute), now, "incomplete attempt"),
		newNotificationReconcileDelivery("ndl_null_stale_sending", "sending", now.Add(-7*time.Minute), now.Add(-deliveryTimeout-time.Minute), "incomplete attempt"),
		newNotificationReconcileDelivery("ndl_fresh_pending", "pending", now.Add(-6*time.Minute), now.Add(-notificationReconcilePendingAge+time.Minute), ""),
		newNotificationReconcileDelivery("ndl_fresh_retry_pending", notificationDeliveryRetryPendingStatus, now.Add(-11*time.Minute/2), now.Add(-notificationReconcilePendingAge+time.Minute), "temporary send failure"),
		newNotificationReconcileDelivery("ndl_recent_sending", "sending", now.Add(-5*time.Minute), now, "active attempt"),
		newNotificationReconcileDelivery("ndl_null_recent_sending", "sending", now.Add(-4*time.Minute), now.Add(-time.Minute), "active attempt"),
		newNotificationReconcileDelivery("ndl_succeeded", "succeeded", now.Add(-3*time.Minute), now.Add(-time.Hour), ""),
	}
	deliveries[2].AttemptCount = 1
	deliveries[3].StartedAt = &staleStartedAt
	deliveries[3].AttemptCount = 1
	deliveries[4].AttemptCount = 2
	deliveries[6].AttemptCount = 1
	deliveries[7].StartedAt = &recentStartedAt
	deliveries[7].AttemptCount = 1
	deliveries[8].AttemptCount = 2
	if err := db.Create(&deliveries).Error; err != nil {
		t.Fatalf("create reconcile deliveries: %v", err)
	}

	enqueuer := &recordingNotificationReconcileEnqueuer{errors: []error{asynq.ErrDuplicateTask, nil}}
	runner := &Runner{db: db, notificationDeliveryEnqueuer: enqueuer}
	if err := runner.reconcileNotificationDeliveries(t.Context(), now); err != nil {
		t.Fatalf("reconcile notification deliveries: %v", err)
	}
	if got := strings.Join(enqueuer.deliveryIDs, ","); got != "ndl_enqueue_failed,ndl_stale_pending,ndl_stale_retry_pending,ndl_stale_sending,ndl_null_stale_sending" {
		t.Fatalf("enqueued deliveries = %q", got)
	}

	stored := loadNotificationReconcileDeliveries(t, db)
	for _, id := range []string{"ndl_enqueue_failed", "ndl_stale_pending", "ndl_stale_retry_pending", "ndl_stale_sending", "ndl_null_stale_sending"} {
		if stored[id].Status != "pending" || stored[id].ErrorMessage != "" || stored[id].StartedAt != nil {
			t.Fatalf("reconciled delivery %s = status %q error %q", id, stored[id].Status, stored[id].ErrorMessage)
		}
	}
	if stored["ndl_stale_retry_pending"].AttemptCount != 1 || stored["ndl_stale_sending"].AttemptCount != 1 || stored["ndl_null_stale_sending"].AttemptCount != 2 {
		t.Fatalf("reconciler changed lease generations: retry=%d stale=%d null-stale=%d", stored["ndl_stale_retry_pending"].AttemptCount, stored["ndl_stale_sending"].AttemptCount, stored["ndl_null_stale_sending"].AttemptCount)
	}
	if stored["ndl_fresh_pending"].Status != "pending" || stored["ndl_fresh_retry_pending"].Status != notificationDeliveryRetryPendingStatus || stored["ndl_recent_sending"].Status != "sending" ||
		stored["ndl_null_recent_sending"].Status != "sending" || stored["ndl_succeeded"].Status != "succeeded" {
		t.Fatalf("unrelated delivery states changed: %#v", stored)
	}
}

func TestNotificationReconcileContinuesAfterIndividualEnqueueFailures(t *testing.T) {
	db := openNotificationReconcilerTestDB(t, "notification_reconcile_partial_failure_test")
	now := time.Now().UTC().Truncate(time.Microsecond)
	deliveries := []model.NotificationDelivery{
		newNotificationReconcileDelivery("ndl_first", "enqueue_failed", now.Add(-3*time.Minute), now, "old first detail"),
		newNotificationReconcileDelivery("ndl_second", "enqueue_failed", now.Add(-2*time.Minute), now, "old second detail"),
		newNotificationReconcileDelivery("ndl_third", "enqueue_failed", now.Add(-time.Minute), now, "old third detail"),
	}
	if err := db.Create(&deliveries).Error; err != nil {
		t.Fatalf("create partial failure deliveries: %v", err)
	}

	firstErr := errors.New("first queue failure with private detail")
	thirdErr := errors.New("third queue failure with private detail")
	enqueuer := &recordingNotificationReconcileEnqueuer{errors: []error{firstErr, asynq.ErrDuplicateTask, thirdErr}}
	runner := &Runner{db: db, notificationDeliveryEnqueuer: enqueuer}
	err := runner.reconcileNotificationDeliveries(t.Context(), now)
	if !errors.Is(err, firstErr) || !errors.Is(err, thirdErr) {
		t.Fatalf("reconcile error = %v, want both enqueue failures", err)
	}
	if got := strings.Join(enqueuer.deliveryIDs, ","); got != "ndl_first,ndl_second,ndl_third" {
		t.Fatalf("enqueued deliveries = %q, want all candidates in stable order", got)
	}

	stored := loadNotificationReconcileDeliveries(t, db)
	for _, id := range []string{"ndl_first", "ndl_third"} {
		if stored[id].Status != "enqueue_failed" || stored[id].ErrorMessage != notification.DeliveryEnqueueFailedCode {
			t.Fatalf("failed delivery %s = status %q error %q", id, stored[id].Status, stored[id].ErrorMessage)
		}
		if stored[id].QueuedAt.Before(now) {
			t.Fatalf("failed delivery %s queued at %s, want refreshed after %s", id, stored[id].QueuedAt, now)
		}
		if strings.Contains(stored[id].ErrorMessage, "private detail") {
			t.Fatalf("failed delivery %s persisted private queue detail", id)
		}
	}
	if stored["ndl_second"].Status != "pending" || stored["ndl_second"].ErrorMessage != "" {
		t.Fatalf("duplicate delivery = status %q error %q", stored["ndl_second"].Status, stored["ndl_second"].ErrorMessage)
	}
}

func TestNotificationReconcileWithoutEnqueuerFailsClosed(t *testing.T) {
	db := openNotificationReconcilerTestDB(t, "notification_reconcile_missing_queue_test")
	now := time.Now().UTC().Truncate(time.Microsecond)
	delivery := newNotificationReconcileDelivery("ndl_no_queue", "enqueue_failed", now.Add(-time.Minute), now, "existing stable error")
	if err := db.Create(&delivery).Error; err != nil {
		t.Fatalf("create missing queue delivery: %v", err)
	}

	err := (&Runner{db: db}).reconcileNotificationDeliveries(t.Context(), now)
	if err == nil || !strings.Contains(err.Error(), "queue is unavailable") {
		t.Fatalf("missing enqueuer error = %v", err)
	}
	var stored model.NotificationDelivery
	if err := db.First(&stored, "id = ?", delivery.ID).Error; err != nil {
		t.Fatalf("load delivery after fail-closed reconcile: %v", err)
	}
	if stored.Status != delivery.Status || stored.ErrorMessage != delivery.ErrorMessage {
		t.Fatalf("delivery changed without an enqueuer: %#v", stored)
	}
}

func TestNotificationReconcileHandlerIsRegistered(t *testing.T) {
	task, err := tasks.NewNotificationReconcileTask(tasks.NotificationReconcilePayload{})
	if err != nil {
		t.Fatalf("create notification reconcile task: %v", err)
	}
	mux := asynq.NewServeMux()
	registerTaskHandlers(mux, &Runner{})
	err = mux.ProcessTask(t.Context(), task)
	if err == nil || !strings.Contains(err.Error(), "queue is unavailable") {
		t.Fatalf("registered reconcile handler error = %v", err)
	}
}

func TestNotificationReconcilePropagatesTaskTraceToDeliveryEnqueue(t *testing.T) {
	db := openNotificationReconcilerTestDB(t, "notification_reconcile_trace_test")
	now := time.Now().UTC().Truncate(time.Microsecond)
	delivery := newNotificationReconcileDelivery("ndl_reconcile_trace", "enqueue_failed", now.Add(-time.Minute), now, "old error")
	if err := db.Create(&delivery).Error; err != nil {
		t.Fatalf("create trace delivery: %v", err)
	}

	previousProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
		_ = provider.Shutdown(context.Background())
	})

	producerCtx, producerSpan := provider.Tracer("notification-reconcile-test").Start(t.Context(), "producer")
	wantTraceID := producerSpan.SpanContext().TraceID()
	headers := telemetry.InjectMap(producerCtx)
	producerSpan.End()
	task, err := tasks.NewNotificationReconcileTask(tasks.NotificationReconcilePayload{})
	if err != nil {
		t.Fatalf("create notification reconcile task: %v", err)
	}
	task = asynq.NewTaskWithHeaders(task.Type(), task.Payload(), headers)

	enqueuer := &recordingNotificationReconcileEnqueuer{}
	mux := asynq.NewServeMux()
	mux.Use(taskTelemetryMiddleware)
	registerTaskHandlers(mux, &Runner{db: db, notificationDeliveryEnqueuer: enqueuer})
	if err := mux.ProcessTask(t.Context(), task); err != nil {
		t.Fatalf("process traced notification reconcile task: %v", err)
	}
	if len(enqueuer.traceIDs) != 1 || enqueuer.traceIDs[0] != wantTraceID {
		t.Fatalf("enqueue trace IDs = %#v, want %s", enqueuer.traceIDs, wantTraceID)
	}
}

func TestNotificationReconcileMaterializesPendingEventWithOriginalTrace(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix: "notification_reconcile_fanout_trace_test",
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
	user := model.User{ID: "usr_fanout_trace", Email: "fanout-trace@example.com", Name: "Fanout Trace"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create fanout trace user: %v", err)
	}

	previousProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
		_ = provider.Shutdown(context.Background())
	})

	producerCtx, producerSpan := provider.Tracer("notification-fanout-reconcile-test").Start(t.Context(), "original-operation")
	wantTraceID := producerSpan.SpanContext().TraceID()
	headers := telemetry.InjectMap(producerCtx)
	producerSpan.End()
	event := notification.Event{
		ID: "evt_fanout_trace", Type: "build.failed", Severity: notification.SeverityError,
		Actor: notification.ActorContext{ID: user.ID}, OccurredAt: time.Now(), Message: "build failed",
	}
	storedEvent, created, err := (platformevent.Service{DB: db}).RecordNotification(t.Context(), platformevent.RecordInput{
		ID: event.ID, Type: event.Type, Severity: event.Severity, ActorID: user.ID,
		Message: event.Message, Detail: event, TraceID: wantTraceID.String(), OccurredAt: event.OccurredAt,
	}, headers["traceparent"], headers["tracestate"])
	if err != nil || !created {
		t.Fatalf("record pending notification event created=%v err=%v", created, err)
	}
	if storedEvent.NotificationFanoutStatus != notification.NotificationFanoutStatusPending {
		t.Fatalf("pending event status = %q", storedEvent.NotificationFanoutStatus)
	}

	enqueuer := &recordingNotificationReconcileEnqueuer{}
	runner := &Runner{db: db, notificationDeliveryEnqueuer: enqueuer}
	if err := runner.reconcileNotificationDeliveries(t.Context(), time.Now()); err != nil {
		t.Fatalf("reconcile pending notification event: %v", err)
	}
	if len(enqueuer.deliveryIDs) != 1 || len(enqueuer.traceIDs) != 1 || enqueuer.traceIDs[0] != wantTraceID {
		t.Fatalf("fanout enqueue ids=%#v traces=%#v, want trace %s", enqueuer.deliveryIDs, enqueuer.traceIDs, wantTraceID)
	}
	if err := db.First(&storedEvent, "id = ?", event.ID).Error; err != nil {
		t.Fatalf("reload reconciled event: %v", err)
	}
	if storedEvent.NotificationFanoutStatus != notification.NotificationFanoutStatusCompleted {
		t.Fatalf("reconciled event status = %q, want completed", storedEvent.NotificationFanoutStatus)
	}
	var delivery model.NotificationDelivery
	if err := db.First(&delivery, "event_id = ?", event.ID).Error; err != nil {
		t.Fatalf("load materialized delivery: %v", err)
	}
	if delivery.RecipientUserID != user.ID || delivery.ChannelID != notification.UserEmailChannelID || delivery.Traceparent != headers["traceparent"] {
		t.Fatalf("materialized delivery = %#v", delivery)
	}
}

func TestRecentSendingNotificationDeliveryReturnsBeforeValidation(t *testing.T) {
	db := openNotificationReconcilerTestDB(t, "notification_delivery_recent_sending_test")
	now := time.Now().UTC().Truncate(time.Microsecond)
	delivery := newNotificationReconcileDelivery("ndl_recent_sending_noop", "sending", now.Add(-time.Minute), now, "")
	delivery.StartedAt = &now
	if err := db.Create(&delivery).Error; err != nil {
		t.Fatalf("create recent sending delivery: %v", err)
	}
	task, err := tasks.NewNotificationDeliverTask(tasks.NotificationDeliverPayload{DeliveryID: delivery.ID})
	if err != nil {
		t.Fatalf("create notification delivery task: %v", err)
	}
	if err := (&Runner{db: db}).handleNotificationDeliver(t.Context(), task); err != nil {
		t.Fatalf("recent sending delivery returned error before no-op: %v", err)
	}
	var stored model.NotificationDelivery
	if err := db.First(&stored, "id = ?", delivery.ID).Error; err != nil {
		t.Fatalf("load recent sending delivery: %v", err)
	}
	if stored.Status != "sending" || stored.AttemptCount != 0 || stored.StartedAt == nil || !stored.StartedAt.Equal(now) {
		t.Fatalf("recent sending delivery changed during duplicate task: %#v", stored)
	}
}

func TestNonEmailNotificationClaimsBeforeChannelFailure(t *testing.T) {
	db := openNotificationReconcilerTestDB(t, "notification_delivery_claim_before_validation_test")
	now := time.Now().UTC().Truncate(time.Microsecond)
	delivery := newNotificationReconcileDelivery("ndl_claim_before_channel", "pending", now, now, "")
	if err := db.Create(&delivery).Error; err != nil {
		t.Fatalf("create delivery with missing channel: %v", err)
	}
	task, err := tasks.NewNotificationDeliverTask(tasks.NotificationDeliverPayload{DeliveryID: delivery.ID})
	if err != nil {
		t.Fatalf("create notification delivery task: %v", err)
	}
	if err := (&Runner{db: db}).handleNotificationDeliver(t.Context(), task); !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("missing channel delivery error = %v, want SkipRetry", err)
	}
	var stored model.NotificationDelivery
	if err := db.First(&stored, "id = ?", delivery.ID).Error; err != nil {
		t.Fatalf("load failed delivery: %v", err)
	}
	if stored.Status != "failed" || stored.AttemptCount != 1 {
		t.Fatalf("delivery status=%q attempt=%d, want failed/1 after pre-validation claim", stored.Status, stored.AttemptCount)
	}
}

func TestNonEmailNotificationLeavesLeaseSendingOnChannelDatabaseError(t *testing.T) {
	db := openNotificationReconcilerTestDB(t, "notification_delivery_channel_database_error_test")
	now := time.Now().UTC().Truncate(time.Microsecond)
	delivery := newNotificationReconcileDelivery("ndl_channel_database_error", "pending", now, now, "")
	if err := db.Create(&delivery).Error; err != nil {
		t.Fatalf("create channel database error delivery: %v", err)
	}
	wantErr := errors.New("injected notification channel database error")
	if err := db.Callback().Query().Before("gorm:query").Register("test:fail_notification_channel_query", func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Table == "notification_channels" {
			tx.AddError(wantErr)
		}
	}); err != nil {
		t.Fatalf("register notification channel query failure: %v", err)
	}
	task, err := tasks.NewNotificationDeliverTask(tasks.NotificationDeliverPayload{DeliveryID: delivery.ID})
	if err != nil {
		t.Fatalf("create notification delivery task: %v", err)
	}
	if err := (&Runner{db: db}).handleNotificationDeliver(t.Context(), task); !errors.Is(err, wantErr) {
		t.Fatalf("channel database error = %v, want injected error", err)
	}
	var stored model.NotificationDelivery
	if err := db.First(&stored, "id = ?", delivery.ID).Error; err != nil {
		t.Fatalf("load delivery after channel database error: %v", err)
	}
	if stored.Status != "sending" || stored.AttemptCount != 1 || stored.ErrorMessage != "" {
		t.Fatalf("delivery after channel database error = %#v", stored)
	}
}

func TestNotificationDeliverySendFailureSeparatesRetryableAndTerminalStates(t *testing.T) {
	db := openNotificationReconcilerTestDB(t, "notification_delivery_send_failure_state_test")
	now := time.Now().UTC().Truncate(time.Microsecond)
	deliveries := []model.NotificationDelivery{
		newNotificationReconcileDelivery("ndl_transient_send_failure", "sending", now, now, ""),
		newNotificationReconcileDelivery("ndl_terminal_send_failure", "sending", now.Add(time.Second), now, ""),
	}
	for index := range deliveries {
		deliveries[index].AttemptCount = 1
		deliveries[index].StartedAt = &now
	}
	if err := db.Create(&deliveries).Error; err != nil {
		t.Fatalf("create send failure deliveries: %v", err)
	}

	transientErr := errors.New("temporary upstream failure")
	skipRetry, err := markNotificationDeliveryLeaseSendFailure(
		db,
		notificationDeliveryLease{DeliveryID: deliveries[0].ID, Generation: 1},
		notification.SendResult{StatusCode: 503, ResponseSnippet: "temporary"},
		transientErr,
		transientErr,
		time.Second,
		`{"method":"POST"}`,
		"temporary",
	)
	if err != nil || skipRetry {
		t.Fatalf("transient send failure skipRetry=%v error=%v", skipRetry, err)
	}

	terminalErr := errors.New("webhook returned status 400")
	skipRetry, err = markNotificationDeliveryLeaseSendFailure(
		db,
		notificationDeliveryLease{DeliveryID: deliveries[1].ID, Generation: 1},
		notification.SendResult{StatusCode: 400, ResponseSnippet: "invalid"},
		terminalErr,
		terminalErr,
		time.Second,
		`{"method":"POST"}`,
		"invalid",
	)
	if err != nil || !skipRetry {
		t.Fatalf("terminal send failure skipRetry=%v error=%v", skipRetry, err)
	}

	stored := loadNotificationReconcileDeliveries(t, db)
	transient := stored[deliveries[0].ID]
	if transient.Status != notificationDeliveryRetryPendingStatus || transient.FinishedAt != nil || transient.ErrorMessage != transientErr.Error() {
		t.Fatalf("transient send failure delivery = %#v", transient)
	}
	terminal := stored[deliveries[1].ID]
	if terminal.Status != "failed" || terminal.FinishedAt == nil || terminal.ErrorMessage != terminalErr.Error() {
		t.Fatalf("terminal send failure delivery = %#v", terminal)
	}
}

func TestClaimNotificationDeliveryUsesReadStatusAndOnlyRecoversStaleSending(t *testing.T) {
	db := openNotificationReconcilerTestDB(t, "notification_delivery_claim_test")
	now := time.Now().UTC().Truncate(time.Microsecond)
	taskTimeout := tasks.PolicyForType(tasks.TypeNotificationDeliver).Timeout
	staleStartedAt := now.Add(-taskTimeout - time.Minute)
	recentStartedAt := now.Add(-time.Minute)
	deliveries := []model.NotificationDelivery{
		newNotificationReconcileDelivery("ndl_claim_pending", "pending", now.Add(-time.Hour), now.Add(-time.Hour), ""),
		newNotificationReconcileDelivery("ndl_claim_raced", "pending", now.Add(-time.Hour), now.Add(-time.Hour), ""),
		newNotificationReconcileDelivery("ndl_claim_stale_sending", "sending", now.Add(-time.Hour), now.Add(-time.Hour), ""),
		newNotificationReconcileDelivery("ndl_claim_recent_sending", "sending", now.Add(-time.Hour), now.Add(-time.Hour), ""),
		newNotificationReconcileDelivery("ndl_claim_retry_pending", notificationDeliveryRetryPendingStatus, now.Add(-time.Hour), now.Add(-time.Hour), "temporary failure"),
		newNotificationReconcileDelivery("ndl_claim_terminal_failed", "failed", now.Add(-time.Hour), now.Add(-time.Hour), "permanent failure"),
	}
	deliveries[2].StartedAt = &staleStartedAt
	deliveries[2].AttemptCount = 1
	deliveries[3].StartedAt = &recentStartedAt
	deliveries[3].AttemptCount = 1
	deliveries[4].AttemptCount = 1
	deliveries[5].AttemptCount = 1
	if err := db.Create(&deliveries).Error; err != nil {
		t.Fatalf("create claim deliveries: %v", err)
	}

	var pending model.NotificationDelivery
	if err := db.First(&pending, "id = ?", "ndl_claim_pending").Error; err != nil {
		t.Fatalf("load pending delivery: %v", err)
	}
	lease, claimed, err := claimNotificationDelivery(db, pending, now)
	if err != nil || !claimed {
		t.Fatalf("first pending claim = %v, %v", claimed, err)
	}
	if lease.Generation != 1 {
		t.Fatalf("first pending lease generation = %d, want 1", lease.Generation)
	}
	_, claimed, err = claimNotificationDelivery(db, pending, now.Add(time.Second))
	if err != nil || claimed {
		t.Fatalf("duplicate claim from the same read state = %v, %v", claimed, err)
	}

	var raced model.NotificationDelivery
	if err := db.First(&raced, "id = ?", "ndl_claim_raced").Error; err != nil {
		t.Fatalf("load raced delivery: %v", err)
	}
	if err := db.Model(&model.NotificationDelivery{}).Where("id = ?", raced.ID).Update("status", "failed").Error; err != nil {
		t.Fatalf("advance raced delivery state: %v", err)
	}
	_, claimed, err = claimNotificationDelivery(db, raced, now)
	if err != nil || claimed {
		t.Fatalf("claim after original read state changed = %v, %v", claimed, err)
	}

	var stale model.NotificationDelivery
	if err := db.First(&stale, "id = ?", "ndl_claim_stale_sending").Error; err != nil {
		t.Fatalf("load stale sending delivery: %v", err)
	}
	lease, claimed, err = claimNotificationDelivery(db, stale, now)
	if err != nil || !claimed {
		t.Fatalf("stale sending claim = %v, %v", claimed, err)
	}
	if lease.Generation != 2 {
		t.Fatalf("stale sending lease generation = %d, want 2", lease.Generation)
	}

	var recent model.NotificationDelivery
	if err := db.First(&recent, "id = ?", "ndl_claim_recent_sending").Error; err != nil {
		t.Fatalf("load recent sending delivery: %v", err)
	}
	_, claimed, err = claimNotificationDelivery(db, recent, now)
	if err != nil || claimed {
		t.Fatalf("recent sending claim = %v, %v", claimed, err)
	}

	var retryPending model.NotificationDelivery
	if err := db.First(&retryPending, "id = ?", "ndl_claim_retry_pending").Error; err != nil {
		t.Fatalf("load retry-pending delivery: %v", err)
	}
	lease, claimed, err = claimNotificationDelivery(db, retryPending, now)
	if err != nil || !claimed || lease.Generation != 2 {
		t.Fatalf("retry-pending claim lease=%#v claimed=%v error=%v", lease, claimed, err)
	}

	var terminalFailed model.NotificationDelivery
	if err := db.First(&terminalFailed, "id = ?", "ndl_claim_terminal_failed").Error; err != nil {
		t.Fatalf("load terminal failed delivery: %v", err)
	}
	_, claimed, err = claimNotificationDelivery(db, terminalFailed, now)
	if err != nil || claimed {
		t.Fatalf("terminal failed claim = %v, %v", claimed, err)
	}

	stored := loadNotificationReconcileDeliveries(t, db)
	if stored["ndl_claim_pending"].Status != "sending" || stored["ndl_claim_pending"].AttemptCount != 1 {
		t.Fatalf("claimed pending delivery = %#v", stored["ndl_claim_pending"])
	}
	if stored["ndl_claim_raced"].Status != "failed" || stored["ndl_claim_raced"].AttemptCount != 0 {
		t.Fatalf("raced delivery was overwritten = %#v", stored["ndl_claim_raced"])
	}
	if stored["ndl_claim_stale_sending"].Status != "sending" || stored["ndl_claim_stale_sending"].AttemptCount != 2 ||
		stored["ndl_claim_stale_sending"].StartedAt == nil || !stored["ndl_claim_stale_sending"].StartedAt.Equal(now) {
		t.Fatalf("recovered stale sending delivery = %#v", stored["ndl_claim_stale_sending"])
	}
	if stored["ndl_claim_recent_sending"].AttemptCount != 1 || stored["ndl_claim_recent_sending"].StartedAt == nil ||
		!stored["ndl_claim_recent_sending"].StartedAt.Equal(recentStartedAt) {
		t.Fatalf("recent sending delivery changed = %#v", stored["ndl_claim_recent_sending"])
	}
	if stored["ndl_claim_retry_pending"].Status != "sending" || stored["ndl_claim_retry_pending"].AttemptCount != 2 {
		t.Fatalf("retry-pending delivery was not claimed = %#v", stored["ndl_claim_retry_pending"])
	}
	if stored["ndl_claim_terminal_failed"].Status != "failed" || stored["ndl_claim_terminal_failed"].AttemptCount != 1 {
		t.Fatalf("terminal failed delivery was reclaimed = %#v", stored["ndl_claim_terminal_failed"])
	}
}

func TestNotificationDeliveryClaimAllowsOnlyOneConcurrentLease(t *testing.T) {
	db := testdb.Open(t, testdb.Options{
		SchemaPrefix:       "notification_delivery_concurrent_claim_test",
		MaxOpenConnections: 4,
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.NotificationDelivery{})
		},
	})
	now := time.Now().UTC().Truncate(time.Microsecond)
	delivery := newNotificationReconcileDelivery("ndl_concurrent_claim", "pending", now, now, "")
	if err := db.Create(&delivery).Error; err != nil {
		t.Fatalf("create concurrent claim delivery: %v", err)
	}
	var firstRead model.NotificationDelivery
	var secondRead model.NotificationDelivery
	if err := db.First(&firstRead, "id = ?", delivery.ID).Error; err != nil {
		t.Fatalf("load first claim snapshot: %v", err)
	}
	if err := db.First(&secondRead, "id = ?", delivery.ID).Error; err != nil {
		t.Fatalf("load second claim snapshot: %v", err)
	}

	type claimResult struct {
		lease   notificationDeliveryLease
		claimed bool
		err     error
	}
	start := make(chan struct{})
	results := make(chan claimResult, 2)
	for _, snapshot := range []model.NotificationDelivery{firstRead, secondRead} {
		snapshot := snapshot
		go func() {
			<-start
			lease, claimed, err := claimNotificationDelivery(db, snapshot, now)
			results <- claimResult{lease: lease, claimed: claimed, err: err}
		}()
	}
	close(start)
	claimedCount := 0
	for i := 0; i < 2; i++ {
		result := <-results
		if result.err != nil {
			t.Fatalf("concurrent claim error: %v", result.err)
		}
		if result.claimed {
			claimedCount++
			if result.lease.Generation != 1 {
				t.Fatalf("concurrent lease generation = %d, want 1", result.lease.Generation)
			}
		}
	}
	if claimedCount != 1 {
		t.Fatalf("successful concurrent claims = %d, want exactly 1", claimedCount)
	}
	var stored model.NotificationDelivery
	if err := db.First(&stored, "id = ?", delivery.ID).Error; err != nil {
		t.Fatalf("load concurrently claimed delivery: %v", err)
	}
	if stored.Status != "sending" || stored.AttemptCount != 1 {
		t.Fatalf("concurrently claimed delivery = status %q attempt %d", stored.Status, stored.AttemptCount)
	}
}

func TestNotificationReconcileRecoversClaimCrashAndFencesOldCompletion(t *testing.T) {
	db := openNotificationReconcilerTestDB(t, "notification_delivery_crash_recovery_test")
	now := time.Now().UTC().Truncate(time.Microsecond)
	staleStartedAt := now.Add(-tasks.PolicyForType(tasks.TypeNotificationDeliver).Timeout - time.Minute)
	delivery := newNotificationReconcileDelivery("ndl_claim_crash", "sending", now.Add(-time.Hour), now, "")
	delivery.AttemptCount = 1
	delivery.StartedAt = &staleStartedAt
	if err := db.Create(&delivery).Error; err != nil {
		t.Fatalf("create crashed delivery lease: %v", err)
	}

	enqueuer := &recordingNotificationReconcileEnqueuer{}
	runner := &Runner{db: db, notificationDeliveryEnqueuer: enqueuer}
	if err := runner.reconcileNotificationDeliveries(t.Context(), now); err != nil {
		t.Fatalf("recover crashed notification claim: %v", err)
	}
	if got := strings.Join(enqueuer.deliveryIDs, ","); got != delivery.ID {
		t.Fatalf("re-enqueued crashed deliveries = %q", got)
	}

	oldLease := notificationDeliveryLease{DeliveryID: delivery.ID, Generation: 1}
	finishedAt := now.Add(time.Second)
	updated, err := updateNotificationDeliveryLease(db, oldLease, notificationDeliverySucceededUpdates(time.Second, `{}`, ``, finishedAt))
	if err != nil || updated {
		t.Fatalf("old lease completion after reconcile = updated %v error %v", updated, err)
	}
	var recovered model.NotificationDelivery
	if err := db.First(&recovered, "id = ?", delivery.ID).Error; err != nil {
		t.Fatalf("load recovered pending delivery: %v", err)
	}
	if recovered.Status != "pending" || recovered.AttemptCount != 1 || recovered.StartedAt != nil {
		t.Fatalf("recovered delivery = %#v", recovered)
	}

	newLease, claimed, err := claimNotificationDelivery(db, recovered, now.Add(2*time.Second))
	if err != nil || !claimed || newLease.Generation != 2 {
		t.Fatalf("new lease = %#v claimed=%v error=%v", newLease, claimed, err)
	}
	updated, err = markNotificationDeliveryLeaseFailed(db, oldLease, errors.New("stale claimant failed late"), time.Second, "", "")
	if err != nil || updated {
		t.Fatalf("old lease failure after new claim = updated %v error %v", updated, err)
	}
	updated, err = updateNotificationDeliveryLease(db, newLease, notificationDeliverySucceededUpdates(time.Second, `{}`, ``, finishedAt))
	if err != nil || !updated {
		t.Fatalf("new lease completion = updated %v error %v", updated, err)
	}
	var completed model.NotificationDelivery
	if err := db.First(&completed, "id = ?", delivery.ID).Error; err != nil {
		t.Fatalf("load completed recovered delivery: %v", err)
	}
	if completed.Status != "succeeded" || completed.AttemptCount != 2 || completed.ErrorMessage != "" {
		t.Fatalf("completed recovered delivery = %#v", completed)
	}
}

func openNotificationReconcilerTestDB(t *testing.T, schemaPrefix string) *gorm.DB {
	t.Helper()
	return testdb.Open(t, testdb.Options{
		SchemaPrefix: schemaPrefix,
		Migrate: func(db *gorm.DB) error {
			return db.AutoMigrate(&model.PlatformEvent{}, &model.NotificationChannel{}, &model.NotificationDelivery{})
		},
	})
}

func newNotificationReconcileDelivery(id string, status string, queuedAt time.Time, updatedAt time.Time, errorMessage string) model.NotificationDelivery {
	return model.NotificationDelivery{
		ID:           id,
		EventID:      "evt_" + id,
		EventType:    "build.failed",
		ChannelID:    "nch_" + id,
		AdapterKind:  "webhook",
		EventJSON:    `{}`,
		Status:       status,
		ErrorMessage: errorMessage,
		QueuedAt:     queuedAt,
		CreatedAt:    queuedAt,
		UpdatedAt:    updatedAt,
	}
}

func loadNotificationReconcileDeliveries(t *testing.T, db *gorm.DB) map[string]model.NotificationDelivery {
	t.Helper()
	var deliveries []model.NotificationDelivery
	if err := db.Find(&deliveries).Error; err != nil {
		t.Fatalf("load notification deliveries: %v", err)
	}
	byID := make(map[string]model.NotificationDelivery, len(deliveries))
	for _, delivery := range deliveries {
		byID[delivery.ID] = delivery
	}
	return byID
}
