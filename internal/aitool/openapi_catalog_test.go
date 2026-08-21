package aitool

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestPlatformCatalogDefaultsToRegularOpenAPIOperations(t *testing.T) {
	operations, err := PlatformCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if len(operations) < 100 {
		t.Fatalf("OpenAPI-derived Agent catalog is unexpectedly small: %d", len(operations))
	}
	byID := make(map[string]OpenAPIOperation, len(operations))
	for _, operation := range operations {
		if operation.OperationID == "" || operation.Path == "" || operation.Method == "" || operation.Summary == "" {
			t.Fatalf("incomplete operation: %#v", operation)
		}
		if len(operation.RequiredScopes) == 0 {
			t.Fatalf("operation %s has no stable scope", operation.OperationID)
		}
		if operation.InputSchema["type"] != "object" || operation.InputSchema["additionalProperties"] != false {
			t.Fatalf("operation %s has unsafe input schema: %#v", operation.OperationID, operation.InputSchema)
		}
		byID[operation.OperationID] = operation
	}
	for _, operationID := range []string{
		"getDashboard", "listProjects", "getProject", "createProject", "updateProject", "deleteProject",
		"listProjectVolumes", "getProjectVolume", "createProjectVolume", "updateProjectVolume",
		"previewProjectVolumeDeletion", "deleteProjectVolume",
	} {
		if _, ok := byID[operationID]; !ok {
			t.Errorf("regular platform operation is missing: %s", operationID)
		}
	}
	for _, operationID := range []string{"login", "createAccessToken", "createVolumeImport", "streamReleaseRuntimeTerminal"} {
		if _, ok := byID[operationID]; ok {
			t.Errorf("protocol or credential operation entered catalog: %s", operationID)
		}
	}
}

func TestPlatformCatalogJSONIsMinimalAndSelfContained(t *testing.T) {
	operation, ok := PlatformOperation("createProjectVolume")
	if !ok {
		t.Fatal("missing createProjectVolume")
	}
	encoded, err := json.Marshal(operation)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, removed := range []string{"\"contract\"", "predecessors", "followups", "verification", "searchHints", "\"allowed\""} {
		if strings.Contains(text, removed) {
			t.Fatalf("legacy catalog field %s leaked: %s", removed, text)
		}
	}
	for _, required := range []string{"operationId", "summary", "tags", "aliases", "requiresApproval", "inputSchema", "outputSchema", "parameters"} {
		if !strings.Contains(text, `"`+required+`"`) {
			t.Fatalf("catalog JSON is missing %s: %s", required, text)
		}
	}
	if !operation.RequiresApproval {
		t.Fatal("high-risk volume creation must require approval")
	}
}

func TestProjectVolumeCatalogCarriesBilingualSearchTermsAndSchemas(t *testing.T) {
	operation, ok := PlatformOperation("listProjectVolumes")
	if !ok {
		t.Fatal("missing listProjectVolumes")
	}
	if !strings.Contains(strings.Join(operation.Aliases.ZH, " "), "项目数据卷") ||
		!strings.Contains(strings.Join(operation.Aliases.EN, " "), "list project volumes") {
		t.Fatalf("aliases are incomplete: %#v", operation.Aliases)
	}
	if len(mapValue(operation.InputSchema["properties"])) == 0 || len(operation.OutputSchema) == 0 {
		t.Fatalf("schemas are incomplete: %#v", operation)
	}
	if operation.RequiresApproval {
		t.Fatal("read-only volume list must not require approval")
	}
}

func TestRuntimeResourceOperationsExposeDistinctStrictArguments(t *testing.T) {
	list, ok := PlatformOperation("listRuntimeClusterResources")
	if !ok {
		t.Fatal("missing listRuntimeClusterResources")
	}
	listProperties := mapValue(list.InputSchema["properties"])
	category := mapValue(listProperties["resourceCategory"])
	if _, legacy := listProperties["kind"]; legacy || !reflect.DeepEqual(category["enum"], []any{"namespaces", "workloads", "services", "configs", "storage"}) {
		t.Fatalf("resourceCategory schema = %#v", listProperties)
	}
}

func TestPlatformCatalogMarksRuntimeSecretInputsAsSensitive(t *testing.T) {
	for _, operationID := range []string{"updateDeploymentTargetRuntimeSecrets", "updateProjectRuntimeConfigSetRuntimeSecrets"} {
		operation, ok := PlatformOperation(operationID)
		if !ok {
			t.Fatalf("missing secure runtime secret operation %s", operationID)
		}
		if !operation.RequiresApproval {
			t.Fatalf("runtime secret operation must require approval: %#v", operation)
		}
		if len(operation.SensitivePaths) != 1 || operation.SensitivePaths[0] != "body.items.*.value" {
			t.Fatalf("runtime secret sensitive paths = %#v", operation.SensitivePaths)
		}
	}
}

func TestDisabledOperationsAlwaysExplainWhy(t *testing.T) {
	for operationID, reason := range agentDisabledOperations {
		if strings.TrimSpace(operationID) == "" || strings.TrimSpace(reason) == "" {
			t.Fatalf("invalid disabled operation entry %q=%q", operationID, reason)
		}
	}
}
