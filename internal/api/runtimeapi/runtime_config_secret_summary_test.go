package runtimeapi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
)

func TestProjectRuntimeConfigSetRuntimeSecretsSummaryDoesNotExposeReferences(t *testing.T) {
	summary := projectRuntimeConfigSetRuntimeSecretsSummaryFor(model.ProjectRuntimeConfigSet{
		SecretRefs: `{"TOKEN":"secret-id:one","EMPTY":"","PASSWORD":"secret-id:two"}`,
	})

	if len(summary.EnvironmentVariables) != 2 {
		t.Fatalf("summary variables = %#v", summary.EnvironmentVariables)
	}
	for _, variable := range summary.EnvironmentVariables {
		if variable.ValueMode != runtimeEnvironmentValueModeSecret || !variable.Configured || variable.Value != "" {
			t.Fatalf("unexpected secret summary item: %#v", variable)
		}
	}
	encoded, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "secret-id:") {
		t.Fatalf("secret reference leaked: %s", encoded)
	}
}
