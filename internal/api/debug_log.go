package api

import (
	"log/slog"
	"os"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/telemetry"
)

func debugLog(format string, args ...any) {
	if !debugLogEnabled() {
		return
	}
	telemetry.Logger().Debug("API diagnostic checkpoint",
		slog.String("event.name", "api.debug.checkpoint"),
		slog.String("message.template", format),
		slog.Int("argument.count", len(args)),
	)
}

func debugLogEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("LOG_LEVEL"))) {
	case "debug", "trace":
		return true
	}
	return false
}

func shortDebugHash(value string) string {
	if value == "" {
		return ""
	}
	hashed := hashToken(value)
	if len(hashed) <= 12 {
		return hashed
	}
	return hashed[:12]
}
