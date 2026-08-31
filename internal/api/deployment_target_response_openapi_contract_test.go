package api

import (
	"path/filepath"
	"testing"
)

func TestDeploymentTargetResponseOpenAPIUsesCanonicalCollections(t *testing.T) {
	t.Parallel()

	document := readOpenAPIDocument(t, filepath.Join(apiRepositoryRoot(t), "openapi", "openapi.yaml"))
	components, _ := document["components"].(map[string]any)
	schemas, _ := components["schemas"].(map[string]any)
	target, _ := schemas["DeploymentTarget"].(map[string]any)
	properties, _ := target["properties"].(map[string]any)
	for _, field := range []string{"servicePorts", "runtimeConfigRefs"} {
		property, _ := properties[field].(map[string]any)
		if property["type"] != "array" {
			t.Fatalf("DeploymentTarget.%s = %#v, want array", field, property)
		}
	}
	for _, legacy := range []string{"servicePort", "runtimeConfigSetIds", "configRefs"} {
		if properties[legacy] != nil {
			t.Fatalf("DeploymentTarget response retains legacy field %s", legacy)
		}
	}
	required, _ := schemaStringList(target["required"])
	for _, field := range []string{"servicePorts", "runtimeConfigRefs"} {
		if !containsString(required, field) {
			t.Fatalf("DeploymentTarget response does not require %s", field)
		}
	}
}
