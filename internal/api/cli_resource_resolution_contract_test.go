package api

import (
	"path/filepath"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/aitool"
)

func TestCLIResourceExactFiltersStayInOpenAPIAndPlatformCatalog(t *testing.T) {
	document := readOpenAPIDocument(t, filepath.Join(apiRepositoryRoot(t), "openapi", "openapi.yaml"))
	tests := []struct {
		path        string
		operationID string
		parameter   string
		scope       string
		maximum     float64
	}{
		{path: "/api/v1/projects", operationID: "listProjects", parameter: "identifier", scope: "project:read", maximum: 22},
		{path: "/api/v1/projects/{projectId}/applications", operationID: "listApplications", parameter: "identifier", scope: "application:read", maximum: 22},
		{path: "/api/v1/projects/{projectId}/applications/{applicationId}/deployment-targets", operationID: "listDeploymentTargets", parameter: "stage", scope: "deployment:read", maximum: 12},
	}

	catalog, err := aitool.PlatformCatalog()
	if err != nil {
		t.Fatal(err)
	}
	byOperationID := make(map[string]aitool.OpenAPIOperation, len(catalog))
	for _, operation := range catalog {
		byOperationID[operation.OperationID] = operation
	}

	for _, test := range tests {
		t.Run(test.operationID, func(t *testing.T) {
			operation := openAPIOperationAt(t, document, test.path, "get")
			parameter := openAPIParametersByName(t, operation)[test.parameter]
			if parameter == nil || parameter["in"] != "query" {
				t.Fatalf("%s is missing query parameter %q", test.operationID, test.parameter)
			}
			schema, _ := parameter["schema"].(map[string]any)
			if schema["minimum"] != nil || schema["minLength"] != float64(2) || schema["maxLength"] != test.maximum {
				t.Fatalf("%s.%s schema = %#v", test.operationID, test.parameter, schema)
			}
			if schema["pattern"] != "^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$" {
				t.Fatalf("%s.%s pattern = %#v", test.operationID, test.parameter, schema["pattern"])
			}

			catalogOperation, ok := byOperationID[test.operationID]
			if !ok {
				t.Fatalf("%s is missing from PlatformCatalog", test.operationID)
			}
			if len(catalogOperation.RequiredScopes) != 1 || catalogOperation.RequiredScopes[0] != test.scope {
				t.Fatalf("%s scopes = %v, want [%s]", test.operationID, catalogOperation.RequiredScopes, test.scope)
			}
			if _, ok := catalogOperation.InputSchema["properties"].(map[string]any)[test.parameter]; !ok {
				t.Fatalf("%s PlatformCatalog input is missing %q", test.operationID, test.parameter)
			}
		})
	}
}

