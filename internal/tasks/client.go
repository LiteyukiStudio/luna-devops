package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/LiteyukiStudio/devops/internal/redisconfig"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/hibiken/asynq"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const (
	TypeDeployRun            = "deploy:run"
	TypeBuildRun             = "build:run"
	TypeGatewayApply         = "gateway:apply"
	TypeApplicationDelete    = "application:delete"
	TypeResourceCleanup      = "resource:cleanup"
	TypeNotificationDeliver  = "notification:deliver"
	TypeGitAccountRefresh    = "git:accounts:refresh"
	TypeSyncStatus           = "sync:status"
	TypeBillingRuntime       = "billing:runtime"
	TypeBillingAI            = "billing:ai"
	TypeRetentionRun         = "retention:run"

	QueueDeploy = "deploy"
	QueueBuild  = "build"
	QueueLight  = "light"
)

type BuildRunPayload struct {
	Envelope   TaskEnvelope `json:"envelope"`
	BuildRunID string       `json:"buildRunId"`
	BuildJobID string       `json:"buildJobId"`
	ProjectID  string       `json:"projectId"`
	ActorID    string       `json:"actorId"`
}

type DeployRunPayload struct {
	Envelope  TaskEnvelope `json:"envelope"`
	ReleaseID string       `json:"releaseId"`
	ProjectID string       `json:"projectId"`
	ActorID   string       `json:"actorId"`
}

type GatewayApplyPayload struct {
	Envelope       TaskEnvelope `json:"envelope"`
	GatewayRouteID string       `json:"gatewayRouteId"`
	ProjectID      string       `json:"projectId"`
	ActorID        string       `json:"actorId"`
}

type ApplicationDeletePayload struct {
	Envelope      TaskEnvelope `json:"envelope"`
	ApplicationID string       `json:"applicationId"`
	ProjectID     string       `json:"projectId"`
	ActorID       string       `json:"actorId"`
	DeleteData    bool         `json:"deleteData"`
}

type ResourceCleanupPayload struct {
	Envelope     TaskEnvelope `json:"envelope"`
	ResourceType string       `json:"resourceType"`
	ResourceID   string       `json:"resourceId"`
	ProjectID    string       `json:"projectId"`
	ActorID      string       `json:"actorId"`
	DeleteData   bool         `json:"deleteData"`
}

type NotificationDeliverPayload struct {
	Envelope   TaskEnvelope `json:"envelope"`
	DeliveryID string       `json:"deliveryId"`
	ActorID    string       `json:"actorId"`
}

type GitAccountRefreshPayload struct {
	Envelope TaskEnvelope `json:"envelope"`
	ActorID  string       `json:"actorId"`
}

type TaskEnvelope struct {
	TaskID      string    `json:"taskId"`
	TaskType    string    `json:"taskType"`
	DedupeKey   string    `json:"dedupeKey"`
	ActorID     string    `json:"actorId"`
	ResourceRef string    `json:"resourceRef"`
	TraceID     string    `json:"traceId"`
	Attempt     int       `json:"attempt"`
	CreatedAt   time.Time `json:"createdAt"`
}

type Client struct {
	client *asynq.Client
}

type EnqueuePolicy struct {
	Queue     string
	MaxRetry  int
	Timeout   time.Duration
	Retention time.Duration
	Unique    time.Duration
}

var (
	taskProducerMetricsOnce sync.Once
	taskEnqueuedTotal       metric.Int64Counter
	taskEnqueueDuration     metric.Float64Histogram
)

func NewClient(redisAddr string) *Client {
	return NewClientWithRedis(redisconfig.Options{Addr: redisAddr})
}

func NewClientWithRedis(options redisconfig.Options) *Client {
	return &Client{
		client: asynq.NewClient(options.Asynq()),
	}
}

func (c *Client) Close() error {
	return c.client.Close()
}

func (c *Client) EnqueueDeployRun(ctx context.Context, payload DeployRunPayload) (*asynq.TaskInfo, error) {
	task, err := NewDeployRunTask(payload)
	if err != nil {
		return nil, err
	}

	return c.enqueueWithPolicy(ctx, task, PolicyForType(TypeDeployRun))
}

func (c *Client) EnqueueBuildRun(ctx context.Context, payload BuildRunPayload) (*asynq.TaskInfo, error) {
	task, err := NewBuildRunTask(payload)
	if err != nil {
		return nil, err
	}

	return c.enqueueWithPolicy(ctx, task, PolicyForType(TypeBuildRun))
}

