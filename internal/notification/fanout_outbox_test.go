package notification

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/platformevent"
	"github.com/LiteyukiStudio/devops/internal/testdb"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

func TestMaterializeEventRejectsDirectPlatformEvent(t *testing.T) {
	db := openNotificationFanoutTestDB(t, "notification_direct_event_fanout_test")
	stored, created, err := (platformevent.Service{DB: db}).Record(t.Context(), platformevent.RecordInput{
		ID:   "evt_direct_record",
		Type: "build.failed",
		Detail: Event{
			ID: "evt_direct_record", Type: "build.failed", Severity: SeverityError,
		},
	})
	if err != nil || !created {
		t.Fatalf("record direct platform event: created=%t err=%v", created, err)
	}
	if stored.NotificationFanoutStatus != "" {
		t.Fatalf("direct event fanout status = %q, want empty", stored.NotificationFanoutStatus)
	}
	if _, err := (Service{DB: db}).MaterializeEvent(t.Context(), stored.ID); !errors.Is(err, ErrEventNotMaterializable) {
		t.Fatalf("materialize direct event error = %v, want ErrEventNotMaterializable", err)
	}
}

func TestNotificationFanoutPersistsOriginalTraceContext(t *testing.T) {
	db := openNotificationFanoutTestDB(t, "notification_fanout_trace_test")
	user := model.User{ID: "usr_fanout_trace", Email: "trace@example.com", Name: "Trace User"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create trace recipient: %v", err)
	}

	previousPropagator := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTextMapPropagator(previousPropagator) })
	traceState, err := trace.ParseTraceState("vendor=value")
	if err != nil {
		t.Fatalf("parse trace state: %v", err)
	}
	spanContext := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
		SpanID:     trace.SpanID{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18},
		TraceFlags: trace.FlagsSampled,
		TraceState: traceState,
		Remote:     true,
	})
	ctx := trace.ContextWithRemoteSpanContext(t.Context(), spanContext)

	enqueuer := &recordingNotificationEnqueuer{}
	deliveries, err := (Service{DB: db, Enqueuer: enqueuer}).Emit(ctx, Event{
		ID: "evt_fanout_trace", Type: "build.failed", Severity: SeverityError,
		Actor: ActorContext{ID: user.ID},
	})
	if err != nil || len(deliveries) != 1 {
		t.Fatalf("emit traced event: deliveries=%#v err=%v", deliveries, err)
	}

	const wantTraceparent = "00-0102030405060708090a0b0c0d0e0f10-1112131415161718-01"
	const wantTracestate = "vendor=value"
	var storedEvent model.PlatformEvent
	if err := db.First(&storedEvent, "id = ?", "evt_fanout_trace").Error; err != nil {
		t.Fatalf("load traced platform event: %v", err)
	}
	if storedEvent.TraceID != spanContext.TraceID().String() || storedEvent.FanoutTraceparent != wantTraceparent || storedEvent.FanoutTracestate != wantTracestate {
		t.Fatalf("stored event trace = id %q parent %q state %q", storedEvent.TraceID, storedEvent.FanoutTraceparent, storedEvent.FanoutTracestate)
	}
	if deliveries[0].Traceparent != wantTraceparent || deliveries[0].Tracestate != wantTracestate {
		t.Fatalf("delivery trace = parent %q state %q", deliveries[0].Traceparent, deliveries[0].Tracestate)
	}
	if len(enqueuer.traceIDs) != 1 || enqueuer.traceIDs[0] != spanContext.TraceID() {
		t.Fatalf("enqueue trace IDs = %#v, want %s", enqueuer.traceIDs, spanContext.TraceID())
	}

	for name, value := range map[string]any{"event": storedEvent, "delivery": deliveries[0]} {
		encoded, marshalErr := json.Marshal(value)
		if marshalErr != nil {
			t.Fatalf("marshal %s response model: %v", name, marshalErr)
		}
		if strings.Contains(string(encoded), "traceparent") || strings.Contains(string(encoded), "tracestate") || strings.Contains(string(encoded), "notificationFanoutStatus") {
			t.Fatalf("%s response exposed internal fanout fields: %s", name, encoded)
		}
	}
}

func TestEnqueueDeliveriesRefreshesFailedQueueTimestamp(t *testing.T) {
	db := openNotificationFanoutTestDB(t, "notification_enqueue_failure_timestamp_test")
	oldQueuedAt := time.Now().Add(-time.Hour).UTC().Truncate(time.Microsecond)
	delivery := model.NotificationDelivery{
		ID: "ndl_enqueue_failure_timestamp", EventID: "evt_enqueue_failure_timestamp",
		EventType: "build.failed", ChannelID: "nch_enqueue_failure_timestamp", AdapterKind: AdapterKindWebhook,
		EventJSON: `{}`, Status: "pending", QueuedAt: oldQueuedAt, CreatedAt: oldQueuedAt, UpdatedAt: oldQueuedAt,
	}
	if err := db.Create(&delivery).Error; err != nil {
		t.Fatalf("create pending delivery: %v", err)
	}
	enqueuer := &recordingNotificationEnqueuer{failures: 1}
	if err := (Service{DB: db, Enqueuer: enqueuer}).EnqueueDeliveries(t.Context(), []model.NotificationDelivery{delivery}); err == nil {
		t.Fatal("enqueue failure unexpectedly returned nil")
	}
	var stored model.NotificationDelivery
	if err := db.First(&stored, "id = ?", delivery.ID).Error; err != nil {
		t.Fatalf("load enqueue-failed delivery: %v", err)
	}
	if stored.Status != "enqueue_failed" || stored.ErrorMessage != DeliveryEnqueueFailedCode || !stored.QueuedAt.After(oldQueuedAt) {
		t.Fatalf("enqueue-failed delivery status=%q error=%q queuedAt=%s, want stable code and refreshed after %s", stored.Status, stored.ErrorMessage, stored.QueuedAt, oldQueuedAt)
	}
	if strings.Contains(stored.ErrorMessage, "queue unavailable") {
		t.Fatalf("enqueue-failed delivery exposed raw queue error: %q", stored.ErrorMessage)
	}
}

func openNotificationFanoutTestDB(t *testing.T, schemaPrefix string) *gorm.DB {
	t.Helper()
	return testdb.Open(t, testdb.Options{
		SchemaPrefix: schemaPrefix,
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
}
