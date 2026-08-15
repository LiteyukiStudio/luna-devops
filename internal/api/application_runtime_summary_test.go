package api

import (
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
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := summarizeApplicationDeploymentTargets(test.targets); got != test.want {
				t.Fatalf("summarizeApplicationDeploymentTargets() = %#v, want %#v", got, test.want)
			}
		})
	}
}