func (c *Client) EnqueueBuildRunAfter(ctx context.Context, payload BuildRunPayload, delay time.Duration) (*asynq.TaskInfo, error) {
	task, err := NewBuildRunTask(payload)
	if err != nil {
		return nil, err
	}

	policy := PolicyForType(TypeBuildRun)
	return c.enqueue(ctx, task, policy.Queue,
		asynq.Queue(policy.Queue),
		asynq.MaxRetry(policy.MaxRetry),
		asynq.Timeout(policy.Timeout),
		asynq.Retention(policy.Retention),
		asynq.ProcessIn(delay),
	)
}

func (c *Client) EnqueueGatewayApply(ctx context.Context, payload GatewayApplyPayload) (*asynq.TaskInfo, error) {
	task, err := NewGatewayApplyTask(payload)
	if err != nil {
		return nil, err
	}

	return c.enqueueWithPolicy(ctx, task, PolicyForType(TypeGatewayApply))
}

func (c *Client) EnqueueApplicationDelete(ctx context.Context, payload ApplicationDeletePayload) (*asynq.TaskInfo, error) {
	task, err := NewApplicationDeleteTask(payload)
	if err != nil {
		return nil, err
	}

	return c.enqueueWithPolicy(ctx, task, PolicyForType(TypeApplicationDelete))
}

func (c *Client) EnqueueResourceCleanup(ctx context.Context, payload ResourceCleanupPayload) (*asynq.TaskInfo, error) {
	task, err := NewResourceCleanupTask(payload)
	if err != nil {
		return nil, err
	}

	return c.enqueueWithPolicy(ctx, task, PolicyForType(TypeResourceCleanup))
}

func (c *Client) EnqueueNotificationDeliver(ctx context.Context, payload NotificationDeliverPayload) (*asynq.TaskInfo, error) {
	task, err := NewNotificationDeliverTask(payload)
	if err != nil {
		return nil, err
	}

	return c.enqueueWithPolicy(ctx, task, PolicyForType(TypeNotificationDeliver))
}

func (c *Client) EnqueueGitAccountRefresh(ctx context.Context, payload GitAccountRefreshPayload) (*asynq.TaskInfo, error) {
	task, err := NewGitAccountRefreshTask(payload)
	if err != nil {
		return nil, err
	}

	return c.enqueueWithPolicy(ctx, task, PolicyForType(TypeGitAccountRefresh))
}

func (c *Client) enqueueWithPolicy(ctx context.Context, task *asynq.Task, policy EnqueuePolicy) (*asynq.TaskInfo, error) {
	return c.enqueue(ctx, task, policy.Queue,
		asynq.Queue(policy.Queue),
		asynq.MaxRetry(policy.MaxRetry),
		asynq.Timeout(policy.Timeout),
		asynq.Retention(policy.Retention),
		asynq.Unique(policy.Unique),
	)
}

func (c *Client) enqueue(ctx context.Context, task *asynq.Task, queue string, options ...asynq.Option) (info *asynq.TaskInfo, err error) {
	initTaskProducerMetrics()
	operation := taskOperationName(task.Type())
	ctx, end := telemetry.StartOperationWithKind(ctx, "task", "enqueue."+operation, trace.SpanKindProducer,
		attribute.String("messaging.system", "asynq"),
		attribute.String("messaging.destination.name", queue),
		attribute.String("messaging.operation.type", "send"),
		attribute.String("task.type", task.Type()),
	)
	defer func() { end(err) }()

	startedAt := time.Now()
	task = taskWithTraceHeaders(ctx, task)
	info, err = c.client.EnqueueContext(ctx, task, options...)
	outcome := telemetry.ErrorOutcome(err)
	metricOptions := metric.WithAttributes(
		attribute.String("task.type", task.Type()),
		attribute.String("task.queue", queue),
		attribute.String("outcome", outcome),
	)
	if taskEnqueuedTotal != nil {
		taskEnqueuedTotal.Add(ctx, 1, metricOptions)
	}
	if taskEnqueueDuration != nil {
		taskEnqueueDuration.Record(ctx, time.Since(startedAt).Seconds(), metricOptions)
	}
	if err == nil {
		telemetry.Logger().InfoContext(ctx, "worker task enqueued",
			slog.String("event.name", "task.enqueued"),
			slog.String("task.type", task.Type()),
			slog.String("task.queue", queue),
			slog.String("task.id", info.ID),
		)
	} else {
		telemetry.Logger().ErrorContext(ctx, "worker task enqueue failed",
			slog.String("event.name", "task.enqueue.failed"),
			slog.String("task.type", task.Type()),
			slog.String("task.queue", queue),
			slog.String("error.type", telemetry.ErrorType(err)),
		)
	}
	return info, err
}

