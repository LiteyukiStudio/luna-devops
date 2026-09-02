package deploymentapi

import (
	"testing"

	"github.com/LiteyukiStudio/devops/internal/observation"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
)

func TestDeploymentObservationFromSnapshot(t *testing.T) {
	tests := []struct {
		phase           string
		desiredReplicas int32
		want            string
	}{
		{phase: kubeprovider.DeploymentSucceeded, desiredReplicas: 1, want: observation.StatusReady},
		{phase: kubeprovider.DeploymentSucceeded, desiredReplicas: 0, want: observation.StatusScaledToZero},
		{phase: kubeprovider.DeploymentFailed, want: observation.StatusDegraded},
		{phase: kubeprovider.DeploymentRunning, want: observation.StatusProgressing},
	}
	for _, test := range tests {
		if got := deploymentObservationFromSnapshot(kubeprovider.DeploymentSnapshot{Phase: test.phase, DesiredReplicas: test.desiredReplicas}); got != test.want {
			t.Fatalf("phase %q: got %q, want %q", test.phase, got, test.want)
		}
	}
}
