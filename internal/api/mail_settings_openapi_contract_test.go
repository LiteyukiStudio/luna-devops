package api

import (
	"path/filepath"
	"testing"
)

func TestPlatformMailSettingsOpenAPICooldownContract(t *testing.T) {
	t.Parallel()

	document := readOpenAPIDocument(t, filepath.Join(apiRepositoryRoot(t), "openapi", "openapi.yaml"))
	components, _ := document["components"].(map[string]any)
	schemas, _ := components["schemas"].(map[string]any)
	for _, schemaName := range []string{"PlatformMailSettings", "PlatformMailSettingsInput"} {
		schema, _ := schemas[schemaName].(map[string]any)
		properties, _ := schema["properties"].(map[string]any)
		cooldown, _ := properties["personalEmailCooldownSeconds"].(map[string]any)
		if cooldown["type"] != "integer" ||
			cooldown["minimum"] != float64(0) ||
			cooldown["maximum"] != float64(3600) ||
			cooldown["default"] != float64(60) {
			t.Fatalf("%s.personalEmailCooldownSeconds = %#v", schemaName, cooldown)
		}
		required, _ := schemaStringList(schema["required"])
		if !containsString(required, "personalEmailCooldownSeconds") {
			t.Fatalf("%s does not require personalEmailCooldownSeconds", schemaName)
		}
	}
}
