package deploymentapi

import (
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
)

func TestDeploymentTargetMetricsUsesObservedDesiredReplicasForCapacity(t *testing.T) {
	response := deploymentTargetMetricsResponseFromSnapshot(kubeprovider.RuntimeMetricsSnapshot{
		Available: true, DesiredReplicas: 5, ReadyReplicas: 4, AvailableReplicas: 4,
	}, model.DeploymentTarget{Replicas: 2, CPURequest: "1", MemoryRequest: "1Gi"})
	if response.ConfiguredReplicas != 2 || response.DesiredReplicas != 5 || response.ReadyReplicas != 4 {
		t.Fatalf("replica fields = %#v", response)
	}
	if response.CPUCapacityMilli != 5000 || response.MemoryCapacityBytes != 5*1024*1024*1024 {
		t.Fatalf("capacity = cpu %d memory %d", response.CPUCapacityMilli, response.MemoryCapacityBytes)
	}
}

func TestDeploymentTargetMetricsPreservesScaleToZero(t *testing.T) {
	response := deploymentTargetMetricsResponseFromSnapshot(kubeprovider.RuntimeMetricsSnapshot{
		Available: true, DesiredReplicas: 0,
	}, model.DeploymentTarget{Replicas: 2, CPURequest: "1", MemoryRequest: "1Gi"})
	if response.DesiredReplicas != 0 || response.CPUCapacityMilli != 0 || response.MemoryCapacityBytes != 0 {
		t.Fatalf("scale-to-zero metrics = %#v", response)
	}
}
