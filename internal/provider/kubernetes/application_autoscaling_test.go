package kubernetes

import "testing"

func TestAutoScalingAllowsScaleToZero(t *testing.T) {
	spec := ApplicationResourcesSpec{
		AutoScalingEnabled:     true,
		Replicas:               2,
		AutoScalingMinReplicas: 0,
		AutoScalingMaxReplicas: 5,
		AutoScalingCPUPercent:  70,
	}
	if err := validateApplicationAutoScaling(spec); err != nil {
		t.Fatalf("validateApplicationAutoScaling returned error: %v", err)
	}
	if got := autoScalingMinReplicas(spec); got != 0 {
		t.Fatalf("autoScalingMinReplicas = %d, want 0", got)
	}
	if got := autoScalingMaxReplicas(spec); got != 5 {
		t.Fatalf("autoScalingMaxReplicas = %d, want 5", got)
	}
}
