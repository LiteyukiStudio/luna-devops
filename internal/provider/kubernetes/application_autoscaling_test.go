package kubernetes

import "testing"

func TestAutoScalingNormalizesLegacyZeroMinimumForResourceMetrics(t *testing.T) {
	spec := ApplicationResourcesSpec{
		AutoScalingEnabled:     true,
		Replicas:               2,
		AutoScalingMinReplicas: 0,
		AutoScalingMaxReplicas: 5,
		AutoScalingCPUPercent:  70,
	}
	if err := validateApplicationAutoScaling(spec); err != nil {
		t.Fatalf("validateApplicationAutoScaling rejected legacy min replicas 0: %v", err)
	}
	if got := autoScalingMinReplicas(spec); got != 2 {
		t.Fatalf("legacy autoScalingMinReplicas = %d, want replica fallback 2", got)
	}

	spec.AutoScalingMinReplicas = 1
	if err := validateApplicationAutoScaling(spec); err != nil {
		t.Fatalf("validateApplicationAutoScaling returned error: %v", err)
	}
	if got := autoScalingMinReplicas(spec); got != 1 {
		t.Fatalf("autoScalingMinReplicas = %d, want 1", got)
	}
	if got := autoScalingMaxReplicas(spec); got != 5 {
		t.Fatalf("autoScalingMaxReplicas = %d, want 5", got)
	}

	spec.AutoScalingMinReplicas = -1
	if err := validateApplicationAutoScaling(spec); err == nil {
		t.Fatal("validateApplicationAutoScaling accepted negative min replicas")
	}
}
