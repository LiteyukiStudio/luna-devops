package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/redisconfig"
	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestTaskWithTraceHeadersInjectsW3CContext(t *testing.T) {
	previousProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	defer func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
		_ = provider.Shutdown(context.Background())
	}()

	ctx, span := provider.Tracer("test").Start(context.Background(), "producer")
	task, err := NewDeployRunTask(DeployRunPayload{ReleaseID: "rel_1", ProjectID: "prj_1"})
	if err != nil {
		t.Fatalf("NewDeployRunTask returned error: %v", err)
	}
	payloadBeforeHeaders := string(task.Payload())
	task = taskWithTraceHeaders(ctx, task)
	span.End()
	if task.Headers()["traceparent"] == "" {
		t.Fatalf("task headers did not include traceparent: %#v", task.Headers())
	}
	if _, err := time.Parse(time.RFC3339Nano, task.Headers()[HeaderEnqueuedAt]); err != nil {
		t.Fatalf("task headers did not include a valid enqueue timestamp: %#v", task.Headers())
	}
	if got := string(task.Payload()); got != payloadBeforeHeaders {
		t.Fatalf("trace headers changed task payload used by Unique: before %s, after %s", payloadBeforeHeaders, got)
	}
}

func TestPolicyForTypeUsesDedicatedQueuesAndTimeouts(t *testing.T) {
	build := PolicyForType(TypeBuildRun)
	if build.Queue != QueueBuild || build.MaxRetry != 0 {
		t.Fatalf("build policy = %#v", build)
	}
	deploy := PolicyForType(TypeDeployRun)
	if deploy.Queue != QueueDeploy || deploy.Unique != 30*time.Minute {
		t.Fatalf("deploy policy = %#v", deploy)
	}
	git := PolicyForType(TypeGitAccountRefresh)
	if git.Queue != QueueLight || git.MaxRetry != 2 || git.Unique != 5*time.Minute {
		t.Fatalf("git policy = %#v", git)
	}
	appDelete := PolicyForType(TypeApplicationDelete)
	if appDelete.Queue != QueueDeploy || appDelete.Unique != 10*time.Minute {
		t.Fatalf("application delete policy = %#v", appDelete)
	}
	emailDigest := PolicyForType(TypeNotificationEmailDigest)
	if emailDigest.Queue != QueueLight || emailDigest.MaxRetry != 5 || emailDigest.Unique != 24*time.Hour {
		t.Fatalf("notification email digest policy = %#v", emailDigest)
	}
	notificationReconcile := PolicyForType(TypeNotificationReconcile)
	if notificationReconcile.Queue != QueueLight || notificationReconcile.MaxRetry != 0 ||
		notificationReconcile.Timeout != time.Minute || notificationReconcile.Unique != time.Minute {
		t.Fatalf("notification reconcile policy = %#v", notificationReconcile)
	}
}

func TestNewNotificationEmailDigestTaskUsesUserAndDueSecond(t *testing.T) {
	payload := NotificationEmailDigestPayload{RecipientUserID: "usr_digest", DueAtUnix: 1_788_000_000}
	task, err := NewNotificationEmailDigestTask(payload)
	if err != nil {
		t.Fatalf("NewNotificationEmailDigestTask returned error: %v", err)
	}
	if task.Type() != TypeNotificationEmailDigest {
		t.Fatalf("task type = %q", task.Type())
	}
	var got NotificationEmailDigestPayload
	if err := json.Unmarshal(task.Payload(), &got); err != nil {
		t.Fatalf("unmarshal notification email digest payload: %v", err)
	}
	if got != payload {
		t.Fatalf("payload = %#v, want %#v", got, payload)
	}
}

func TestNewNotificationEmailDigestTaskRequiresUserAndDueSecond(t *testing.T) {
	if _, err := NewNotificationEmailDigestTask(NotificationEmailDigestPayload{DueAtUnix: 1}); err == nil {
		t.Fatal("expected missing recipient user id to fail")
	}
	if _, err := NewNotificationEmailDigestTask(NotificationEmailDigestPayload{RecipientUserID: "usr_digest"}); err == nil {
		t.Fatal("expected missing due time to fail")
	}
}

