package api

import (
	"reflect"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
)

func TestNormalizeBuildTimeoutSecondsValue(t *testing.T) {
	tests := []struct {
		name  string
		input int
		want  int
	}{
		{name: "zero uses default", input: 0, want: defaultBuildTimeoutSeconds},
		{name: "negative uses default", input: -1, want: defaultBuildTimeoutSeconds},
		{name: "positive is preserved", input: 900, want: 900},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := normalizeBuildTimeoutSecondsValue(test.input); got != test.want {
				t.Fatalf("normalizeBuildTimeoutSecondsValue(%d) = %d, want %d", test.input, got, test.want)
			}
		})
	}
}

func TestNormalizeDeploymentServicePortName(t *testing.T) {
	tests := []struct {
		name  string
		value string
		port  int
		index int
		want  string
	}{
		{name: "normalizes separators", value: " Web_API ", port: 8080, want: "web-api"},
		{name: "first empty name uses http", port: 8080, want: "http"},
		{name: "later empty name uses port", port: 9090, index: 1, want: "port-9090"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := normalizeDeploymentServicePortName(test.value, test.port, test.index); got != test.want {
				t.Fatalf("normalizeDeploymentServicePortName(%q, %d, %d) = %q, want %q", test.value, test.port, test.index, got, test.want)
			}
		})
	}
}

func TestRuntimeConfigRefInputs(t *testing.T) {
	input := deploymentTargetInput{
		RuntimeConfigRefs: []deploymentRuntimeConfigRefInput{
			{SetID: " set-a ", Mode: string(model.RuntimeConfigRefModeSnapshot)},
			{SetID: "set-a", Mode: string(model.RuntimeConfigRefModeLive)},
			{SetID: "", Mode: string(model.RuntimeConfigRefModeLive)},
			{SetID: "set-b", Mode: string(model.RuntimeConfigRefModeLive)},
		},
	}
	want := []deploymentRuntimeConfigRefInput{
		{SetID: "set-a", Mode: string(model.RuntimeConfigRefModeSnapshot)},
		{SetID: "set-b", Mode: string(model.RuntimeConfigRefModeLive)},
	}

	if got := runtimeConfigRefInputs(input); !reflect.DeepEqual(got, want) {
		t.Fatalf("runtimeConfigRefInputs() = %#v, want %#v", got, want)
	}
}

func TestNormalizeSecretRefsInput(t *testing.T) {
	if got := normalizeSecretRefsInput("  {}  "); got != "" {
		t.Fatalf("normalizeSecretRefsInput(empty object) = %q, want empty", got)
	}
	if got := normalizeSecretRefsInput("  {\"TOKEN\":\"secret-id\"}  "); got != "{\"TOKEN\":\"secret-id\"}" {
		t.Fatalf("normalizeSecretRefsInput(value) = %q", got)
	}
}
