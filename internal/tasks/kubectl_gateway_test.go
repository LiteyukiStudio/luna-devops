package tasks

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/redisconfig"
	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
)

func TestNewKubectlGatewayTaskRequiresClusterID(t *testing.T) {
	if _, err := NewKubectlGatewayTask(KubectlGatewayPayload{}); err == nil {
		t.Fatal("expected missing cluster id to fail")
	}
}

func TestNewKubectlGatewayTaskBuildsTypedPayload(t *testing.T) {
	task, err := NewKubectlGatewayTask(KubectlGatewayPayload{ClusterID: "rcl_demo"})
	if err != nil {
		t.Fatalf("NewKubectlGatewayTask() error = %v", err)
	}
	if task.Type() != TypeKubectlGateway {
		t.Fatalf("task type = %q", task.Type())
	}
	var payload KubectlGatewayPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.ClusterID != "rcl_demo" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestEnqueueKubectlGatewayDoesNotUseUniqueDeduplication(t *testing.T) {
	redis := miniredis.RunT(t)
	client := NewClientWithRedis(redisconfig.Options{Addr: redis.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	payload := KubectlGatewayPayload{ClusterID: "rcl_demo"}
	if _, err := client.EnqueueKubectlGateway(t.Context(), payload); err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	if _, err := client.EnqueueKubectlGateway(t.Context(), payload); err != nil {
		t.Fatalf("second enqueue: %v", err)
	}
}

func TestKubectlGatewayEnqueuePolicyHasNoUniqueWindow(t *testing.T) {
	policy := KubectlGatewayEnqueuePolicy()
	if policy.Unique != 0 || policy.Queue != QueueDeploy {
		t.Fatalf("policy = %#v", policy)
	}
	_ = asynq.Queue(policy.Queue)
}

func TestNewKubectlGatewaySweepTaskUsesDedicatedType(t *testing.T) {
	task := NewKubectlGatewaySweepTask()
	if task.Type() != TypeKubectlGatewaySweep || string(task.Payload()) != "{}" {
		t.Fatalf("task = %q %q", task.Type(), string(task.Payload()))
	}
	policy := KubectlGatewaySweepEnqueuePolicy()
	if policy.Queue != QueueLight || policy.Unique != 5*time.Minute {
		t.Fatalf("policy = %#v", policy)
	}
}