func TestNewNotificationReconcileTaskUsesStableEmptyPayload(t *testing.T) {
	task, err := NewNotificationReconcileTask(NotificationReconcilePayload{})
	if err != nil {
		t.Fatalf("NewNotificationReconcileTask returned error: %v", err)
	}
	if task.Type() != TypeNotificationReconcile {
		t.Fatalf("task type = %q", task.Type())
	}
	if got := string(task.Payload()); got != "{}" {
		t.Fatalf("task payload = %q, want stable empty object", got)
	}
}

func TestEnqueueNotificationEmailDigestDeduplicatesOnlySameUserAndDueSecond(t *testing.T) {
	redis := miniredis.RunT(t)
	client := NewClientWithRedis(redisconfig.Options{Addr: redis.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	dueAtUnix := time.Now().Add(time.Minute).Unix()
	payload := NotificationEmailDigestPayload{RecipientUserID: "usr_digest", DueAtUnix: dueAtUnix}
	if _, err := client.EnqueueNotificationEmailDigest(t.Context(), payload); err != nil {
		t.Fatalf("enqueue first user digest: %v", err)
	}
	if _, err := client.EnqueueNotificationEmailDigest(t.Context(), payload); !errors.Is(err, asynq.ErrDuplicateTask) {
		t.Fatalf("same user and due second error = %v, want ErrDuplicateTask", err)
	}
	if _, err := client.EnqueueNotificationEmailDigest(t.Context(), NotificationEmailDigestPayload{
		RecipientUserID: "usr_other", DueAtUnix: dueAtUnix,
	}); err != nil {
		t.Fatalf("enqueue independent user digest: %v", err)
	}
	if _, err := client.EnqueueNotificationEmailDigest(t.Context(), NotificationEmailDigestPayload{
		RecipientUserID: payload.RecipientUserID, DueAtUnix: dueAtUnix + 1,
	}); err != nil {
		t.Fatalf("enqueue same user at next due second: %v", err)
	}
}

func TestNewDeployRunTaskBuildsTypedPayload(t *testing.T) {
	payload := DeployRunPayload{
		ReleaseID: "rel_1",
		ProjectID: "prj_1",
		ActorID:   "usr_1",
	}

	task, err := NewDeployRunTask(payload)
	if err != nil {
		t.Fatalf("NewDeployRunTask returned error: %v", err)
	}
	if task.Type() != TypeDeployRun {
		t.Fatalf("task type = %q", task.Type())
	}

	var got DeployRunPayload
	if err := json.Unmarshal(task.Payload(), &got); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if got.ReleaseID != payload.ReleaseID || got.ProjectID != payload.ProjectID {
		t.Fatalf("payload = %#v", got)
	}
}

func TestNewDeployRunTaskRequiresCoreIDs(t *testing.T) {
	if _, err := NewDeployRunTask(DeployRunPayload{ProjectID: "prj_1"}); err == nil {
		t.Fatal("expected missing release id to fail")
	}
	if _, err := NewDeployRunTask(DeployRunPayload{ReleaseID: "rel_1"}); err == nil {
		t.Fatal("expected missing project id to fail")
	}
}

func TestNewGatewayApplyTaskBuildsTypedPayload(t *testing.T) {
	payload := GatewayApplyPayload{
		GatewayRouteID:          "gwr_1",
		ProjectID:               "prj_1",
		ActorID:                 "usr_1",
		RouteUpdatedAtUnixMicro: 1_788_000_000_123_456,
	}

	task, err := NewGatewayApplyTask(payload)
	if err != nil {
		t.Fatalf("NewGatewayApplyTask returned error: %v", err)
	}
	if task.Type() != TypeGatewayApply {
		t.Fatalf("task type = %q", task.Type())
	}

	var got GatewayApplyPayload
	if err := json.Unmarshal(task.Payload(), &got); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if got != payload {
		t.Fatalf("payload = %#v", got)
	}
}

func TestNewGatewayApplyTaskRequiresCoreIDs(t *testing.T) {
	if _, err := NewGatewayApplyTask(GatewayApplyPayload{ProjectID: "prj_1", RouteUpdatedAtUnixMicro: 1}); err == nil {
		t.Fatal("expected missing gateway route id to fail")
	}
	if _, err := NewGatewayApplyTask(GatewayApplyPayload{GatewayRouteID: "gwr_1", RouteUpdatedAtUnixMicro: 1}); err == nil {
		t.Fatal("expected missing project id to fail")
	}
	if _, err := NewGatewayApplyTask(GatewayApplyPayload{GatewayRouteID: "gwr_1", ProjectID: "prj_1"}); err == nil {
		t.Fatal("expected missing gateway route generation to fail")
	}
}

func TestNewApplicationDeleteTaskBuildsTypedPayload(t *testing.T) {
	payload := ApplicationDeletePayload{
		ApplicationID: "app_1",
		ProjectID:     "prj_1",
		ActorID:       "usr_1",
	}

	task, err := NewApplicationDeleteTask(payload)
	if err != nil {
		t.Fatalf("NewApplicationDeleteTask returned error: %v", err)
	}
	if task.Type() != TypeApplicationDelete {
		t.Fatalf("task type = %q", task.Type())
	}

	var got ApplicationDeletePayload
	if err := json.Unmarshal(task.Payload(), &got); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if got.ApplicationID != payload.ApplicationID || got.ProjectID != payload.ProjectID {
		t.Fatalf("payload = %#v", got)
	}
}

func TestNewApplicationDeleteTaskRequiresCoreIDs(t *testing.T) {
	if _, err := NewApplicationDeleteTask(ApplicationDeletePayload{ProjectID: "prj_1"}); err == nil {
		t.Fatal("expected missing application id to fail")
	}
	if _, err := NewApplicationDeleteTask(ApplicationDeletePayload{ApplicationID: "app_1"}); err == nil {
		t.Fatal("expected missing project id to fail")
	}
}

func TestNewResourceCleanupTaskBuildsTypedPayload(t *testing.T) {
	payload := ResourceCleanupPayload{
		ResourceType: "deployment_target",
		ResourceID:   "dplt_1",
		ProjectID:    "prj_1",
	}

	task, err := NewResourceCleanupTask(payload)
	if err != nil {
		t.Fatalf("NewResourceCleanupTask returned error: %v", err)
	}
	if task.Type() != TypeResourceCleanup {
		t.Fatalf("task type = %q", task.Type())
	}

	var got ResourceCleanupPayload
	if err := json.Unmarshal(task.Payload(), &got); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if got.ResourceType != payload.ResourceType || got.ResourceID != payload.ResourceID || got.ProjectID != payload.ProjectID {
		t.Fatalf("payload = %#v", got)
	}
}

func TestNewGitAccountRefreshTaskBuildsTypedPayload(t *testing.T) {
	payload := GitAccountRefreshPayload{ActorID: "system"}

	task, err := NewGitAccountRefreshTask(payload)
	if err != nil {
		t.Fatalf("NewGitAccountRefreshTask returned error: %v", err)
	}
	if task.Type() != TypeGitAccountRefresh {
		t.Fatalf("task type = %q", task.Type())
	}

	var got GitAccountRefreshPayload
	if err := json.Unmarshal(task.Payload(), &got); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if got.ActorID != payload.ActorID {
		t.Fatalf("payload = %#v", got)
	}
}

func TestTaskPayloadIsStableForSameInput(t *testing.T) {
	payload := GatewayApplyPayload{GatewayRouteID: "gwr_1", ProjectID: "prj_1", ActorID: "usr_1", RouteUpdatedAtUnixMicro: 1_788_000_000_123_456}
	first, err := NewGatewayApplyTask(payload)
	if err != nil {
		t.Fatalf("NewGatewayApplyTask returned error: %v", err)
	}
	second, err := NewGatewayApplyTask(payload)
	if err != nil {
		t.Fatalf("NewGatewayApplyTask returned error: %v", err)
	}

	if string(first.Payload()) != string(second.Payload()) {
		t.Fatalf("task payloads differ: %s / %s", first.Payload(), second.Payload())
	}
}
