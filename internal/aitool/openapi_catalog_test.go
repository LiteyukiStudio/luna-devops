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
