package observation

import "time"

const (
	StatusReady         = "ready"
	StatusScaledToZero  = "scaled-to-zero"
	StatusDegraded      = "degraded"
	StatusProgressing   = "progressing"
	StatusNotFound      = "not-found"
	StatusNotConfigured = "not-configured"
	StatusUnavailable   = "unavailable"
	StatusUnknown       = "unknown"
	StatusDeclared      = "declared"
	// StatusDisabled marks a resource the user disabled; it never participates
	// in health aggregation even when its workload is still observed.
	StatusDisabled = "disabled"
)

// Result describes a point-in-time observation from an upstream system.
// It is response-only data and must never be persisted as current state.
type Result struct {
	Status     string    `json:"status"`
	Code       string    `json:"observationCode,omitempty"`
	ObservedAt time.Time `json:"observedAt"`
}
