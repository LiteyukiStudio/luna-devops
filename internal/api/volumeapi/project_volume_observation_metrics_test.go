package volumeapi

import "testing"

func TestProjectVolumeObservationMetricCodeIsBounded(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"":                               "none",
		volumeObservationUnavailableCode: volumeObservationUnavailableCode,
		"volume.claim_not_found":         "volume.claim_not_found",
		"user-controlled-code":           "unknown",
	}
	for input, expected := range tests {
		if got := projectVolumeObservationMetricCode(input); got != expected {
			t.Fatalf("metric code for %q = %q, want %q", input, got, expected)
		}
	}
}
