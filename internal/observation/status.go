package observation

import "time"

const (
	StatusReady         = "ready"
	StatusDegraded      = "degraded"
	StatusProgressing   = "progressing"
	StatusNotFound      = "not-found"
	StatusNotConfigured = "not-configured"
	StatusUnavailable   = "unavailable"
	StatusUnknown       = "unknown"
	StatusDeclared      = "declared"
)

// Result describes a point-in-time observation from an upstream system.
// It is response-only data and must never be persisted as current state.
type Result struct {
	Status     string    `json:"status"`
	Code       string    `json:"observationCode,omitempty"`
	ObservedAt time.Time `json:"observedAt"`
}
