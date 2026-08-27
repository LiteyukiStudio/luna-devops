package tasks

import (
	"context"
	"encoding/json"
	"testing"
	"time"

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
		GatewayRouteID: "gwr_1",
		ProjectID:      "prj_1",
		ActorID:        "usr_1",
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
	if got.GatewayRouteID != payload.GatewayRouteID || got.ProjectID != payload.ProjectID {
		t.Fatalf("payload = %#v", got)
	}
}

func TestNewGatewayApplyTaskRequiresCoreIDs(t *testing.T) {
	if _, err := NewGatewayApplyTask(GatewayApplyPayload{ProjectID: "prj_1"}); err == nil {
		t.Fatal("expected missing gateway route id to fail")
	}
	if _, err := NewGatewayApplyTask(GatewayApplyPayload{GatewayRouteID: "gwr_1"}); err == nil {
		t.Fatal("expected missing project id to fail")
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
		ActorID:      "usr_1",
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
	first, err := NewGatewayApplyTask(GatewayApplyPayload{GatewayRouteID: "gwr_1", ProjectID: "prj_1", ActorID: "usr_1"})
	if err != nil {
		t.Fatalf("NewGatewayApplyTask returned error: %v", err)
	}
	second, err := NewGatewayApplyTask(GatewayApplyPayload{GatewayRouteID: "gwr_1", ProjectID: "prj_1", ActorID: "usr_1"})
	if err != nil {
		t.Fatalf("NewGatewayApplyTask returned error: %v", err)
	}

	if string(first.Payload()) != string(second.Payload()) {
		t.Fatalf("task payloads differ: %s / %s", first.Payload(), second.Payload())
	}
}
