package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
)

func TestBuildEnvironmentResponseNeverExposesSecretReferences(t *testing.T) {
	response := buildEnvironmentConfigResponseFromModel(model.BuildEnvironmentConfig{
		Scope:      model.BuildEnvironmentScopeApplication,
		ScopeRef:   "app_test",
		Variables:  `{"PUBLIC_VALUE":"visible"}`,
		SecretRefs: `{"PRIVATE_VALUE":"secret-value:private-ref"}`,
	})

	content, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "private-ref") {
		t.Fatalf("secret reference leaked in response: %s", content)
	}
	if !response.Secrets["PRIVATE_VALUE"] || response.Variables["PUBLIC_VALUE"] != "visible" {
		t.Fatalf("unexpected safe response: %#v", response)
	}
}
