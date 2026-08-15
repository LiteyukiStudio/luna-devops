package volume

import (
	"context"
	"sync"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var (
	volumeMetricOnce        sync.Once
	volumeOperationCounter  metric.Int64Counter
	volumeOperationDuration metric.Float64Histogram
)

func recordVolumeOperationMetrics(ctx context.Context, operation, sourceKind string, startedAt time.Time, err error) {
	volumeMetricOnce.Do(func() {
		meter := otel.Meter("github.com/LiteyukiStudio/devops/internal/volume")
		volumeOperationCounter, _ = meter.Int64Counter("luna_devops_volume_operations_total",
			metric.WithDescription("Completed project volume operations."))
		volumeOperationDuration, _ = meter.Float64Histogram("luna_devops_volume_operation_duration_seconds",
			metric.WithDescription("Project volume operation duration."), metric.WithUnit("s"))
	})
	operation = volumeMetricOperation(operation)
	sourceKind = volumeMetricSourceKind(sourceKind)
	if volumeOperationCounter != nil {
		volumeOperationCounter.Add(ctx, 1, metric.WithAttributes(
			attribute.String("operation", operation),
			attribute.String("outcome", telemetry.ErrorOutcome(err)),
			attribute.String("source_kind", sourceKind),
		))
	}
	if volumeOperationDuration != nil {
		volumeOperationDuration.Record(ctx, time.Since(startedAt).Seconds(), metric.WithAttributes(
			attribute.String("operation", operation),
		))
	}
}

func volumeMetricOperation(value string) string {
	switch value {
	case "create", "update", "delete", "retry", "bind", "unbind":
		return value
	default:
		return "unknown"
	}
}

func volumeMetricSourceKind(value string) string {
	switch value {
	case model.ProjectVolumeSourceBlank,
		model.ProjectVolumeSourceManaged,
		model.ProjectVolumeSourceRetained,
		model.ProjectVolumeSourceArchiveImport,
		model.ProjectVolumeSourceSnapshotRestore,
		model.ProjectVolumeSourceExistingClaim,
		"empty_dir":
		return value
	default:
		return "unknown"
	}
}
