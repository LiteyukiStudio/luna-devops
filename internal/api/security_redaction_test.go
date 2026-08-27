package api

import (
	"strings"
	"testing"

	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
)

func TestRedactSensitiveLogContent(t *testing.T) {
	content := strings.Join([]string{
		"Authorization: Bearer bearer-secret",
		"git clone https://x-access-token:git-secret@github.com/acme/app.git",
		"registry password=registry-secret&token=query-secret",
		"plain build secret is build-secret",
	}, "\n")

	got := redactSensitiveLogContent(content, []string{"git-secret", "registry-secret", "query-secret", "build-secret"})

	for _, secret := range []string{"bearer-secret", "git-secret", "registry-secret", "query-secret", "build-secret"} {
		if strings.Contains(got, secret) {
			t.Fatalf("redacted log still contains secret %q: %s", secret, got)
		}
	}
	if count := strings.Count(got, redactedLogValue); count < 5 {
		t.Fatalf("redacted log replaced %d values, want at least 5: %s", count, got)
	}
	if !strings.Contains(got, "x-access-token:"+redactedLogValue+"@github.com") {
		t.Fatalf("redacted git URL did not keep a usable shape: %s", got)
	}
}

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
	for _, expected := range []string{"pod=app-123", "container=resolved-default-container", "exitCode=0", "truncated=true", "durationMs=42", "commandBytes=", "commandSha256="} {
		if !strings.Contains(got, expected) {
			t.Fatalf("audit message missing %q: %s", expected, got)
		}
	}
}
