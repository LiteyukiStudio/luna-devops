package tasks

import (
	"context"

	"github.com/hibiken/asynq"
)

const TypeNetworkPolicyReconcile = "network-policy:reconcile"

// NetworkPolicyReconcilePayload 限定重新下发的范围：RuntimeClusterID 为空时覆盖全部活动集群，
// ProjectID 非空时只处理该项目空间的命名空间。
type NetworkPolicyReconcilePayload struct {
	RuntimeClusterID string `json:"runtimeClusterId,omitempty"`
	ProjectID        string `json:"projectId,omitempty"`
}

func NewNetworkPolicyReconcileTask(payload NetworkPolicyReconcilePayload) (*asynq.Task, error) {
	return marshalTask(TypeNetworkPolicyReconcile, payload)
}

func (c *Client) EnqueueNetworkPolicyReconcile(ctx context.Context, payload NetworkPolicyReconcilePayload) (*asynq.TaskInfo, error) {
	task, err := NewNetworkPolicyReconcileTask(payload)
	if err != nil {
		return nil, err
	}
	return c.enqueueWithPolicy(ctx, task, PolicyForType(TypeNetworkPolicyReconcile))
}

const TypeNetworkPolicyProbe = "network-policy:probe"

// NetworkPolicyProbePayload 指定要主动探测 NetworkPolicy 执行能力的集群；为空时探测全部活动集群。
type NetworkPolicyProbePayload struct {
	RuntimeClusterID string `json:"runtimeClusterId,omitempty"`
}

func NewNetworkPolicyProbeTask(payload NetworkPolicyProbePayload) (*asynq.Task, error) {
	return marshalTask(TypeNetworkPolicyProbe, payload)
}

func (c *Client) EnqueueNetworkPolicyProbe(ctx context.Context, payload NetworkPolicyProbePayload) (*asynq.TaskInfo, error) {
	task, err := NewNetworkPolicyProbeTask(payload)
	if err != nil {
		return nil, err
	}
	return c.enqueueWithPolicy(ctx, task, PolicyForType(TypeNetworkPolicyProbe))
}