func TestDeploymentTargetTerminalOpenAPIAndRouterContract(t *testing.T) {
	repositoryRoot := apiRepositoryRoot(t)
	document := readOpenAPIDocument(t, filepath.Join(repositoryRoot, "openapi", "openapi.yaml"))
	registered := registeredRouterOperations(t, filepath.Join(repositoryRoot, "internal", "api", "router.go"))
	tests := []struct {
		path        string
		method      string
		operationID string
	}{
		{
			path:        "/api/v1/projects/{projectId}/applications/{applicationId}/deployment-targets/{targetId}/terminal/authorize",
			method:      "post",
			operationID: "authorizeDeploymentTargetRuntimeTerminal",
		},
		{
			path:        "/api/v1/projects/{projectId}/applications/{applicationId}/deployment-targets/{targetId}/terminal",
			method:      "get",
			operationID: "streamDeploymentTargetRuntimeTerminal",
		},
		{
			path:        "/api/v1/projects/{projectId}/releases/{releaseId}/terminal/authorize",
			method:      "post",
			operationID: "authorizeReleaseRuntimeTerminal",
		},
		{
			path:        "/api/v1/projects/{projectId}/releases/{releaseId}/terminal",
			method:      "get",
			operationID: "streamReleaseRuntimeTerminal",
		},
	}

	for _, test := range tests {
		t.Run(test.operationID, func(t *testing.T) {
			operation := openAPIOperationAt(t, document, test.path, test.method)
			if operation["operationId"] != test.operationID {
				t.Fatalf("operationId = %#v, want %s", operation["operationId"], test.operationID)
			}
			cli, _ := operation["x-luna-cli"].(map[string]any)
			scopes, ok := schemaStringList(cli["requiredScopes"])
			if !ok || len(scopes) != 1 || scopes[0] != "deployment:exec" {
				t.Fatalf("%s requiredScopes = %#v", test.operationID, cli["requiredScopes"])
			}
			if cli["classification"] != "protocol-adapter" || cli["hidden"] != true {
				t.Fatalf("%s CLI protocol metadata = %#v", test.operationID, cli)
			}
			if test.method == "post" {
				responses, _ := operation["responses"].(map[string]any)
				response, _ := responses["200"].(map[string]any)
				headers, _ := response["headers"].(map[string]any)
				cacheControl, _ := headers["Cache-Control"].(map[string]any)
				if cacheControl == nil {
					t.Fatalf("%s does not declare the no-store response header", test.operationID)
				}
				schema, _ := cacheControl["schema"].(map[string]any)
				values, ok := schemaStringList(schema["enum"])
				if !ok || len(values) != 1 || values[0] != "no-store" {
					t.Fatalf("%s Cache-Control contract = %#v", test.operationID, cacheControl)
				}
			}
			route := map[string]string{"get": "GET", "post": "POST"}[test.method] + " " + normalizeOpenAPIPath(test.path)
			if !registered[route] {
				t.Fatalf("router is missing %s", route)
			}
		})
	}
}

func TestRuntimeTerminalTicketTransportsStayExplicit(t *testing.T) {
	document := readOpenAPIDocument(t, filepath.Join(apiRepositoryRoot(t), "openapi", "openapi.yaml"))
	components, _ := document["components"].(map[string]any)
	schemes, _ := components["securitySchemes"].(map[string]any)
	headerScheme, _ := schemes["RuntimeTerminalTicket"].(map[string]any)
	legacyScheme, _ := schemes["LegacyRuntimeTerminalTicket"].(map[string]any)
	if headerScheme["type"] != "apiKey" || headerScheme["in"] != "header" || headerScheme["name"] != "X-Luna-Terminal-Ticket" {
		t.Fatalf("runtime terminal header scheme = %#v", headerScheme)
	}
	if legacyScheme["type"] != "apiKey" || legacyScheme["in"] != "query" || legacyScheme["name"] != "ticket" {
		t.Fatalf("legacy terminal ticket scheme = %#v", legacyScheme)
	}

	for _, test := range []struct {
		path   string
		legacy bool
	}{
		{path: "/api/v1/projects/{projectId}/applications/{applicationId}/deployment-targets/{targetId}/terminal"},
		{path: "/api/v1/projects/{projectId}/releases/{releaseId}/terminal", legacy: true},
		{path: "/api/v1/runtime/clusters/{clusterId}/pods/terminal", legacy: true},
	} {
		operation := openAPIOperationAt(t, document, test.path, "get")
		security, _ := operation["security"].([]any)
		securityNames := make(map[string]bool, len(security))
		for _, raw := range security {
			requirement, _ := raw.(map[string]any)
			for name := range requirement {
				securityNames[name] = true
			}
		}
		if !securityNames["SessionCookie"] || !securityNames["RuntimeTerminalTicket"] || securityNames["LegacyRuntimeTerminalTicket"] != test.legacy {
			t.Fatalf("%s security = %#v", test.path, security)
		}

		parameters := openAPIParametersByName(t, operation)
		legacyParameter := parameters["ticket"]
		if test.legacy {
			if legacyParameter == nil || legacyParameter["in"] != "query" || legacyParameter["deprecated"] != true {
				t.Fatalf("%s legacy ticket parameter = %#v", test.path, legacyParameter)
			}
		} else if legacyParameter != nil {
			t.Fatalf("%s unexpectedly accepts a query ticket", test.path)
		}
	}
}
