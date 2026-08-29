package api

import (
	"log/slog"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/telemetry"
)

func (h *Handlers) debugLog(format string, args ...any) {
	debugLogWithConfig(h.config, format, args...)
}

func debugLogWithConfig(cfg Config, format string, args ...any) {
	if !debugLogEnabled(cfg) {
		return
	}
	telemetry.Logger().Debug("API diagnostic checkpoint",
		slog.String("event.name", "api.debug.checkpoint"),
		slog.String("message.template", format),
		slog.Int("argument.count", len(args)),
	)
}

func debugLogEnabled(cfg Config) bool {
	switch strings.ToLower(strings.TrimSpace(cfg.LogLevel)) {
	case "debug":
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
