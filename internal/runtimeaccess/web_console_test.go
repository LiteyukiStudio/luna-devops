package runtimeaccess

import "testing"

func TestEnabledUsesProjectCeiling(t *testing.T) {
	enabled, disabled := true, false
	for _, test := range []struct {
		project  bool
		override *bool
		want     bool
	}{
		{project: true, want: true},
		{project: true, override: &disabled, want: false},
		{project: false, want: false},
		{project: false, override: &enabled, want: false},
	} {
		if got := Enabled(test.project, test.override); got != test.want {
			t.Fatalf("Enabled(%v, %v) = %v, want %v", test.project, test.override, got, test.want)
		}
	}
}

func TestNormalizeOverrideOnlyKeepsFurtherDisable(t *testing.T) {
	enabled, disabled := true, false
	if got := NormalizeOverride(nil); got != nil {
		t.Fatalf("inherit override = %v, want nil", got)
	}
	if got := NormalizeOverride(&enabled); got != nil {
		t.Fatalf("enabled override = %v, want inherit", *got)
	}
	if got := NormalizeOverride(&disabled); got == nil || *got {
		t.Fatalf("disabled override = %v, want false", got)
	}
}
