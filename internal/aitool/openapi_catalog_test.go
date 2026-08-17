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
		"previewApplicationDeletion",
		"listProjectVolumes", "getProjectVolume", "createProjectVolume", "updateProjectVolume",
		"previewProjectVolumeDeletion", "deleteProjectVolume", "createVolumeExport",
		"listVolumeTransfers", "getVolumeTransfer", "cancelVolumeTransfer",
	} {
		if _, ok := byID[operationID]; !ok {
			t.Errorf("missing common workflow operation %s", operationID)
		}
	}
	for _, operationID := range []string{
		"login", "exchangeOAuthToken", "receiveGitWebhook",
		"streamReleaseRuntimeTerminal", "createAccessToken",
		"getVolumeImportUploadOffset", "uploadVolumeImportContent",
		"authorizeVolumeTransferDownload", "headVolumeTransferContent", "downloadVolumeTransferContent",
		"retryProjectVolumeOperation", "retryVolumeTransfer",
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

func TestPlatformCatalogMarksRuntimeSecretInputsAsSensitive(t *testing.T) {
	operation, ok := PlatformOperation("updateDeploymentTargetRuntimeSecrets")
	if !ok {
		t.Fatal("missing secure runtime secret operation")
	}
	if operation.Risk != "sensitive" || operation.Approval != "always" || operation.StepUpPurpose != "secret_update" {
		t.Fatalf("runtime secret policy = %#v", operation)
	}
	if len(operation.SensitivePaths) != 1 || operation.SensitivePaths[0] != "body.items.*.value" {
		t.Fatalf("runtime secret sensitive paths = %#v", operation.SensitivePaths)
	}
	if _, exists := operation.InputSchema["generateSecret"]; exists {
		t.Fatal("legacy generateSecret input leaked into runtime secret schema")
	}
	if _, exists := PlatformOperation("generateSecret"); exists {
		t.Fatal("legacy generateSecret operation remains in Agent catalog")
	}
	for _, operationID := range []string{"getDeploymentTargetRuntimeSecretsSummary"} {
		if _, exists := PlatformOperation(operationID); exists {
			t.Fatalf("human-only runtime secret operation entered Agent catalog: %s", operationID)
		}
	}
	configOperation, ok := PlatformOperation("updateProjectRuntimeConfigSetRuntimeSecrets")
	if !ok || configOperation.Risk != "sensitive" || configOperation.StepUpPurpose != "secret_update" {
		t.Fatalf("runtime config secret policy = %#v", configOperation)
	}
}

func TestProjectListCatalogExposesExplicitScope(t *testing.T) {
	operation, ok := PlatformOperation("listProjects")
	if !ok {
		t.Fatal("missing operation listProjects")
	}
	properties, _ := operation.InputSchema["properties"].(map[string]any)
	scope, _ := properties["scope"].(map[string]any)
	values, _ := scope["enum"].([]any)
	if scope["default"] != "related" || len(values) != 2 || values[0] != "related" || values[1] != "all" {
		t.Fatalf("listProjects scope schema = %#v", scope)
	}
	found := false
	for _, parameter := range operation.Parameters {
		if parameter.InputName == "scope" && parameter.WireName == "scope" && parameter.In == "query" && !parameter.Required {
			found = true
		}
	}
	if !found {
		t.Fatalf("listProjects scope query parameter missing: %#v", operation.Parameters)
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
	for _, operationID := range []string{"deleteApplication", "deleteProject", "rollbackRelease"} {
		operation, ok := PlatformOperation(operationID)
		if !ok {
			t.Fatalf("missing operation %s", operationID)
		}
		if operation.Risk != "destructive" || operation.Approval != "always" {
			t.Errorf("operation %s policy = %#v", operationID, operation)
		}
	}
}

func TestApplicationDeletionCatalogDoesNotExposeLegacyDataAction(t *testing.T) {
	operation, ok := PlatformOperation("deleteApplication")
	if !ok {
		t.Fatal("missing operation deleteApplication")
	}
	properties, _ := operation.InputSchema["properties"].(map[string]any)
	if _, exists := properties["body"]; exists {
		t.Fatalf("deleteApplication must not expose a legacy dataAction body: %#v", properties)
	}
}
