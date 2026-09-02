package volumeapi

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var (
	projectVolumeObservationMetricsOnce sync.Once
	projectVolumeObservationCounter     metric.Int64Counter
)

func recordProjectVolumeObservationMetrics(ctx context.Context, observations map[string]projectVolumeObservationResponse) {
	projectVolumeObservationMetricsOnce.Do(func() {
		meter := otel.Meter("github.com/LiteyukiStudio/devops/internal/api/project-volume-observation")
		projectVolumeObservationCounter, _ = meter.Int64Counter("luna_devops_volume_observation_total",
			metric.WithDescription("Project volume Kubernetes observation results."))
	})
	if projectVolumeObservationCounter == nil {
		return
	}
	for _, observation := range observations {
		code := projectVolumeObservationMetricCode(observation.ObservationCode)
		outcome := "success"
		if code != "none" {
			outcome = "unavailable"
		}
		projectVolumeObservationCounter.Add(ctx, 1, metric.WithAttributes(
			attribute.String("outcome", outcome),
			attribute.String("observation_code", code),
		))
	}
}

func projectVolumeObservationMetricCode(value string) string {
	switch value {
	case "":
		return "none"
	case volumeObservationUnavailableCode, "volume.claim_not_found":
		return value
	default:
		return "unknown"
	}
}
