package api

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/authz"
)

func TestBearerProtectedOpenAPIOperationsDeclareKnownScopes(t *testing.T) {
	document := readOpenAPIDocument(t, filepath.Join(apiRepositoryRoot(t), "openapi", "openapi.yaml"))
	paths := document["paths"].(map[string]any)
	checked := 0
	for path, rawPathItem := range paths {
		pathItem, ok := rawPathItem.(map[string]any)
		if !ok {
			continue
		}
		for _, method := range []string{"get", "post", "put", "patch", "delete"} {
			operation, ok := pathItem[method].(map[string]any)
			if !ok || !operationAllowsBearerToken(operation) {
				continue
			}
			checked++
			operationID, _ := operation["operationId"].(string)
			cli, _ := operation["x-luna-cli"].(map[string]any)
			scopes, _ := schemaStringList(cli["requiredScopes"])
			if len(scopes) == 0 {
				t.Errorf("%s %s (%s) allows BearerToken without x-luna-cli.requiredScopes", strings.ToUpper(method), path, operationID)
				continue
			}
			for _, scope := range scopes {
				if normalized := authz.NormalizeOAuthScope(scope); normalized != scope {
					t.Errorf("%s declares unknown BearerToken scope %q", operationID, scope)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("BearerToken OpenAPI operations were not found")
	}
}

func operationAllowsBearerToken(operation map[string]any) bool {
	security, _ := operation["security"].([]any)
	for _, rawRequirement := range security {
		requirement, ok := rawRequirement.(map[string]any)
		if ok && requirement["BearerToken"] != nil {
			return true
		}
	}
	return false
}
