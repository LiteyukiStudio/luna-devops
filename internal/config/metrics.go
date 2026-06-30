package config

import "strings"

func normalizeMetricsPath(value string) string {
	path := strings.TrimSpace(value)
	if path == "" {
		return "/metrics"
	}
	if !strings.HasPrefix(path, "/") {
		return "/" + path
	}
	return path
}
