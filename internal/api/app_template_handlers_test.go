package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAppTemplateInstallOpenAPIContractIsAgentRepairable(t *testing.T) {
	document := readOpenAPIDocument(t, apiRepositoryRoot(t)+"/openapi/openapi.yaml")
	paths := document["paths"].(map[string]any)
	operation := paths["/api/v1/projects/{projectId}/app-templates/{templateId}/install"].(map[string]any)["post"].(map[string]any)
	cli := operation["x-luna-cli"].(map[string]any)
	scopes, _ := schemaStringList(cli["requiredScopes"])
	if cli["command"] != "app-template.install" || cli["classification"] != "business-command" || cli["risk"] != "medium" || cli["agentAllowed"] != true || !reflect.DeepEqual(scopes, []string{"project:write"}) {
		t.Fatalf("installAppTemplate CLI metadata = %#v", cli)
	}
	agent := operation["x-luna-agent"].(map[string]any)
	for _, field := range []string{"purpose", "aliases", "avoidWhen", "preconditions", "successEvidence"} {
		if agent[field] == nil {
			t.Fatalf("installAppTemplate Agent metadata is missing %s: %#v", field, agent)
		}
	}
	schemas := document["components"].(map[string]any)["schemas"].(map[string]any)
	input := schemas["AppTemplateInstallInput"].(map[string]any)
	stage := input["properties"].(map[string]any)["stage"].(map[string]any)
	values, ok := schemaStringList(stage["enum"])
	if !ok || !reflect.DeepEqual(values, publicDeploymentStages) || !strings.Contains(stage["description"].(string), "default") {
		t.Fatalf("AppTemplateInstallInput.stage = %#v", stage)
	}
}

func TestDeploymentStageInvalidErrorIsStructuredAndNotRetryable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/projects/prj/app-templates/redis/install", nil)
	writeDeploymentStageInvalid(ctx, "stage", "deployment stage must be canonical")

	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusBadRequest || response["code"] != "deployment.stage_invalid" || response["path"] != "stage" || response["retryable"] != false {
		t.Fatalf("structured stage error = %d %#v", recorder.Code, response)
	}
	allowed, ok := schemaStringList(response["allowedValues"])
	if !ok || !reflect.DeepEqual(allowed, publicDeploymentStages) {
		t.Fatalf("allowedValues = %#v", response["allowedValues"])
	}
}

func TestListAppTemplatesFiltersSummariesByQueryAndCategory(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handlers := &Handlers{}

	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/app-templates?query=transactional&category=database", nil)
	handlers.ListAppTemplates(ctx)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	var items []map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0]["id"] != "postgresql" {
		t.Fatalf("items = %#v", items)
	}
	if _, exists := items[0]["values"]; exists {
		t.Fatal("list response must not embed full template values")
	}
	if items[0]["valueCount"] == nil || items[0]["requiredValueCount"] == nil {
		t.Fatalf("summary counts missing: %#v", items[0])
	}

	response = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/app-templates?query=Dify", nil)
	handlers.ListAppTemplates(ctx)
	if response.Code != http.StatusOK || response.Body.String() != "[]" {
		t.Fatalf("Dify no-match response = %d %s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/app-templates?category=storage", nil)
	handlers.ListAppTemplates(ctx)
	if response.Code != http.StatusOK {
		t.Fatalf("storage status = %d body=%s", response.Code, response.Body.String())
	}
	items = nil
	if err := json.Unmarshal(response.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("storage items = %#v, want Garage, Verdaccio, and Docker Registry", items)
	}
	for _, item := range items {
		if item["category"] != "storage" {
			t.Fatalf("storage filter returned category %v", item["category"])
		}
	}
}

func TestGetAppTemplateReturnsSanitizedFullDefinition(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/app-templates/mongodb", nil)
	ctx.Params = gin.Params{{Key: "templateId", Value: "mongodb"}}
	(&Handlers{}).GetAppTemplate(ctx)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	var template map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &template); err != nil {
		t.Fatal(err)
	}
	values, ok := template["values"].([]any)
	if !ok || len(values) == 0 {
		t.Fatalf("values = %#v", template["values"])
	}
	for _, raw := range values {
		value := raw.(map[string]any)
		if value["secret"] == true && value["default"] != "" {
			t.Fatalf("secret default leaked: %#v", value)
		}
	}
	for _, internal := range []string{"env", "secretEnv", "configFiles", "secretFiles"} {
		if _, exists := template[internal]; exists {
			t.Fatalf("internal rendering field %s leaked", internal)
		}
	}
}
