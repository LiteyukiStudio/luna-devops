package worker

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/hibiken/asynq"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

var (
	taskConsumerMetricsOnce sync.Once
	taskStartedTotal        metric.Int64Counter
	taskCompletedTotal      metric.Int64Counter
	taskRetryTotal          metric.Int64Counter
	taskInflight            metric.Int64UpDownCounter
	taskProcessDuration     metric.Float64Histogram
	taskQueueWaitDuration   metric.Float64Histogram
)

type asynqTelemetryLogger struct{}

func (asynqTelemetryLogger) Debug(args ...interface{}) { logAsynqRuntime(slog.LevelDebug, args) }
func (asynqTelemetryLogger) Info(args ...interface{})  { logAsynqRuntime(slog.LevelInfo, args) }
func (asynqTelemetryLogger) Warn(args ...interface{})  { logAsynqRuntime(slog.LevelWarn, args) }
func (asynqTelemetryLogger) Error(args ...interface{}) { logAsynqRuntime(slog.LevelError, args) }
func (asynqTelemetryLogger) Fatal(args ...interface{}) { logAsynqRuntime(slog.LevelError, args) }

func logAsynqRuntime(level slog.Level, args []interface{}) {
	telemetry.Logger().Log(context.Background(), level, "Asynq runtime event",
		slog.String("event.name", "worker.asynq.runtime"),
		slog.Int("argument.count", len(args)),
	)
}

func taskTelemetryMiddleware(next asynq.Handler) asynq.Handler {
	return asynq.HandlerFunc(func(ctx context.Context, task *asynq.Task) (err error) {
		initTaskConsumerMetrics()
		ctx = telemetry.ExtractMap(ctx, task.Headers())
		taskID, ok := asynq.GetTaskID(ctx)
		if !ok || strings.TrimSpace(taskID) == "" {
			taskID = task.Type()
		}
		queue := tasks.PolicyForType(task.Type()).Queue
		retryCount, _ := asynq.GetRetryCount(ctx)
		ctx, end := telemetry.StartOperationWithKind(ctx, "task", "process."+workerTaskOperationName(task.Type()), trace.SpanKindConsumer,
			attribute.String("messaging.system", "asynq"),
			attribute.String("messaging.destination.name", queue),
			attribute.String("messaging.operation.type", "process"),
			attribute.String("task.type", task.Type()),
			attribute.String("task.id", taskID),
			attribute.Int("task.retry_count", retryCount),
		)
		defer func() { end(err) }()
		startedAt := time.Now()
		baseMetricOptions := metric.WithAttributes(
			attribute.String("task.type", task.Type()),
			attribute.String("task.queue", queue),
		)
		if taskStartedTotal != nil {
			taskStartedTotal.Add(ctx, 1, baseMetricOptions)
		}
		if taskInflight != nil {
			taskInflight.Add(ctx, 1, baseMetricOptions)
			defer taskInflight.Add(ctx, -1, baseMetricOptions)
		}
		if retryCount > 0 && taskRetryTotal != nil {
			taskRetryTotal.Add(ctx, 1, baseMetricOptions)
		}
		if enqueuedAt, parseErr := time.Parse(time.RFC3339Nano, task.Headers()[tasks.HeaderEnqueuedAt]); parseErr == nil && taskQueueWaitDuration != nil {
			wait := time.Since(enqueuedAt)
			if wait >= 0 {
				taskQueueWaitDuration.Record(ctx, wait.Seconds(), baseMetricOptions)
			}
		}

		telemetry.Logger().InfoContext(ctx, "worker task started",
			slog.String("event.name", "task.started"),
			slog.String("task.type", task.Type()),
			slog.String("task.queue", queue),
			slog.String("task.id", taskID),
			slog.Int("task.retry_count", retryCount),
		)
		err = next.ProcessTask(ctx, task)
		outcomeMetricOptions := metric.WithAttributes(
			attribute.String("task.type", task.Type()),
			attribute.String("task.queue", queue),
			attribute.String("outcome", telemetry.ErrorOutcome(err)),
		)
		if taskCompletedTotal != nil {
			taskCompletedTotal.Add(ctx, 1, outcomeMetricOptions)
		}
		if taskProcessDuration != nil {
			taskProcessDuration.Record(ctx, time.Since(startedAt).Seconds(), outcomeMetricOptions)
		}
		if err == nil {
			telemetry.Logger().InfoContext(ctx, "worker task completed",
				slog.String("event.name", "task.completed"),
				slog.String("task.type", task.Type()),
				slog.String("task.queue", queue),
				slog.String("task.id", taskID),
			)
		} else {
			telemetry.LogError(ctx, "Worker task failed", "task.failed", "task.execute",
				"task.execute.failed", err,
				slog.String("task.type", task.Type()),
				slog.String("task.queue", queue),
				slog.String("resource.type", "task"),
				slog.String("resource.id", taskID))
		}
		return err
	})
}

