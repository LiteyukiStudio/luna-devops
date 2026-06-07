package api

import (
	"log"
	"os"
	"strings"
)

func debugLog(format string, args ...any) {
	if !debugLogEnabled() {
		return
	}
	log.Printf("[DEBUG] "+format, args...)
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