func initTaskProducerMetrics() {
	taskProducerMetricsOnce.Do(func() {
		meter := otel.Meter("github.com/LiteyukiStudio/devops/internal/tasks")
		taskEnqueuedTotal, _ = meter.Int64Counter("luna_devops_worker_task_enqueued_total",
			metric.WithDescription("Total worker tasks accepted or rejected by the queue client."))
		taskEnqueueDuration, _ = meter.Float64Histogram("luna_devops_worker_task_enqueue_duration_seconds",
			metric.WithDescription("Duration of worker queue enqueue operations."), metric.WithUnit("s"))
	})
}

func taskWithTraceHeaders(ctx context.Context, task *asynq.Task) *asynq.Task {
	headers := task.Headers()
	if headers == nil {
		headers = make(map[string]string)
	}
	for key, value := range telemetry.InjectMap(ctx) {
		headers[key] = value
	}
	return asynq.NewTaskWithHeaders(task.Type(), taskPayloadWithEnqueueTimestamp(task.Payload()), headers)
}

func taskPayloadWithEnqueueTimestamp(payload []byte) []byte {
	var document map[string]json.RawMessage
	if err := json.Unmarshal(payload, &document); err != nil {
		return payload
	}
	var envelope TaskEnvelope
	if rawEnvelope, ok := document["envelope"]; ok {
		if err := json.Unmarshal(rawEnvelope, &envelope); err != nil {
			return payload
		}
	}
	envelope.CreatedAt = time.Now().UTC()
	encodedEnvelope, err := json.Marshal(envelope)
	if err != nil {
		return payload
	}
	document["envelope"] = encodedEnvelope
	encodedPayload, err := json.Marshal(document)
	if err != nil {
		return payload
	}
	return encodedPayload
}

func taskOperationName(taskType string) string {
	name := strings.NewReplacer(":", ".", "/", ".", " ", "_").Replace(strings.TrimSpace(taskType))
	if name == "" {
		return "unknown"
	}
	return name
}

func PolicyForType(taskType string) EnqueuePolicy {
	switch taskType {
	case TypeBuildRun:
		return EnqueuePolicy{Queue: QueueBuild, MaxRetry: 1, Timeout: 90 * time.Minute, Retention: 24 * time.Hour, Unique: 30 * time.Minute}
	case TypeDeployRun:
		return EnqueuePolicy{Queue: QueueDeploy, MaxRetry: 3, Timeout: 30 * time.Minute, Retention: 24 * time.Hour, Unique: 30 * time.Minute}
	case TypeGatewayApply:
		return EnqueuePolicy{Queue: QueueDeploy, MaxRetry: 3, Timeout: 10 * time.Minute, Retention: 24 * time.Hour, Unique: 10 * time.Minute}
	case TypeApplicationDelete:
		return EnqueuePolicy{Queue: QueueDeploy, MaxRetry: 3, Timeout: 15 * time.Minute, Retention: 24 * time.Hour, Unique: 10 * time.Minute}
	case TypeResourceCleanup:
		return EnqueuePolicy{Queue: QueueDeploy, MaxRetry: 3, Timeout: 15 * time.Minute, Retention: 24 * time.Hour, Unique: 10 * time.Minute}
	case TypeNotificationDeliver:
		return EnqueuePolicy{Queue: QueueLight, MaxRetry: 5, Timeout: 2 * time.Minute, Retention: 24 * time.Hour, Unique: 30 * time.Second}
	case TypeGitAccountRefresh:
		return EnqueuePolicy{Queue: QueueLight, MaxRetry: 2, Timeout: 10 * time.Minute, Retention: 24 * time.Hour, Unique: 5 * time.Minute}
	case TypeVolumeProvision, TypeVolumeDelete:
		return EnqueuePolicy{Queue: QueueDeploy, MaxRetry: 10, Timeout: 30 * time.Minute, Retention: 24 * time.Hour, Unique: 30 * time.Minute}
	case TypeVolumeImport, TypeVolumeExport:
		return EnqueuePolicy{Queue: QueueDeploy, MaxRetry: 5, Timeout: 2 * time.Hour, Retention: 24 * time.Hour, Unique: 2 * time.Hour}
	case TypeVolumeReconcile:
		return EnqueuePolicy{Queue: QueueLight, MaxRetry: 3, Timeout: 10 * time.Minute, Retention: 24 * time.Hour, Unique: time.Minute}
	case TypeVolumeTransferCleanup:
		return EnqueuePolicy{Queue: QueueLight, MaxRetry: 3, Timeout: 15 * time.Minute, Retention: 24 * time.Hour, Unique: 5 * time.Minute}
	default:
		return EnqueuePolicy{Queue: QueueLight, MaxRetry: 1, Timeout: 5 * time.Minute, Retention: 24 * time.Hour, Unique: 1 * time.Minute}
	}
}

