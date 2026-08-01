package observability

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type QueueInspector interface {
	GetQueueInfo(string) (*asynq.QueueInfo, error)
}

// RegisterAsynqQueueTelemetry observes queue state from the worker process and
// exports it through the process-wide OpenTelemetry MeterProvider.
func RegisterAsynqQueueTelemetry(inspector QueueInspector, queues []string) (metric.Registration, error) {
	meter := otel.Meter("github.com/LiteyukiStudio/devops/internal/observability/queue")
	depth, err := meter.Int64ObservableGauge(
		"luna_devops_asynq_queue_depth",
		metric.WithDescription("Current Asynq queue task count by state."),
		metric.WithUnit("{task}"),
	)
	if err != nil {
		return nil, err
	}
	failed, err := meter.Int64ObservableCounter(
		"luna_devops_asynq_queue_failed_total",
		metric.WithDescription("Total Asynq tasks failed by queue."),
		metric.WithUnit("{task}"),
	)
	if err != nil {
		return nil, err
	}
	latency, err := meter.Float64ObservableGauge(
		"luna_devops_asynq_queue_latency_seconds",
		metric.WithDescription("Latency of the oldest pending task in an Asynq queue."),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}
	processed, err := meter.Int64ObservableCounter(
		"luna_devops_asynq_queue_processed_total",
		metric.WithDescription("Total Asynq tasks processed by queue."),
		metric.WithUnit("{task}"),
	)
	if err != nil {
		return nil, err
	}

	queueNames := append([]string(nil), queues...)
	return meter.RegisterCallback(func(_ context.Context, observer metric.Observer) error {
		if inspector == nil {
			return nil
		}
		for _, queue := range queueNames {
			info, inspectErr := inspector.GetQueueInfo(queue)
			if inspectErr != nil || info == nil {
				continue
			}
			queueName := stableLabel(info.Queue, queue)
			queueAttr := attribute.String("queue", queueName)
			states := map[string]int{
				"active":      info.Active,
				"aggregating": info.Aggregating,
				"archived":    info.Archived,
				"completed":   info.Completed,
				"pending":     info.Pending,
				"retry":       info.Retry,
				"scheduled":   info.Scheduled,
			}
			for state, value := range states {
				observer.ObserveInt64(depth, int64(value), metric.WithAttributes(queueAttr, attribute.String("state", state)))
			}
			observer.ObserveInt64(failed, int64(info.FailedTotal), metric.WithAttributes(queueAttr))
			observer.ObserveFloat64(latency, maxDuration(info.Latency).Seconds(), metric.WithAttributes(queueAttr))
			observer.ObserveInt64(processed, int64(info.ProcessedTotal), metric.WithAttributes(queueAttr))
		}
		return nil
	}, depth, failed, latency, processed)
}

func maxDuration(value time.Duration) time.Duration {
	if value < 0 {
		return 0
	}
	return value
}
