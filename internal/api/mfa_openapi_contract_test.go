package api

import (
	"path/filepath"
	"testing"
)

func TestMFAVerifyOpenAPIRequiresStepUpAssertionID(t *testing.T) {
	document := readOpenAPIDocument(t, filepath.Join(apiRepositoryRoot(t), "openapi", "openapi.yaml"))
	schemas := document["components"].(map[string]any)["schemas"].(map[string]any)
	result := schemas["MFAVerifyResult"].(map[string]any)
	required, ok := schemaStringList(result["required"])
	requiredSet := make(map[string]struct{}, len(required))
	for _, field := range required {
		requiredSet[field] = struct{}{}
	}
	if !ok || len(requiredSet) != 3 {
		t.Fatalf("MFAVerifyResult.required = %#v", result["required"])
	}
	for _, field := range []string{"verified", "purpose", "stepUpAssertionId"} {
		if _, exists := requiredSet[field]; !exists {
			t.Fatalf("MFAVerifyResult.required is missing %q: %#v", field, result["required"])
		}
	}
	properties := result["properties"].(map[string]any)
	assertion, ok := properties["stepUpAssertionId"].(map[string]any)
	if !ok || assertion["type"] != "string" {
		t.Fatalf("MFAVerifyResult.stepUpAssertionId = %#v", properties["stepUpAssertionId"])
	}
}