func NewBuildRunTask(payload BuildRunPayload) (*asynq.Task, error) {
	if strings.TrimSpace(payload.BuildRunID) == "" {
		return nil, errors.New("build run id is required")
	}
	if strings.TrimSpace(payload.BuildJobID) == "" {
		return nil, errors.New("build job id is required")
	}
	if strings.TrimSpace(payload.ProjectID) == "" {
		return nil, errors.New("project id is required")
	}

	payload.Envelope = ensureEnvelope(payload.Envelope, TypeBuildRun, payload.ActorID, payload.ProjectID, payload.BuildJobID)
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeBuildRun, data), nil
}

func NewDeployRunTask(payload DeployRunPayload) (*asynq.Task, error) {
	if strings.TrimSpace(payload.ReleaseID) == "" {
		return nil, errors.New("release id is required")
	}
	if strings.TrimSpace(payload.ProjectID) == "" {
		return nil, errors.New("project id is required")
	}

	payload.Envelope = ensureEnvelope(payload.Envelope, TypeDeployRun, payload.ActorID, payload.ProjectID, payload.ReleaseID)
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeDeployRun, data), nil
}

func NewGatewayApplyTask(payload GatewayApplyPayload) (*asynq.Task, error) {
	if strings.TrimSpace(payload.GatewayRouteID) == "" {
		return nil, errors.New("gateway route id is required")
	}
	if strings.TrimSpace(payload.ProjectID) == "" {
		return nil, errors.New("project id is required")
	}

	payload.Envelope = ensureEnvelope(payload.Envelope, TypeGatewayApply, payload.ActorID, payload.ProjectID, payload.GatewayRouteID)
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeGatewayApply, data), nil
}

func NewApplicationDeleteTask(payload ApplicationDeletePayload) (*asynq.Task, error) {
	if strings.TrimSpace(payload.ApplicationID) == "" {
		return nil, errors.New("application id is required")
	}
	if strings.TrimSpace(payload.ProjectID) == "" {
		return nil, errors.New("project id is required")
	}

	payload.Envelope = ensureEnvelope(payload.Envelope, TypeApplicationDelete, payload.ActorID, payload.ProjectID, payload.ApplicationID)
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeApplicationDelete, data), nil
}

func NewResourceCleanupTask(payload ResourceCleanupPayload) (*asynq.Task, error) {
	if strings.TrimSpace(payload.ResourceType) == "" {
		return nil, errors.New("resource type is required")
	}
	if strings.TrimSpace(payload.ResourceID) == "" {
		return nil, errors.New("resource id is required")
	}
	if strings.TrimSpace(payload.ProjectID) == "" {
		return nil, errors.New("project id is required")
	}

	resourceType := strings.TrimSpace(payload.ResourceType)
	payload.Envelope = ensureEnvelope(payload.Envelope, TypeResourceCleanup, payload.ActorID, payload.ProjectID, resourceType+":"+strings.TrimSpace(payload.ResourceID))
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeResourceCleanup, data), nil
}

func NewNotificationDeliverTask(payload NotificationDeliverPayload) (*asynq.Task, error) {
	if strings.TrimSpace(payload.DeliveryID) == "" {
		return nil, errors.New("notification delivery id is required")
	}

	payload.Envelope = ensureEnvelope(payload.Envelope, TypeNotificationDeliver, payload.ActorID, "notification", payload.DeliveryID)
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeNotificationDeliver, data), nil
}

func NewGitAccountRefreshTask(payload GitAccountRefreshPayload) (*asynq.Task, error) {
	payload.Envelope = ensureEnvelope(payload.Envelope, TypeGitAccountRefresh, payload.ActorID, "system", "git-accounts")
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeGitAccountRefresh, data), nil
}

func ensureEnvelope(envelope TaskEnvelope, taskType string, actorID string, scope string, resourceID string) TaskEnvelope {
	if strings.TrimSpace(envelope.TaskType) == "" {
		envelope.TaskType = taskType
	}
	if strings.TrimSpace(envelope.ActorID) == "" {
		envelope.ActorID = strings.TrimSpace(actorID)
	}
	if strings.TrimSpace(envelope.ResourceRef) == "" {
		envelope.ResourceRef = strings.TrimSpace(resourceID)
	}
	if strings.TrimSpace(envelope.DedupeKey) == "" {
		envelope.DedupeKey = taskType + ":" + strings.TrimSpace(scope) + ":" + strings.TrimSpace(resourceID)
	}
	if strings.TrimSpace(envelope.TaskID) == "" {
		envelope.TaskID = envelope.DedupeKey
	}
	if strings.TrimSpace(envelope.TraceID) == "" {
		envelope.TraceID = envelope.TaskID
	}
	return envelope
}
