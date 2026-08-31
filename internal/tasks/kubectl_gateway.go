package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/hibiken/asynq"
)

const (
	TypeKubectlGateway      = "kubectl:gateway:reconcile"
	TypeKubectlGatewaySweep = "kubectl:gateway:sweep"
)

type KubectlGatewayPayload struct {
	ClusterID string `json:"clusterId"`
}

func KubectlGatewayEnqueuePolicy() EnqueuePolicy {
	return EnqueuePolicy{
		Queue:     QueueDeploy,
		MaxRetry:  3,
		Timeout:   5 * time.Minute,
		Retention: 24 * time.Hour,
	}
}

func KubectlGatewaySweepEnqueuePolicy() EnqueuePolicy {
	return EnqueuePolicy{
		Queue:     QueueLight,
		MaxRetry:  1,
		Timeout:   5 * time.Minute,
		Retention: 24 * time.Hour,
		Unique:    5 * time.Minute,
	}
}

func NewKubectlGatewayTask(payload KubectlGatewayPayload) (*asynq.Task, error) {
	payload.ClusterID = strings.TrimSpace(payload.ClusterID)
	if payload.ClusterID == "" {
		return nil, errors.New("cluster id is required")
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeKubectlGateway, data), nil
}

func NewKubectlGatewaySweepTask() *asynq.Task {
	return asynq.NewTask(TypeKubectlGatewaySweep, []byte("{}"))
}

func (c *Client) EnqueueKubectlGateway(ctx context.Context, payload KubectlGatewayPayload) (*asynq.TaskInfo, error) {
	task, err := NewKubectlGatewayTask(payload)
	if err != nil {
		return nil, err
	}
	policy := KubectlGatewayEnqueuePolicy()
	return c.enqueue(ctx, task, policy.Queue,
		asynq.Queue(policy.Queue),
		asynq.MaxRetry(policy.MaxRetry),
		asynq.Timeout(policy.Timeout),
		asynq.Retention(policy.Retention),
	)
}
