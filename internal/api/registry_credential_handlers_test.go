package api

import (
	"context"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestListRegistryCredentialsAcceptsOptionalProjectFilterInOpenAPI(t *testing.T) {
	document := readOpenAPIDocument(t, apiRepositoryRoot(t)+"/openapi/openapi.yaml")
	paths := document["paths"].(map[string]any)
	operation := paths["/api/v1/registries/{registryId}/credentials"].(map[string]any)["get"].(map[string]any)
	for _, raw := range operation["parameters"].([]any) {
		parameter, ok := raw.(map[string]any)
		if !ok || parameter["name"] != "projectId" {
			continue
		}
		schema := parameter["schema"].(map[string]any)
		if schema["type"] != "string" || parameter["required"] == true {
			t.Fatalf("projectId parameter = %#v, want optional string", parameter)
		}
		return
	}
	t.Fatal("listRegistryCredentials is missing optional projectId parameter")
}

func TestListRegistryCredentialsUsesUnifiedProjectVisibility(t *testing.T) {
	function := parseAPIFunction(t, "registry_credential_handlers.go", "ListRegistryCredentials")
	calls := calledFunctions(function.Body)
	if !calls["Query"] || !calls["applyScopedResourceVisibility"] {
		t.Fatal("ListRegistryCredentials must pass projectId through the shared project visibility filter")
	}

	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=127.0.0.1 user=test password=test dbname=test port=1 sslmode=disable",
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("open dry-run database: %v", err)
	}
	query := (&Handlers{db: db}).applyScopedResourceVisibilityForProject(
		db.WithContext(context.Background()).Model(&model.RegistryCredential{}),
		scopedResourceRegistryCredential,
		model.User{ID: "usr_current"},
		"prj_current",
		context.Background(),
	)
	var credentials []model.RegistryCredential
	statement := query.Where("registry_id = ?", "areg_current").Find(&credentials).Statement
	explained := db.Dialector.Explain(statement.SQL.String(), statement.Vars...)

	for _, expected := range []string{
		"registry_id = 'areg_current'",
		"scope = 'global'",
		"scope = 'user' and owner_ref = 'usr_current'",
		"scope = 'project'",
		"resource_type = 'registry_credential'",
		"project_id = 'prj_current'",
	} {
		if !strings.Contains(explained, expected) {
			t.Fatalf("project-scoped credential query is missing %q: %s", expected, explained)
		}
	}
	if strings.Contains(explained, "prj_other") {
		t.Fatalf("project-scoped credential query includes another project: %s", explained)
	}
}
