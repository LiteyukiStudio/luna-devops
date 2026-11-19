package api

import (
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/aitool"
)

func TestUpdateDeploymentTargetDeclaresLiveScaleContract(t *testing.T) {
	document := readOpenAPIDocument(t, filepath.Join(apiRepositoryRoot(t), "openapi", "openapi.yaml"))
	operation := openAPIOperationAt(t, document, "/api/v1/projects/{projectId}/applications/{applicationId}/deployment-targets/{targetId}", "put")
	responses, _ := operation["responses"].(map[string]any)
	for _, status := range []string{"200", "400", "404", "409", "502", "504"} {
		if _, ok := responses[status]; !ok {
			t.Fatalf("updateDeploymentTarget response %s is missing", status)
		}
	}
	response, _ := responses["200"].(map[string]any)
	headers, _ := response["headers"].(map[string]any)
	cacheControl, _ := headers["Cache-Control"].(map[string]any)
	schema, _ := cacheControl["schema"].(map[string]any)
	if schema["const"] != "no-store" {
		t.Fatalf("updateDeploymentTarget Cache-Control contract = %#v", cacheControl)
	}
}

func TestDeploymentTargetFacadeToolsSplitSpecEnvScaleAndVolumes(t *testing.T) {
	catalogByID := map[string]aitool.CatalogOperation{}
	for _, op := range DefaultAIToolCatalog() {
		catalogByID[op.OperationID] = op
	}

	// Raw full-replacement REST operations must never appear in the AI catalog.
	for _, forbidden := range []string{"createDeploymentTarget", "updateDeploymentTarget", "revealDeploymentTargetRuntimeSecrets"} {
		if _, leaked := catalogByID[forbidden]; leaked {
			t.Fatalf("raw REST operation %s leaked into DefaultAIToolCatalog", forbidden)
		}
	}

	createOp, ok := catalogByID["delivery_create_target"]
	if !ok {
		t.Fatal("delivery_create_target is missing from DefaultAIToolCatalog")
	}
	reqFields, ok := schemaStringList(createOp.InputSchema["required"])
	if !ok {
		t.Fatalf("delivery_create_target has no required list: %#v", createOp.InputSchema)
	}
	sort.Strings(reqFields)
	if !reflect.DeepEqual(reqFields, []string{"application", "project", "stage"}) {
		t.Fatalf("delivery_create_target required = %v, want [application project stage]", reqFields)
	}

	props := schemaProperties(t, createOp.InputSchema)
	assertSchemaEnum(t, props, "stage", "dev", "test", "staging", "prod")
	assertSchemaEnum(t, props, "workloadType", "Deployment", "StatefulSet")
	assertSchemaEnum(t, props, "sourceType", "image", "repository")

	for _, splitTool := range []string{
		"delivery_update_target_runtime",
		"delivery_configure_target_config",
		"delivery_configure_target_volumes",
		"delivery_scale_target",
	} {
		op, exists := catalogByID[splitTool]
		if !exists {
			t.Fatalf("expected Poka-Yoke facade tool %s in DefaultAIToolCatalog", splitTool)
		}
		if op.ApprovalTier != aitool.ApprovalTierL2 {
			t.Fatalf("%s approvalTier = %q, want L2", splitTool, op.ApprovalTier)
		}
	}
}

func schemaStringList(raw any) ([]string, bool) {
	switch values := raw.(type) {
	case []string:
		return append([]string(nil), values...), true
	case []any:
		output := make([]string, 0, len(values))
		for _, value := range values {
			item, ok := value.(string)
			if !ok {
				return nil, false
			}
			output = append(output, item)
		}
		return output, true
	default:
		return nil, false
	}
}

func schemaProperties(t *testing.T, schema map[string]any) map[string]any {
	t.Helper()
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema has no object properties: %#v", schema)
	}
	return properties
}

func assertSchemaEnum(t *testing.T, properties map[string]any, name string, values ...string) {
	t.Helper()
	schema, ok := properties[name].(map[string]any)
	if !ok {
		t.Fatalf("schema property %q is missing", name)
	}
	raw, ok := schema["enum"].([]any)
	if !ok {
		t.Fatalf("schema property %q has no enum: %#v", name, schema)
	}
	got := make([]string, 0, len(raw))
	for _, value := range raw {
		got = append(got, value.(string))
	}
	if !reflect.DeepEqual(got, values) {
		t.Fatalf("schema property %q enum = %v, want %v", name, got, values)
	}
}
