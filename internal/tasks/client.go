package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/redisconfig"
	"github.com/hibiken/asynq"
)

const (
	TypeDeployRun            = "deploy:run"
	TypeBuildRun             = "build:run"
	TypeGatewayApply         = "gateway:apply"
	TypeApplicationDelete    = "application:delete"
	TypeResourceCleanup      = "resource:cleanup"
	TypeSystemComponentApply = "system_component:apply"
	TypeNotificationDeliver  = "notification:deliver"
	TypeGitAccountRefresh    = "git:accounts:refresh"
	TypeSyncStatus           = "sync:status"
	TypeBillingRuntime       = "billing:runtime"
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

type SystemComponentApplyPayload struct {
	Envelope       TaskEnvelope `json:"envelope"`
	InstallationID string       `json:"installationId"`
	ComponentID    string       `json:"componentId"`
	ClusterID      string       `json:"clusterId"`
	ActorID        string       `json:"actorId"`
	ReportToken    string       `json:"reportToken"`
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
	return c.client.EnqueueContext(
		ctx,
		task,
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

func (c *Client) EnqueueSystemComponentApply(ctx context.Context, payload SystemComponentApplyPayload) (*asynq.TaskInfo, error) {
	task, err := NewSystemComponentApplyTask(payload)
	if err != nil {
		return nil, err
	}

	return c.enqueueWithPolicy(ctx, task, PolicyForType(TypeSystemComponentApply))
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
	return c.client.EnqueueContext(
		ctx,
		task,
		asynq.Queue(policy.Queue),
		asynq.MaxRetry(policy.MaxRetry),
		asynq.Timeout(policy.Timeout),
		asynq.Retention(policy.Retention),
		asynq.Unique(policy.Unique),
	)
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
	case TypeSystemComponentApply:
		return EnqueuePolicy{Queue: QueueDeploy, MaxRetry: 3, Timeout: 10 * time.Minute, Retention: 24 * time.Hour, Unique: 5 * time.Minute}
	case TypeNotificationDeliver:
		return EnqueuePolicy{Queue: QueueLight, MaxRetry: 5, Timeout: 2 * time.Minute, Retention: 24 * time.Hour, Unique: 30 * time.Second}
	case TypeGitAccountRefresh:
		return EnqueuePolicy{Queue: QueueLight, MaxRetry: 2, Timeout: 10 * time.Minute, Retention: 24 * time.Hour, Unique: 5 * time.Minute}
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

func NewSystemComponentApplyTask(payload SystemComponentApplyPayload) (*asynq.Task, error) {
	if strings.TrimSpace(payload.InstallationID) == "" {
		return nil, errors.New("system component installation id is required")
	}
	if strings.TrimSpace(payload.ComponentID) == "" {
		return nil, errors.New("system component id is required")
	}
	if strings.TrimSpace(payload.ClusterID) == "" {
		return nil, errors.New("runtime cluster id is required")
	}

	payload.Envelope = ensureEnvelope(payload.Envelope, TypeSystemComponentApply, payload.ActorID, payload.ClusterID, payload.InstallationID)
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeSystemComponentApply, data), nil
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
