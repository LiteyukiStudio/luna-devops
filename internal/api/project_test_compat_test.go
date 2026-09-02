package api

import (
	"time"

	"github.com/LiteyukiStudio/devops/internal/api/projectapi"
	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
)

type projectPinResponse = projectapi.ProjectPinResponse

func projectPageQuery(query *gorm.DB, pagination paginationParams) *gorm.DB {
	return projectapi.ProjectPageQuery(query, pagination)
}

func projectPinResponseFrom(project model.Project, pin model.ProjectPin, dashboardOrder int) projectPinResponse {
	return projectapi.ProjectPinResponseFrom(project, pin, dashboardOrder)
}

func normalizedProjectOrderIDs(values []string) []string {
	return projectapi.NormalizedProjectOrderIDs(values)
}

func topologyOrigins(raw string) map[string]bool {
	return projectapi.TopologyOrigins(raw)
}

type continuousAuthorizationState projectapi.ContinuousAuthorizationState
type runtimeTerminalAuthorizationState = continuousAuthorizationState

func (state continuousAuthorizationState) active(binding continuousAuthorizationBinding, now time.Time) bool {
	return projectapi.ContinuousAuthorizationStateActive(projectapi.ContinuousAuthorizationState(state), binding, now)
}

func (state continuousAuthorizationState) identityActive(binding continuousAuthorizationBinding, now time.Time) bool {
	return projectapi.ContinuousAuthorizationStateIdentityActive(projectapi.ContinuousAuthorizationState(state), binding, now)
}
