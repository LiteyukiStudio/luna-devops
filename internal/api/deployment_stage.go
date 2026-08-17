package api

import (
	"strings"

	"github.com/LiteyukiStudio/devops/internal/model"
)

// normalizeStage is the single normalization boundary for deployment stages.
// Public writes must additionally call normalizePublicStage; persisted sys-*
// stages are reserved for platform-managed resources and are never accepted
// from public create, template, or deployment bundle inputs.
func normalizeStage(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "prod", "production":
		return "prod"
	case "dev", "staging", "test":
		return normalized
	case "":
		return model.DefaultDeploymentStage
	default:
		return normalized
	}
}

func normalizePublicStage(value string) (string, bool) {
	stage := normalizeStage(value)
	switch stage {
	case "dev", "test", "staging", "prod":
		return stage, true
	default:
		return stage, false
	}
}
