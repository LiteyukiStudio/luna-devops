package platformevent

type CatalogEntry struct {
	Type              string `json:"type"`
	Category          string `json:"category"`
	DefaultSeverity   string `json:"defaultSeverity"`
	RecommendedNotify bool   `json:"recommendedNotify"`
}

var catalog = []CatalogEntry{
	{Type: "build.started", Category: "build", DefaultSeverity: "info"},
	{Type: "build.succeeded", Category: "build", DefaultSeverity: "info"},
	{Type: "build.failed", Category: "build", DefaultSeverity: "error", RecommendedNotify: true},
	{Type: "build.webhook_rejected", Category: "build", DefaultSeverity: "warning", RecommendedNotify: true},
	{Type: "release.started", Category: "release", DefaultSeverity: "info"},
	{Type: "release.succeeded", Category: "release", DefaultSeverity: "info"},
	{Type: "release.failed", Category: "release", DefaultSeverity: "error", RecommendedNotify: true},
	{Type: "runtime.pod_oom_killed", Category: "runtime", DefaultSeverity: "error", RecommendedNotify: true},
	{Type: "runtime.pod_crash_loop", Category: "runtime", DefaultSeverity: "error", RecommendedNotify: true},
	{Type: "runtime.pod_start_failed", Category: "runtime", DefaultSeverity: "error", RecommendedNotify: true},
	{Type: "runtime.workload_degraded", Category: "runtime", DefaultSeverity: "error", RecommendedNotify: true},
	{Type: "runtime.recovered", Category: "runtime", DefaultSeverity: "info", RecommendedNotify: true},
	{Type: "hook.started", Category: "hook", DefaultSeverity: "info"},
	{Type: "hook.succeeded", Category: "hook", DefaultSeverity: "info"},
	{Type: "hook.failed", Category: "hook", DefaultSeverity: "error", RecommendedNotify: true},
	{Type: "gateway.applied", Category: "gateway", DefaultSeverity: "info"},
	{Type: "gateway.apply_failed", Category: "gateway", DefaultSeverity: "error", RecommendedNotify: true},
	{Type: "certificate.pending", Category: "certificate", DefaultSeverity: "warning"},
	{Type: "certificate.issued", Category: "certificate", DefaultSeverity: "info"},
	{Type: "certificate.renewed", Category: "certificate", DefaultSeverity: "info"},
	{Type: "certificate.failed", Category: "certificate", DefaultSeverity: "error", RecommendedNotify: true},
	{Type: "certificate.expired", Category: "certificate", DefaultSeverity: "error", RecommendedNotify: true},
	{Type: "service_binding.created", Category: "service_binding", DefaultSeverity: "info"},
	{Type: "service_binding.updated", Category: "service_binding", DefaultSeverity: "info"},
	{Type: "service_binding.deleted", Category: "service_binding", DefaultSeverity: "info"},
	{Type: "service_binding.invalid", Category: "service_binding", DefaultSeverity: "error", RecommendedNotify: true},
	{Type: "service_binding.recovered", Category: "service_binding", DefaultSeverity: "info"},
}

func Catalog() []CatalogEntry {
	return append([]CatalogEntry(nil), catalog...)
}
