package api

import (
	"path/filepath"
	"testing"
)

func TestAIProviderConnectionOpenAPIDocumentsNotConfigured(t *testing.T) {
	t.Parallel()

	tests := []struct {
		document string
		path     string
	}{
		{document: "openapi.yaml", path: "/api/v1/configs/ai/provider/test"},
		{document: "agent-internal.yaml", path: "/internal/v1/provider/test"},
	}
	for _, test := range tests {
		t.Run(test.document, func(t *testing.T) {
			document := readOpenAPIDocument(t, filepath.Join(apiRepositoryRoot(t), "openapi", test.document))
			operation := openAPIOperationAt(t, document, test.path, "post")
			responses, _ := operation["responses"].(map[string]any)
			if responses["409"] == nil {
				t.Fatalf("%s must document the Provider not-configured response", test.path)
			}
		})
	}
}
