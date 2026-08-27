package api

import "strings"

const (
	siteBrandColorPresetKey   = "site.brandColorPreset"
	defaultBrandColorPreset   = "blue"
	brandThemeHTMLPlaceholder = "__LUNA_DEVOPS_BRAND_THEME__"
)

// The backend accepts the same compact catalog exposed by the settings picker.
var brandColorPresetOptions = []string{
	"aurora",
	"harbor",
	"sunset",
	"botanical",
	"meadow",
	"citrus",
	"gold",
	"orange",
	"red",
	"pink",
	"violet",
	"blue",
	"cyan",
	"teal",
	"green",
	"lime",
}

func normalizeBrandColorPreset(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if configOptionAllowed(value, brandColorPresetOptions) {
		return value
	}
	return defaultBrandColorPreset
}

func normalizeUserBrandColorPreset(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "", true
	}
	return value, configOptionAllowed(value, brandColorPresetOptions)
}
