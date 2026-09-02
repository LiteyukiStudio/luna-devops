package applicationapi

import (
	"reflect"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/observation"
)

func TestSummarizeApplicationDeploymentTargets(t *testing.T) {
	tests := []struct {
		name    string
		targets []model.DeploymentTarget
		want    applicationDeploymentSummary
	}{
		{name: "not deployed", want: applicationDeploymentSummary{Status: applicationRuntimeNotDeployed}},
		{
			name: "ready replicas are aggregated",
			targets: []model.DeploymentTarget{
				{Status: observation.StatusReady, DesiredReplicas: 2, ReadyReplicas: 2},
				{Status: observation.StatusReady, DesiredReplicas: 1, ReadyReplicas: 1},
			},
			want: applicationDeploymentSummary{TargetCount: 2, DesiredReplicas: 3, ReadyReplicas: 3, Status: observation.StatusReady},
		},
		{
			name: "partial replicas are progressing",
			targets: []model.DeploymentTarget{
				{Status: observation.StatusProgressing, DesiredReplicas: 3, ReadyReplicas: 1},
			},
			want: applicationDeploymentSummary{TargetCount: 1, DesiredReplicas: 3, ReadyReplicas: 1, Status: observation.StatusProgressing},
		},
		{
			name: "unavailable observation is not reported as undeployed",
			targets: []model.DeploymentTarget{
				{Status: observation.StatusUnavailable},
			},
			want: applicationDeploymentSummary{TargetCount: 1, Status: observation.StatusUnavailable},
		},
		{
			name: "scale to zero has a distinct observed runtime status",
			targets: []model.DeploymentTarget{
				{Status: observation.StatusScaledToZero, DesiredReplicas: 0, ReadyReplicas: 0},
			},
			want: applicationDeploymentSummary{TargetCount: 1, Status: observation.StatusScaledToZero},
		},
		{
			name: "ready and scaled targets remain ready when the aggregate serves replicas",
			targets: []model.DeploymentTarget{
				{Status: observation.StatusReady, DesiredReplicas: 1, ReadyReplicas: 1},
				{Status: observation.StatusScaledToZero, DesiredReplicas: 0, ReadyReplicas: 0},
			},
			want: applicationDeploymentSummary{TargetCount: 2, DesiredReplicas: 1, ReadyReplicas: 1, Status: observation.StatusReady},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := summarizeApplicationDeploymentTargets(test.targets)
			got.Targets = nil
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("summarizeApplicationDeploymentTargets() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestSummarizeApplicationDeploymentTargetsKeepsStageStatusSeparate(t *testing.T) {
	summary := summarizeApplicationDeploymentTargets([]model.DeploymentTarget{
		{ID: "dev-target", Stage: "dev", Status: observation.StatusReady, DesiredReplicas: 1, ReadyReplicas: 1},
		{ID: "test-target", Stage: "test", Status: "disabled"},
		{ID: "prod-target", Stage: "prod", Status: observation.StatusDegraded, DesiredReplicas: 2, ReadyReplicas: 1},
	})

	want := []applicationDeploymentTargetSummary{
		{ID: "prod-target", Stage: "prod", Status: observation.StatusDegraded, DesiredReplicas: 2, ReadyReplicas: 1},
		{ID: "dev-target", Stage: "dev", Status: observation.StatusReady, DesiredReplicas: 1, ReadyReplicas: 1},
		{ID: "test-target", Stage: "test", Status: "disabled"},
	}
	if !reflect.DeepEqual(summary.Targets, want) {
		t.Fatalf("summarizeApplicationDeploymentTargets().Targets = %#v, want %#v", summary.Targets, want)
	}
}