func initTaskConsumerMetrics() {
	taskConsumerMetricsOnce.Do(func() {
		meter := otel.Meter("github.com/LiteyukiStudio/devops/internal/worker")
		taskStartedTotal, _ = meter.Int64Counter("luna_devops_worker_task_started_total",
			metric.WithDescription("Total worker task processing attempts."))
		taskCompletedTotal, _ = meter.Int64Counter("luna_devops_worker_task_completed_total",
			metric.WithDescription("Total completed worker task processing attempts."))
		taskRetryTotal, _ = meter.Int64Counter("luna_devops_worker_task_retries_total",
			metric.WithDescription("Total worker retry attempts."))
		taskInflight, _ = meter.Int64UpDownCounter("luna_devops_worker_task_inflight",
			metric.WithDescription("Current in-flight worker tasks."))
		taskProcessDuration, _ = meter.Float64Histogram("luna_devops_worker_task_duration_seconds",
			metric.WithDescription("Worker task processing duration."), metric.WithUnit("s"))
		taskQueueWaitDuration, _ = meter.Float64Histogram("luna_devops_worker_task_queue_wait_duration_seconds",
			metric.WithDescription("Time between worker task creation and processing start."), metric.WithUnit("s"))
	})
}

func workerTaskOperationName(taskType string) string {
	name := strings.NewReplacer(":", ".", "/", ".", " ", "_").Replace(strings.TrimSpace(taskType))
	if name == "" {
		return "unknown"
	}
	return name
}

func workerStage(ctx context.Context, operation string, run func(context.Context) error, attrs ...attribute.KeyValue) (err error) {
	ctx, end := telemetry.StartOperation(ctx, "worker", operation, attrs...)
	defer func() { end(err) }()
	telemetry.Logger().InfoContext(ctx, "worker stage started",
		slog.String("event.name", "worker.stage.started"),
		slog.String("operation", operation),
	)
	err = run(ctx)
	if err == nil {
		telemetry.Logger().InfoContext(ctx, "worker stage completed",
			slog.String("event.name", "worker.stage.completed"),
			slog.String("operation", operation),
		)
	}
	return err
}

func workerStageValue[T any](ctx context.Context, operation string, run func(context.Context) (T, error), attrs ...attribute.KeyValue) (value T, err error) {
	ctx, end := telemetry.StartOperation(ctx, "worker", operation, attrs...)
	defer func() { end(err) }()
	telemetry.Logger().InfoContext(ctx, "worker stage started",
		slog.String("event.name", "worker.stage.started"),
		slog.String("operation", operation),
	)
	value, err = run(ctx)
	if err == nil {
		telemetry.Logger().InfoContext(ctx, "worker stage completed",
			slog.String("event.name", "worker.stage.completed"),
			slog.String("operation", operation),
		)
	}
	return value, err
}
