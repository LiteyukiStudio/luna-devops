package api

import (
	"context"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestProjectVolumeObservationKeepsParentAndReportsUnavailable(t *testing.T) {
	previous := otel.GetTracerProvider()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
		otel.SetTracerProvider(previous)
	})

	parentCtx, parent := otel.Tracer("test").Start(context.Background(), "parent")
	adapter := &projectVolumeClusterAdapter{}
	observations := adapter.ObserveProjectVolumes(parentCtx, []model.ProjectVolume{{
		ID: "pvol_sensitive", ProjectID: "prj_sensitive", ClusterID: "clu_sensitive", Namespace: "ns-sensitive",
		OwnershipMode: model.ProjectVolumeOwnershipManaged, ClaimName: "claim-sensitive",
	}})
	parent.End()

	if observation := observations["pvol_sensitive"]; observation.ObservationCode != volumeObservationUnavailableCode {
		t.Fatalf("observation code = %q, want %q", observation.ObservationCode, volumeObservationUnavailableCode)
	}
	var operation sdktrace.ReadOnlySpan
	for _, span := range recorder.Ended() {
		if span.Name() == "volume.observe" {
			operation = span
			break
		}
	}
	if operation == nil {
		t.Fatal("volume.observe span not recorded")
	}
	if operation.Parent().SpanID() != parent.SpanContext().SpanID() {
		t.Fatalf("operation parent = %s, want %s", operation.Parent().SpanID(), parent.SpanContext().SpanID())
	}
	if operation.Status().Code != codes.Error {
		t.Fatalf("operation status = %s, want error", operation.Status().Code)
	}
	for _, attribute := range operation.Attributes() {
		if attribute.Key == "project.id" || attribute.Key == "volume.id" || attribute.Key == "cluster.id" {
			t.Fatalf("high-cardinality attribute recorded: %s", attribute.Key)
		}
	}
}
