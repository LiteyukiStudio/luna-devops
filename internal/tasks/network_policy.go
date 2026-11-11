package tasks

import (
	"context"

	"github.com/hibiken/asynq"
)

const TypeNetworkPolicyReconcile = "network-policy:reconcile"

// NetworkPolicyReconcilePayload 指定要重新下发项目空间网络策略的集群；为空时覆盖全部活动集群。
type NetworkPolicyReconcilePayload struct {
	RuntimeClusterID string `json:"runtimeClusterId,omitempty"`
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
