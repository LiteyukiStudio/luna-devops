package api

import (
	"crypto/sha256"
	"fmt"
	"strings"

	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
)

func runtimeExecAuditMessage(command string, result kubeprovider.RuntimeExecResult) string {
	command = strings.TrimSpace(command)
	digest := sha256.Sum256([]byte(command))
	return fmt.Sprintf(
		"pod=%s container=%s exitCode=%d truncated=%t durationMs=%d commandBytes=%d commandSha256=%x",
		strings.TrimSpace(result.Pod),
		strings.TrimSpace(result.Container),
		result.ExitCode,
		result.Truncated,
		result.Duration,
		len([]byte(command)),
		digest,
	)
}
