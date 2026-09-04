package projectapi

import (
	"context"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	projectservice "github.com/LiteyukiStudio/devops/internal/project"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ContinuousAuthorizationBinding = continuousAuthorizationBinding
type ContinuousAuthorizationState = continuousAuthorizationState
type ProjectPinResponse = projectPinResponse

func ResolveListVisibility(ctx *gin.Context, user model.User) (projectservice.ListVisibility, bool) {
	return resolveListVisibility(ctx, user)
}

func NormalizeOwnerScope(value string) string {
	return normalizeOwnerScope(value)
}

func ApplyScopedResourceVisibilityQuery(query *gorm.DB, bindingDB *gorm.DB, resourceType, userID, projectID string, projectIDs []string, includeAllProjects, includeUnboundProjectScope bool) *gorm.DB {
	return applyScopedResourceVisibilityQuery(query, bindingDB, resourceType, userID, projectID, projectIDs, includeAllProjects, includeUnboundProjectScope)
}

func SortedProjectIDs(projectIDs []string) []string {
	return sortedProjectIDs(projectIDs)
}

func WriteProjectAuthorizationError(ctx *gin.Context, err error) {
	writeProjectAuthorizationError(ctx, err)
}

func NormalizeHookPhase(value string) string {
	return normalizeHookPhase(value)
}

func TopologyOrigins(raw string) map[string]bool {
	return topologyOrigins(raw)
}

func ProjectPageQuery(query *gorm.DB, pagination paginationParams) *gorm.DB {
	return projectPageQuery(query, pagination)
}

func ProjectPinResponseFrom(project model.Project, pin model.ProjectPin, dashboardOrder int) ProjectPinResponse {
	return projectPinResponseFrom(project, pin, dashboardOrder)
}

func NormalizedProjectOrderIDs(values []string) []string {
	return normalizedProjectOrderIDs(values)
}

func ContinuousAuthorizationBindingForAccessToken(userID string, token model.AccessToken) ContinuousAuthorizationBinding {
	return continuousAuthorizationBindingForAccessToken(userID, token)
}

func ContinuousAuthorizationStateActive(state ContinuousAuthorizationState, binding ContinuousAuthorizationBinding, now time.Time) bool {
	return state.active(binding, now)
}

func ContinuousAccessTokenSubject(tokenID string) string {
	return continuousAccessTokenSubject(tokenID)
}

func ContinuousAuthorizationAccessTokenID(subject string) (string, bool) {
	return continuousAuthorizationAccessTokenID(subject)
}

func WriteContinuousAuthorizationRevoked(ctx *gin.Context) {
	writeContinuousAuthorizationRevoked(ctx)
}

func ReplaceRequestContext(ctx *gin.Context, requestCtx context.Context) func() {
	return replaceRequestContext(ctx, requestCtx)
}

func (h *Handler) CurrentContinuousAuthorizationBinding(ctx *gin.Context, user model.User) (ContinuousAuthorizationBinding, bool) {
	return h.currentContinuousAuthorizationBinding(ctx, user)
}

func (h *Handler) RequireContinuousAuthorizationBinding(ctx *gin.Context, user model.User) (ContinuousAuthorizationBinding, bool) {
	return h.requireContinuousAuthorizationBinding(ctx, user)
}

func (h *Handler) ContinuousAuthorizationCheckInterval() time.Duration {
	return h.continuousAuthorizationCheckInterval()
}

func (h *Handler) MonitorContinuousAuthorization(ctx context.Context, binding ContinuousAuthorizationBinding, authorizationAllowed func(context.Context, model.User) bool, revoke func()) (<-chan struct{}, bool) {
	return h.monitorContinuousAuthorization(ctx, binding, authorizationAllowed, revoke)
}

func (h *Handler) MonitorContinuousAuthorizationWithInterval(ctx context.Context, binding ContinuousAuthorizationBinding, authorizationAllowed func(context.Context, model.User) bool, revoke func(), checkInterval time.Duration) (<-chan struct{}, bool) {
	return h.monitorContinuousAuthorizationWithInterval(ctx, binding, authorizationAllowed, revoke, checkInterval)
}

func (h *Handler) ContinuousAuthorizationActive(ctx context.Context, binding ContinuousAuthorizationBinding, authorizationAllowed func(context.Context, model.User) bool) bool {
	return h.continuousAuthorizationActive(ctx, binding, authorizationAllowed)
}

func (h *Handler) ProjectContinuousAuthorizationAllowed(ctx context.Context, user model.User, projectID string, action authz.Action) bool {
	return h.projectContinuousAuthorizationAllowed(ctx, user, projectID, action)
}
