package api

import (
	"fmt"
	"strings"

	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
)

func runtimeExecAuditMessage(command string, result kubeprovider.RuntimeExecResult) string {
	command = strings.TrimSpace(command)
	return fmt.Sprintf(
		"pod=%s container=%s exitCode=%d truncated=%t durationMs=%d commandBytes=%d",
		strings.TrimSpace(result.Pod),
		strings.TrimSpace(result.Container),
		result.ExitCode,
		result.Truncated,
		result.Duration,
		len([]byte(command)),
	)
}
