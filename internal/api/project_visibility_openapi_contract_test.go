package api

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestProjectVisibilityOpenAPIContract(t *testing.T) {
	t.Parallel()

	document := readOpenAPIDocument(t, filepath.Join(apiRepositoryRoot(t), "openapi", "openapi.yaml"))
	components := document["components"].(map[string]any)
	parameters := components["parameters"].(map[string]any)
	visibility := parameters["Visibility"].(map[string]any)
	if visibility["name"] != "visibility" || visibility["in"] != "query" || visibility["required"] == true {
		t.Fatalf("Visibility parameter = %#v, want optional visibility query parameter", visibility)
	}
	schema := visibility["schema"].(map[string]any)
	if schema["type"] != "string" || schema["default"] != "related" || !reflect.DeepEqual(schema["enum"], []any{"related", "all"}) {
		t.Fatalf("Visibility schema = %#v, want related|all with related default", schema)
	}
	description, _ := visibility["description"].(string)
	for _, expected := range []string{"platform administrators", "400", "403"} {
		if !strings.Contains(description, expected) {
			t.Fatalf("Visibility description is missing %q: %q", expected, description)
		}
	}

	targets := map[string]string{
		"/api/v1/dashboard":                              "getDashboard",
		"/api/v1/projects":                               "listProjects",
		"/api/v1/events":                                 "listPlatformEvents",
		"/api/v1/runtime/clusters":                       "listRuntimeClusters",
		"/api/v1/git/providers":                          "listGitProviders",
		"/api/v1/git/accounts":                           "listGitAccounts",
		"/api/v1/registries":                             "listArtifactRegistries",
		"/api/v1/registries/{registryId}/credentials":    "listRegistryCredentials",
		"/api/v1/registry-credentials":                   "listAllRegistryCredentials",
		"/api/v1/build/variable-sets":                    "listBuildVariableSets",
		"/api/v1/container-images":                       "listContainerImages",
		"/api/v1/runtime/clusters/{clusterId}/resources": "listRuntimeClusterResources",
	}
	paths := document["paths"].(map[string]any)
	for path, operationID := range targets {
		pathItem := paths[path].(map[string]any)
		operation := pathItem["get"].(map[string]any)
		if operation["operationId"] != operationID {
			t.Fatalf("GET %s operationId = %#v, want %q", path, operation["operationId"], operationID)
		}
		assertVisibilityParameterReference(t, path, operation)
		responses := operation["responses"].(map[string]any)
		for _, status := range []string{"400", "403"} {
			if responses[status] == nil {
				t.Errorf("%s is missing the %s visibility response", operationID, status)
			}
		}
	}
}

func TestNotificationAndInboxOpenAPIOmitProjectVisibility(t *testing.T) {
	t.Parallel()

	document := readOpenAPIDocument(t, filepath.Join(apiRepositoryRoot(t), "openapi", "openapi.yaml"))
	paths := document["paths"].(map[string]any)
	checked := 0
	for path, rawPathItem := range paths {
		if !strings.Contains(path, "notification") && !strings.HasPrefix(path, "/api/v1/inbox") {
			continue
		}
		pathItem := rawPathItem.(map[string]any)
		for _, method := range []string{"get", "post", "put", "patch", "delete"} {
			operation, ok := pathItem[method].(map[string]any)
			if !ok {
				continue
			}
			checked++
			if operationUsesVisibilityParameter(operation) {
				t.Errorf("%s %s must keep the personal/shared notification contract and omit project visibility", strings.ToUpper(method), path)
			}
		}
	}
	if checked == 0 {
		t.Fatal("notification and inbox operations were not found")
	}
}

func assertVisibilityParameterReference(t *testing.T, path string, operation map[string]any) {
	t.Helper()

	references := 0
	for _, raw := range operation["parameters"].([]any) {
		parameter, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if parameter["$ref"] == "#/components/parameters/Visibility" {
			references++
		}
		if parameter["name"] == "scope" {
			t.Errorf("GET %s still declares the legacy scope query parameter", path)
		}
		if parameter["name"] == "visibility" {
			t.Errorf("GET %s declares visibility inline instead of using the shared component", path)
		}
	}
	if references != 1 {
		t.Errorf("GET %s Visibility references = %d, want exactly one", path, references)
	}
}

func operationUsesVisibilityParameter(operation map[string]any) bool {
	parameters, _ := operation["parameters"].([]any)
	for _, raw := range parameters {
		parameter, ok := raw.(map[string]any)
		if ok && (parameter["$ref"] == "#/components/parameters/Visibility" || parameter["name"] == "visibility") {
			return true
		}
	}
	return false
}
