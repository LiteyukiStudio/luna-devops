package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/aitool"
)

func TestBuildAIPlatformRequestBindsPathQueryAndBody(t *testing.T) {
	operation := aitool.OpenAPIOperation{
		OperationID: "updateApplication", Method: http.MethodPut,
		Path: "/api/v1/projects/{projectId}/applications/{applicationId}",
		Parameters: []aitool.OpenAPIParameter{
			{Name: "projectId", In: "path", Required: true},
			{Name: "applicationId", In: "path", Required: true},
			{Name: "dryRun", In: "query"},
		},
		RequestBody: true, RequestRequired: true, RequestType: "application/json",
	}
	target, body, err := buildAIPlatformRequest(operation, map[string]any{
		"projectId": "prj_1", "applicationId": "app/1", "dryRun": true,
		"body": map[string]any{"name": "Example"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if target != "/api/v1/projects/prj_1/applications/app%2F1?dryRun=true" {
		t.Fatalf("target = %q", target)
	}
	encoded, _ := io.ReadAll(body)
	var payload map[string]any
	if json.Unmarshal(encoded, &payload) != nil || payload["name"] != "Example" {
		t.Fatalf("body = %s", encoded)
	}
}

func TestBuildAIPlatformRequestRejectsUnknownOrMissingArguments(t *testing.T) {
	operation := aitool.OpenAPIOperation{
		OperationID: "getProject", Method: http.MethodGet,
		Path:       "/api/v1/projects/{projectId}",
		Parameters: []aitool.OpenAPIParameter{{Name: "projectId", In: "path", Required: true}},
	}
	for _, arguments := range []map[string]any{
		{},
		{"projectId": "prj_1", "unexpected": "value"},
	} {
		if _, _, err := buildAIPlatformRequest(operation, arguments); err == nil {
			t.Fatalf("arguments should be rejected: %#v", arguments)
		}
	}
}

func TestAppendAIQueryValuePreservesRepeatedValues(t *testing.T) {
	operation := aitool.OpenAPIOperation{
		OperationID: "listExample", Method: http.MethodGet, Path: "/api/v1/example",
		Parameters: []aitool.OpenAPIParameter{{Name: "status", In: "query"}},
	}
	target, _, err := buildAIPlatformRequest(operation, map[string]any{"status": []any{"ready", "failed"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(target, "status=ready") || !strings.Contains(target, "status=failed") {
		t.Fatalf("target = %q", target)
	}
}
