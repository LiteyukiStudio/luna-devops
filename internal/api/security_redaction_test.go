package api

import (
	"strings"
	"testing"

	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
)

func TestRuntimeExecAuditMessageDoesNotIncludeCommand(t *testing.T) {
	command := "echo super-secret-token"
	got := runtimeExecAuditMessage(command, kubeprovider.RuntimeExecResult{
		Pod:       "app-123",
		Container: "resolved-default-container",
		ExitCode:  0,
		Truncated: true,
		Duration:  42,
	})

	if strings.Contains(got, command) || strings.Contains(got, "super-secret-token") {
		t.Fatalf("audit message leaked command: %s", got)
	}
	for _, expected := range []string{"pod=app-123", "container=resolved-default-container", "exitCode=0", "truncated=true", "durationMs=42", "commandBytes="} {
		if !strings.Contains(got, expected) {
			t.Fatalf("audit message missing %q: %s", expected, got)
		}
	}
	if strings.Contains(got, "commandSha256=") {
		t.Fatalf("audit message retained command fingerprint: %s", got)
	}
}
