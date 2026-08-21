package api

import (
	"reflect"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/aitool"
)

func TestAgentObservabilityCLIMetadataAndInternalAgentBoundary(t *testing.T) {
	document := readOpenAPIDocument(t, apiRepositoryRoot(t)+"/openapi/openapi.yaml")
	paths := document["paths"].(map[string]any)
	operations := map[string]map[string]any{}
	for _, rawPath := range paths {
		path, ok := rawPath.(map[string]any)
		if !ok {
			continue
		}
		for _, rawOperation := range path {
			operation, ok := rawOperation.(map[string]any)
			if !ok {
				continue
			}
			operationID, _ := operation["operationId"].(string)
			if operationID != "" {
				operations[operationID] = operation
			}
		}
	}

	visible := map[string]string{
		"getAgentObservabilityOverview":   "agent-observability.overview",
		"listAgentObservabilityTurns":     "agent-observability.turns",
		"listAgentObservabilityTools":     "agent-observability.tools",
		"listAgentObservabilityToolCalls": "agent-observability.tool-calls",
		"getAgentObservabilityTrace":      "agent-observability.trace",
	}
	for operationID, command := range visible {
		operation := operations[operationID]
		if operation == nil {
			t.Fatalf("missing OpenAPI operation %s", operationID)
		}
		metadata := operation["x-luna-cli"].(map[string]any)
		scopes, _ := schemaStringList(metadata["requiredScopes"])
		if metadata["command"] != command || metadata["classification"] != "business-command" || metadata["risk"] != "low" || metadata["agentAllowed"] != true || !reflect.DeepEqual(scopes, []string{"agent-observability:read"}) {
			t.Fatalf("%s CLI metadata = %#v", operationID, metadata)
		}
		if _, ok := aitool.PlatformOperation(operationID); ok {
			t.Fatalf("platform-internal Agent catalog exposed cross-user operation %s", operationID)
		}
	}

	sourceTest := operations["testAgentObservabilitySource"]["x-luna-cli"].(map[string]any)
	if sourceTest["command"] != "agent-observability.source-test" || sourceTest["agentAllowed"] != false {
		t.Fatalf("source-test CLI metadata = %#v", sourceTest)
	}

	for _, operationID := range []string{"listAgentObservabilityConversations", "getAgentObservabilityConversation"} {
		metadata := operations[operationID]["x-luna-cli"].(map[string]any)
		if metadata["hidden"] != true || metadata["agentAllowed"] != false {
			t.Fatalf("legacy conversation operation %s must remain hidden: %#v", operationID, metadata)
		}
		if _, ok := aitool.PlatformOperation(operationID); ok {
			t.Fatalf("platform-internal Agent catalog exposed %s", operationID)
		}
	}
}

func TestAgentObservabilityOpenAPISupportsBearerAndUnavailableEvidence(t *testing.T) {
	document := readOpenAPIDocument(t, apiRepositoryRoot(t)+"/openapi/openapi.yaml")
	paths := document["paths"].(map[string]any)
	for _, path := range []string{
		"/api/v1/ai/observability/overview",
		"/api/v1/ai/observability/turns",
		"/api/v1/ai/observability/tools",
		"/api/v1/ai/observability/tools/{operationId}/calls",
		"/api/v1/ai/observability/traces/{traceId}",
	} {
		operation := paths[path].(map[string]any)["get"].(map[string]any)
		security := operation["security"].([]any)
		if len(security) != 2 {
			t.Fatalf("%s security = %#v", path, security)
		}
		if _, ok := security[1].(map[string]any)["BearerToken"]; !ok {
			t.Fatalf("%s does not declare BearerToken security: %#v", path, security)
		}
	}

	errorSchema := document["components"].(map[string]any)["schemas"].(map[string]any)["ErrorResponse"].(map[string]any)
	properties := errorSchema["properties"].(map[string]any)
	for _, field := range []string{"requestId", "retryable", "status", "observationCode"} {
		if properties[field] == nil {
			t.Fatalf("ErrorResponse is missing %s", field)
		}
	}
}
