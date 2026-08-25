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

func TestAgentUsageOpenAPIUsesGenericNullableBreakdownContract(t *testing.T) {
	document := readOpenAPIDocument(t, apiRepositoryRoot(t)+"/openapi/openapi.yaml")
	schemas := document["components"].(map[string]any)["schemas"].(map[string]any)

	providerUsage := schemas["AIProviderUsage"].(map[string]any)
	reportedUsage := providerUsage["oneOf"].([]any)[0].(map[string]any)
	required, _ := schemaStringList(reportedUsage["required"])
	if !reflect.DeepEqual(required, []string{"status", "inputTokens", "outputTokens", "totalTokens"}) {
		t.Fatalf("AIProviderUsage reported required fields = %#v", required)
	}
	providerProperties := reportedUsage["properties"].(map[string]any)
	for _, field := range []string{"inputTokens", "outputTokens", "totalTokens"} {
		property := providerProperties[field].(map[string]any)
		if property["type"] != "integer" || property["format"] != "int64" {
			t.Fatalf("AIProviderUsage.%s = %#v", field, property)
		}
	}
	for _, field := range []string{"cacheReadInputTokens", "cacheWriteInputTokens", "reasoningOutputTokens"} {
		property := providerProperties[field].(map[string]any)
		if property["type"] != "integer" || property["format"] != "int64" || property["nullable"] != true {
			t.Fatalf("AIProviderUsage.%s = %#v", field, property)
		}
	}
	for _, legacyField := range []string{"promptTokens", "completionTokens", "cachedPromptTokens", "cacheWritePromptTokens", "reasoningCompletionTokens"} {
		if providerProperties[legacyField] != nil {
			t.Fatalf("AIProviderUsage still exposes pre-release field %s", legacyField)
		}
	}

	for _, schemaName := range []string{"AgentObservabilityConversationTurn", "AgentObservabilityTurn"} {
		assertObservabilityUsageSchema(t, schemaName, schemas[schemaName].(map[string]any))
	}
	overview := schemas["AgentObservabilityOverview"].(map[string]any)
	overviewSummary := overview["properties"].(map[string]any)["summary"].(map[string]any)
	assertObservabilityUsageSchema(t, "AgentObservabilityOverview.summary", overviewSummary)
	assertRequiredNullablePercentage(t, "AgentObservabilityOverview.summary", overviewSummary, "cacheHitRate")

	traceDetail := schemas["AgentObservabilityTraceDetail"].(map[string]any)
	traceRequired, _ := schemaStringList(traceDetail["required"])
	if !containsSchemaField(traceRequired, "usage") {
		t.Fatalf("AgentObservabilityTraceDetail does not require usage: %#v", traceRequired)
	}
	traceUsageProperty := traceDetail["properties"].(map[string]any)["usage"].(map[string]any)
	if traceUsageProperty["nullable"] != true {
		t.Fatalf("AgentObservabilityTraceDetail.usage = %#v", traceUsageProperty)
	}
	traceUsageAllOf := traceUsageProperty["allOf"].([]any)
	if len(traceUsageAllOf) != 1 || traceUsageAllOf[0].(map[string]any)["$ref"] != "#/components/schemas/AgentObservabilityTraceUsage" {
		t.Fatalf("AgentObservabilityTraceDetail.usage ref = %#v", traceUsageAllOf)
	}
	traceUsage := schemas["AgentObservabilityTraceUsage"].(map[string]any)
	assertObservabilityUsageSchema(t, "AgentObservabilityTraceUsage", traceUsage)
	assertRequiredNullablePercentage(t, "AgentObservabilityTraceUsage", traceUsage, "cacheHitRate")

	completed := schemas["AIModelCompletedPayload"].(map[string]any)
	usageRef := completed["properties"].(map[string]any)["usage"].(map[string]any)["$ref"]
	if usageRef != "#/components/schemas/AIProviderUsage" {
		t.Fatalf("AIModelCompletedPayload.usage ref = %#v", usageRef)
	}
}

func assertRequiredNullablePercentage(t *testing.T, name string, schema map[string]any, field string) {
	t.Helper()
	required, _ := schemaStringList(schema["required"])
	if !containsSchemaField(required, field) {
		t.Fatalf("%s does not require %s", name, field)
	}
	property := schema["properties"].(map[string]any)[field].(map[string]any)
	if property["type"] != "number" || property["format"] != "double" || property["nullable"] != true || property["minimum"] != float64(0) || property["maximum"] != float64(100) {
		t.Fatalf("%s.%s = %#v", name, field, property)
	}
}

func containsSchemaField(fields []string, want string) bool {
	for _, field := range fields {
		if field == want {
			return true
		}
	}
	return false
}

func assertObservabilityUsageSchema(t *testing.T, name string, schema map[string]any) {
	t.Helper()
	required, _ := schemaStringList(schema["required"])
	requiredSet := make(map[string]struct{}, len(required))
	for _, field := range required {
		requiredSet[field] = struct{}{}
	}
	properties := schema["properties"].(map[string]any)
	for _, field := range []string{"inputTokens", "outputTokens"} {
		if _, ok := requiredSet[field]; !ok {
			t.Fatalf("%s does not require %s", name, field)
		}
		property := properties[field].(map[string]any)
		if property["type"] != "integer" || property["format"] != "int64" || property["nullable"] == true {
			t.Fatalf("%s.%s = %#v", name, field, property)
		}
	}
	for _, field := range []string{"cacheReadInputTokens", "cacheWriteInputTokens", "reasoningOutputTokens"} {
		if _, ok := requiredSet[field]; !ok {
			t.Fatalf("%s does not require %s", name, field)
		}
		property := properties[field].(map[string]any)
		if property["type"] != "integer" || property["format"] != "int64" || property["nullable"] != true {
			t.Fatalf("%s.%s = %#v", name, field, property)
		}
	}
}
