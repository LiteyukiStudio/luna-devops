package api

import "testing"

func TestNormalizeBrandColorPresetFallsBackToBlue(t *testing.T) {
	if got := normalizeBrandColorPreset(" Teal "); got != "teal" {
		t.Fatalf("normalized preset = %q, want teal", got)
	}
	if got := normalizeBrandColorPreset("custom-css"); got != defaultBrandColorPreset {
		t.Fatalf("invalid preset = %q, want %q", got, defaultBrandColorPreset)
	}
}

func TestNormalizeUserBrandColorPresetAllowsFollowingPlatform(t *testing.T) {
	if got, valid := normalizeUserBrandColorPreset("  "); !valid || got != "" {
		t.Fatalf("empty user preset = %q, valid=%v; want empty and valid", got, valid)
	}
	if got, valid := normalizeUserBrandColorPreset(" Teal "); !valid || got != "teal" {
		t.Fatalf("official user preset = %q, valid=%v; want teal and valid", got, valid)
	}
	if got, valid := normalizeUserBrandColorPreset(" Ruby "); valid || got != "ruby" {
		t.Fatalf("hidden user preset = %q, valid=%v; want ruby and invalid", got, valid)
	}
	if got, valid := normalizeUserBrandColorPreset("custom-css"); valid || got != "custom-css" {
		t.Fatalf("custom user preset = %q, valid=%v; want invalid", got, valid)
	}
}

func TestValidateConfigValuesRejectsUnknownBrandColorPreset(t *testing.T) {
	for _, invalid := range []string{"custom-css", "ruby"} {
		if _, err := validateConfigValues(map[string]any{siteBrandColorPresetKey: invalid}); err == nil {
			t.Fatalf("expected brand color preset %q to be rejected", invalid)
		}
	}
	values, err := validateConfigValues(map[string]any{siteBrandColorPresetKey: "red"})
	if err != nil {
		t.Fatalf("validate official brand color preset: %v", err)
	}
	if values[siteBrandColorPresetKey] != "red" {
		t.Fatalf("validated preset = %q, want red", values[siteBrandColorPresetKey])
	}
}
