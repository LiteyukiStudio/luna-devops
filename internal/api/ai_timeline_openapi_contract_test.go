package api

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestAITimelineOpenAPICursorContract(t *testing.T) {
	t.Parallel()

	repositoryRoot := apiRepositoryRoot(t)
	testCases := []struct {
		name        string
		document    string
		path        string
		responseRef string
	}{
		{
			name:        "public BFF",
			document:    filepath.Join(repositoryRoot, "openapi", "openapi.yaml"),
			path:        "/api/v1/ai/conversations/{conversationId}/timeline",
			responseRef: "#/components/schemas/AITimelinePage",
		},
		{
			name:        "Agent internal",
			document:    filepath.Join(repositoryRoot, "openapi", "agent-internal.yaml"),
			path:        "/internal/v1/conversations/{conversationId}/timeline",
			responseRef: "#/components/schemas/AgentTimelinePage",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			document := readOpenAPIDocument(t, testCase.document)
			operation := openAPIOperationAt(t, document, testCase.path, "get")
			parameters := openAPIParametersByName(t, operation)

			before := parameters["before"]
			if before == nil || before["in"] != "query" {
				t.Fatalf("before query parameter = %#v", before)
			}
			limit := parameters["limit"]
			limitSchema, _ := limit["schema"].(map[string]any)
			if limit == nil || limit["in"] != "query" || fmt.Sprint(limitSchema["minimum"]) != "1" ||
				fmt.Sprint(limitSchema["maximum"]) != "100" || fmt.Sprint(limitSchema["default"]) != "30" {
				t.Fatalf("limit query parameter = %#v", limit)
			}

			responses, _ := operation["responses"].(map[string]any)
			response, _ := responses["200"].(map[string]any)
			content, _ := response["content"].(map[string]any)
			jsonContent, _ := content["application/json"].(map[string]any)
			schema, _ := jsonContent["schema"].(map[string]any)
			if schema["$ref"] != testCase.responseRef {
				t.Fatalf("timeline response schema = %#v", schema)
			}
			if _, ok := responses["400"]; !ok {
				t.Fatal("timeline cursor contract must document HTTP 400")
			}
			badRequest, _ := responses["400"].(map[string]any)
			if description, _ := badRequest["description"].(string); description == "" {
				t.Fatal("timeline cursor contract must describe the stable cursor error")
			}
		})
	}
}

func TestAITimelineOpenAPIContractDoesNotChangeProjectListErrors(t *testing.T) {
	t.Parallel()

	document := readOpenAPIDocument(t, filepath.Join(apiRepositoryRoot(t), "openapi", "openapi.yaml"))
	operation := openAPIOperationAt(t, document, "/api/v1/projects", "get")
	responses, _ := operation["responses"].(map[string]any)
	badRequest, _ := responses["400"].(map[string]any)
	if badRequest["$ref"] != "#/components/responses/BadRequest" {
		t.Fatalf("project list bad request contract = %#v", badRequest)
	}
}

func TestAIConversationDirectoryOpenAPISearchAndSortContract(t *testing.T) {
	t.Parallel()

	repositoryRoot := apiRepositoryRoot(t)
	testCases := []struct {
		document    string
		path        string
		responseRef string
	}{
		{filepath.Join(repositoryRoot, "openapi", "openapi.yaml"), "/api/v1/ai/conversations", "#/components/schemas/AIConversationPage"},
		{filepath.Join(repositoryRoot, "openapi", "agent-internal.yaml"), "/internal/v1/conversations", "#/components/schemas/AgentConversationPage"},
	}

	for _, testCase := range testCases {
		document := readOpenAPIDocument(t, testCase.document)
		operation := openAPIOperationAt(t, document, testCase.path, "get")
		parameters := openAPIParametersByName(t, operation)
		if parameters["search"] == nil {
			t.Fatalf("%s is missing search", testCase.path)
		}
		sortOrder := parameters["sortOrder"]
		schema, _ := sortOrder["schema"].(map[string]any)
		if sortOrder == nil || schema["default"] != "desc" {
			t.Fatalf("%s sortOrder = %#v", testCase.path, sortOrder)
		}
		responses, _ := operation["responses"].(map[string]any)
		response, _ := responses["200"].(map[string]any)
		content, _ := response["content"].(map[string]any)
		jsonContent, _ := content["application/json"].(map[string]any)
		responseSchema, _ := jsonContent["schema"].(map[string]any)
		if responseSchema["$ref"] != testCase.responseRef {
			t.Fatalf("%s response schema = %#v", testCase.path, responseSchema)
		}
	}
}

func openAPIOperationAt(t *testing.T, document map[string]any, path, method string) map[string]any {
	t.Helper()
	paths, _ := document["paths"].(map[string]any)
	pathItem, _ := paths[path].(map[string]any)
	operation, _ := pathItem[method].(map[string]any)
	if operation == nil {
		t.Fatalf("missing %s %s", method, path)
	}
	return operation
}

func openAPIParametersByName(t *testing.T, operation map[string]any) map[string]map[string]any {
	t.Helper()
	rawParameters, _ := operation["parameters"].([]any)
	parameters := make(map[string]map[string]any, len(rawParameters))
	for _, rawParameter := range rawParameters {
		parameter, _ := rawParameter.(map[string]any)
		name, _ := parameter["name"].(string)
		if name != "" {
			parameters[name] = parameter
		}
	}
	return parameters
}
