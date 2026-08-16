package billing

import (
	"testing"
	"time"
)

func TestRuntimeUsageUsesDesiredReplicasIncludingZero(t *testing.T) {
	start := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	for _, test := range []struct {
		name     string
		desired  int32
		wantText string
	}{
		{name: "hpa desired five", desired: 5, wantText: "5"},
		{name: "scale to zero", desired: 0, wantText: "0"},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := RuntimeUsageInput{ProjectID: "prj", DeploymentTargetID: "target", DesiredReplicas: test.desired, PeriodStart: start, PeriodEnd: end}
			if got := runtimeReplicaHours(input).String(); got != test.wantText {
				t.Fatalf("runtimeReplicaHours() = %s, want %s", got, test.wantText)
			}
		})
	}
}
