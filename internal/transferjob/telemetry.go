package transferjob

import (
	"context"
	"sync"
	"time"

	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var (
	transferInstrumentsOnce sync.Once
	transferCounter         metric.Int64Counter
	transferDuration        metric.Float64Histogram
	transferBytes           metric.Int64Counter
)

func recordTransferMetrics(ctx context.Context, config Config, result Result, startedAt time.Time, err error) {
	transferInstrumentsOnce.Do(func() {
		meter := otel.Meter("github.com/LiteyukiStudio/devops/internal/transferjob")
		transferCounter, _ = meter.Int64Counter("luna_devops_volume_transfers_total",
			metric.WithDescription("Completed project volume transfer jobs."))
		transferDuration, _ = meter.Float64Histogram("luna_devops_volume_transfer_duration_seconds",
			metric.WithDescription("Project volume transfer job duration."), metric.WithUnit("s"))
		transferBytes, _ = meter.Int64Counter("luna_devops_volume_transfer_bytes_total",
			metric.WithDescription("Successfully transferred project volume archive bytes."), metric.WithUnit("By"))
	})
	attributes := metric.WithAttributes(
		attribute.String("direction", config.Direction),
		attribute.String("format", config.Format),
		attribute.String("outcome", telemetry.ErrorOutcome(err)),
	)
	if transferCounter != nil {
		transferCounter.Add(ctx, 1, attributes)
	}
	if transferDuration != nil {
		transferDuration.Record(ctx, time.Since(startedAt).Seconds(), attributes)
	}
	if transferBytes != nil && err == nil && result.TransferredBytes > 0 {
		transferBytes.Add(ctx, result.TransferredBytes, metric.WithAttributes(
			attribute.String("direction", config.Direction),
			attribute.String("format", config.Format),
		))
	}
}
