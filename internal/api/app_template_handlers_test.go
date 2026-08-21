package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestListAppTemplatesFiltersSummariesByQueryAndCategory(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handlers := &Handlers{}

	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/app-templates?query=postgres&category=database", nil)
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
