package aitool

import (
	"strings"
	"testing"
)

func TestPlatformCatalogCoversAgentEligibleControlPlane(t *testing.T) {
	operations, err := PlatformCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if len(operations) < 150 {
		t.Fatalf("Agent-eligible catalog contains only %d operations", len(operations))
	}
	t.Logf("Agent-eligible OpenAPI operations: %d", len(operations))
	byID := make(map[string]OpenAPIOperation, len(operations))
	for _, operation := range operations {
		if operation.OperationID == "" || operation.Path == "" || operation.Method == "" {
			t.Fatalf("incomplete operation: %#v", operation)
		}
		if len(operation.RequiredScopes) == 0 {
			t.Fatalf("operation %s has no stable delegated scope", operation.OperationID)
		}
		if operation.InputSchema["type"] != "object" || operation.InputSchema["additionalProperties"] != false {
			t.Fatalf("operation %s has unsafe input schema: %#v", operation.OperationID, operation.InputSchema)
		}
		if strings.Contains(operation.Path, "/stream") || strings.Contains(operation.Path, "/terminal") {
			t.Fatalf("streaming operation entered generic Agent catalog: %#v", operation)
		}
		byID[operation.OperationID] = operation
	}
	for _, operationID := range []string{
		"createProject", "createApplication", "createDeploymentTarget",
		"installAppTemplate", "triggerBuildRun", "createRelease",
		"createGatewayRoute", "getBuildRun", "getReleaseRuntimeLogs",
		"createReleaseRuntimeCommandSession", "executeReleaseRuntimeCommandSession",
		"closeReleaseRuntimeCommandSession",
		"previewApplicationDeletion", "listRetainedVolumes", "deleteRetainedVolume",
	} {
		if _, ok := byID[operationID]; !ok {
			t.Errorf("missing common workflow operation %s", operationID)
		}
	}
	for _, operationID := range []string{
		"login", "exchangeOAuthToken", "receiveGitWebhook",
		"streamReleaseRuntimeTerminal", "createAccessToken",
	} {
		if _, ok := byID[operationID]; ok {
			t.Errorf("unsafe protocol or secret operation entered catalog: %s", operationID)
		}
	}
}

func TestPlatformCatalogProvidesSemanticSearchHints(t *testing.T) {
	operation, ok := PlatformOperation("listProjects")
	if !ok {
		t.Fatal("missing operation listProjects")
	}
	joined := strings.Join(operation.SearchHints, " ")
	if !strings.Contains(joined, "List projects") || !strings.Contains(joined, "Pagination") {
		t.Fatalf("OpenAPI semantic hints are missing: %#v", operation.SearchHints)
	}
	if !strings.HasPrefix(operation.Description, "调用 Luna DevOps") {
		t.Fatalf("model-facing fallback description must remain Chinese: %q", operation.Description)
	}
}

func TestPlatformCatalogClassifiesRuntimeCommandSessions(t *testing.T) {
	for _, operationID := range []string{
		"createReleaseRuntimeCommandSession",
		"executeReleaseRuntimeCommandSession",
	} {
		operation, ok := PlatformOperation(operationID)
		if !ok {
			t.Fatalf("missing operation %s", operationID)
		}
		if operation.Risk != "sensitive" || operation.Approval != "always" || operation.StepUpPurpose != "runtime_exec" {
			t.Errorf("operation %s policy = %#v", operationID, operation)
		}
	}

	operation, ok := PlatformOperation("execReleaseRuntimeCommand")
	if !ok {
		t.Fatal("missing operation execReleaseRuntimeCommand")
	}
	properties, _ := operation.InputSchema["properties"].(map[string]any)
	body, _ := properties["body"].(map[string]any)
	bodyProperties, _ := body["properties"].(map[string]any)
	command, _ := bodyProperties["command"].(map[string]any)
	if command["type"] != "string" {
		t.Fatalf("runtime exec command schema drifted: %#v", command)
	}
}

func TestPlatformCatalogClassifiesDestructiveOperations(t *testing.T) {
	for _, operationID := range []string{"deleteApplication", "deleteProject", "deleteRetainedVolume", "rollbackRelease"} {
		operation, ok := PlatformOperation(operationID)
		if !ok {
			t.Fatalf("missing operation %s", operationID)
		}
		if operation.Risk != "destructive" || operation.Approval != "always" {
			t.Errorf("operation %s policy = %#v", operationID, operation)
		}
	}
}

func TestApplicationDeletionCatalogRequiresDataLifecycleChoice(t *testing.T) {
	operation, ok := PlatformOperation("deleteApplication")
	if !ok {
		t.Fatal("missing operation deleteApplication")
	}
	properties, _ := operation.InputSchema["properties"].(map[string]any)
	body, _ := properties["body"].(map[string]any)
	required, _ := body["required"].([]string)
	if len(required) != 1 || required[0] != "dataAction" {
		t.Fatalf("deleteApplication body must require dataAction: %#v", body)
	}
	bodyProperties, _ := body["properties"].(map[string]any)
	dataAction, _ := bodyProperties["dataAction"].(map[string]any)
	values, _ := dataAction["enum"].([]any)
	if len(values) != 2 || values[0] != "retain" || values[1] != "delete" {
		t.Fatalf("deleteApplication dataAction enum drifted: %#v", dataAction)
	}
}
