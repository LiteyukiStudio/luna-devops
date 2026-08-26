package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/hibiken/asynq"
)

const (
	TypeVolumeProvision       = "volume:provision"
	TypeVolumeImport          = "volume:import"
	TypeVolumeExport          = "volume:export"
	TypeVolumeDelete          = "volume:delete"
	TypeVolumeReconcile       = "volume:reconcile"
	TypeVolumeTransferCleanup = "volume-transfer:cleanup"

	VolumeOperationProvision = "provision"
	VolumeOperationExpand    = "expand"
)

type VolumeProvisionPayload struct {
	VolumeID  string `json:"volumeId"`
	ProjectID string `json:"projectId"`
	ActorID   string `json:"actorId"`
	Operation string `json:"operation"`
}

type VolumeTransferPayload struct {
	TransferID string `json:"transferId"`
	VolumeID   string `json:"volumeId"`
	ProjectID  string `json:"projectId"`
	ActorID    string `json:"actorId"`
}

type VolumeDeletePayload struct {
	VolumeID  string `json:"volumeId"`
	ProjectID string `json:"projectId"`
	ActorID   string `json:"actorId"`
}

type VolumeReconcilePayload struct {
	VolumeID string `json:"volumeId,omitempty"`
	ActorID  string `json:"actorId"`
}

type VolumeTransferCleanupPayload struct {
	TransferID string `json:"transferId,omitempty"`
	ActorID    string `json:"actorId"`
}

func (c *Client) EnqueueVolumeProvision(ctx context.Context, payload VolumeProvisionPayload) (*asynq.TaskInfo, error) {
	task, err := NewVolumeProvisionTask(payload)
	if err != nil {
		return nil, err
	}
	return c.enqueueWithPolicy(ctx, task, PolicyForType(TypeVolumeProvision))
}

func (c *Client) EnqueueVolumeImport(ctx context.Context, payload VolumeTransferPayload) (*asynq.TaskInfo, error) {
	task, err := newVolumeTransferTask(TypeVolumeImport, payload)
	if err != nil {
		return nil, err
	}
	return c.enqueueWithPolicy(ctx, task, PolicyForType(TypeVolumeImport))
}

func (c *Client) EnqueueVolumeExport(ctx context.Context, payload VolumeTransferPayload) (*asynq.TaskInfo, error) {
	task, err := newVolumeTransferTask(TypeVolumeExport, payload)
	if err != nil {
		return nil, err
	}
	return c.enqueueWithPolicy(ctx, task, PolicyForType(TypeVolumeExport))
}

func (c *Client) EnqueueVolumeDelete(ctx context.Context, payload VolumeDeletePayload) (*asynq.TaskInfo, error) {
	task, err := NewVolumeDeleteTask(payload)
	if err != nil {
		return nil, err
	}
	return c.enqueueWithPolicy(ctx, task, PolicyForType(TypeVolumeDelete))
}

func (c *Client) EnqueueVolumeReconcile(ctx context.Context, payload VolumeReconcilePayload) (*asynq.TaskInfo, error) {
	task, err := NewVolumeReconcileTask(payload)
	if err != nil {
		return nil, err
	}
	return c.enqueueWithPolicy(ctx, task, PolicyForType(TypeVolumeReconcile))
}

func (c *Client) EnqueueVolumeTransferCleanup(ctx context.Context, payload VolumeTransferCleanupPayload) (*asynq.TaskInfo, error) {
	task, err := NewVolumeTransferCleanupTask(payload)
	if err != nil {
		return nil, err
	}
	return c.enqueueWithPolicy(ctx, task, PolicyForType(TypeVolumeTransferCleanup))
}

func NewVolumeProvisionTask(payload VolumeProvisionPayload) (*asynq.Task, error) {
	if err := validateVolumeIdentity(payload.VolumeID, payload.ProjectID); err != nil {
		return nil, err
	}
	payload.Operation = strings.ToLower(strings.TrimSpace(payload.Operation))
	if payload.Operation == "" {
		payload.Operation = VolumeOperationProvision
	}
	if payload.Operation != VolumeOperationProvision && payload.Operation != VolumeOperationExpand {
		return nil, errors.New("volume provision operation is invalid")
	}
	return marshalTask(TypeVolumeProvision, payload)
}

func NewVolumeImportTask(payload VolumeTransferPayload) (*asynq.Task, error) {
	return newVolumeTransferTask(TypeVolumeImport, payload)
}

func NewVolumeExportTask(payload VolumeTransferPayload) (*asynq.Task, error) {
	return newVolumeTransferTask(TypeVolumeExport, payload)
}

func newVolumeTransferTask(taskType string, payload VolumeTransferPayload) (*asynq.Task, error) {
	if strings.TrimSpace(payload.TransferID) == "" {
		return nil, errors.New("volume transfer id is required")
	}
	if err := validateVolumeIdentity(payload.VolumeID, payload.ProjectID); err != nil {
		return nil, err
	}
	return marshalTask(taskType, payload)
}

func NewVolumeDeleteTask(payload VolumeDeletePayload) (*asynq.Task, error) {
	if err := validateVolumeIdentity(payload.VolumeID, payload.ProjectID); err != nil {
		return nil, err
	}
	return marshalTask(TypeVolumeDelete, payload)
}

func NewVolumeReconcileTask(payload VolumeReconcilePayload) (*asynq.Task, error) {
	return marshalTask(TypeVolumeReconcile, payload)
}

func NewVolumeTransferCleanupTask(payload VolumeTransferCleanupPayload) (*asynq.Task, error) {
	return marshalTask(TypeVolumeTransferCleanup, payload)
}

func validateVolumeIdentity(volumeID, projectID string) error {
	if strings.TrimSpace(volumeID) == "" {
		return errors.New("project volume id is required")
	}
	if strings.TrimSpace(projectID) == "" {
		return errors.New("project id is required")
	}
	return nil
}

func marshalTask(taskType string, payload any) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(taskType, data), nil
}
