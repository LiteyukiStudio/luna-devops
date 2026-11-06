package deploymentapi

import (
	"encoding/json"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
)

func TestDeploymentTargetMetricsAggregatesTemplateEnvelopeAcrossReplicas(t *testing.T) {
	const gib = int64(1024 * 1024 * 1024)
	// The template envelope already includes sidecars, matching the container set
	// that metrics.k8s.io usage is summed over.
	bounded := kubeprovider.PodTemplateResources{CPURequestMilli: 150, MemoryRequestBytes: gib / 4, CPULimitMilli: 1200, MemoryLimitBytes: gib}
	for _, test := range []struct {
		name               string
		snapshot           kubeprovider.RuntimeMetricsSnapshot
		target             model.DeploymentTarget
		wantCPURequest     int64
		wantCPULimit       int64
		wantMemoryLimit    int64
		wantCPUPercent     *float64
		wantMemoryPercent  *float64
		wantReplicaRequest int64
	}{
		{
			name: "observed desired replicas, not configured replicas",
			snapshot: kubeprovider.RuntimeMetricsSnapshot{
				Available: true, DesiredReplicas: 5, ReadyReplicas: 4, AvailableReplicas: 4,
				CPUUsageMilli: 3000, MemoryUsageBytes: gib, Resources: bounded,
			},
			target:         model.DeploymentTarget{Replicas: 2, CPURequest: "9", MemoryRequest: "9Gi"},
			wantCPURequest: 750, wantCPULimit: 6000, wantMemoryLimit: 5 * gib,
			wantCPUPercent: floatPointer(50), wantMemoryPercent: floatPointer(20), wantReplicaRequest: 150,
		},
		{
			name: "unbounded limit has no percentage",
			snapshot: kubeprovider.RuntimeMetricsSnapshot{
				Available: true, DesiredReplicas: 2, CPUUsageMilli: 400, MemoryUsageBytes: gib,
				Resources: kubeprovider.PodTemplateResources{CPURequestMilli: 100, MemoryRequestBytes: gib / 4},
			},
			wantCPURequest: 200, wantReplicaRequest: 100,
		},
		{
			name:     "scale to zero has no capacity",
			snapshot: kubeprovider.RuntimeMetricsSnapshot{Available: true, DesiredReplicas: 0, Resources: bounded},
			target:   model.DeploymentTarget{Replicas: 2}, wantReplicaRequest: 150,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := deploymentTargetMetricsResponseFromSnapshot(test.snapshot, test.target)
			if response.ConfiguredReplicas != test.target.Replicas || response.DesiredReplicas != test.snapshot.DesiredReplicas {
				t.Fatalf("replica fields = %#v", response)
			}
			if response.CPURequestMilli != test.wantCPURequest || response.CPULimitMilli != test.wantCPULimit || response.MemoryLimitBytes != test.wantMemoryLimit {
				t.Fatalf("aggregate envelope = request %d limit %d memory limit %d", response.CPURequestMilli, response.CPULimitMilli, response.MemoryLimitBytes)
			}
			if response.ReplicaCPURequestMilli != test.wantReplicaRequest {
				t.Fatalf("replica cpu request = %d, want %d", response.ReplicaCPURequestMilli, test.wantReplicaRequest)
			}
			assertPercent(t, "cpu", response.CPUUsagePercent, test.wantCPUPercent)
			assertPercent(t, "memory", response.MemoryUsagePercent, test.wantMemoryPercent)
		})
	}
}

func TestDeploymentTargetMetricsOmitsPercentWithoutLimit(t *testing.T) {
	payload, err := json.Marshal(deploymentTargetMetricsResponseFromSnapshot(kubeprovider.RuntimeMetricsSnapshot{Available: true, DesiredReplicas: 1}, model.DeploymentTarget{}))
	if err != nil {
		t.Fatalf("marshal metrics: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		t.Fatalf("decode metrics: %v", err)
	}
	if _, ok := body["cpuUsagePercent"]; ok {
		t.Fatalf("cpuUsagePercent must be omitted without a limit: %s", payload)
	}
}

func floatPointer(value float64) *float64 { return &value }

func assertPercent(t *testing.T, name string, got, want *float64) {
	t.Helper()
	if (got == nil) != (want == nil) || (got != nil && *got != *want) {
		t.Fatalf("%s percent = %v, want %v", name, got, want)
	}
}
