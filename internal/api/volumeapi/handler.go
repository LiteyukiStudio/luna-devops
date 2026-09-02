package volumeapi

import (
	"context"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/api/projectapi"
	transportapi "github.com/LiteyukiStudio/devops/internal/api/transport"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/volume"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Host interface {
	DBWithContext(ctx context.Context) *gorm.DB
	AuthorizeProject(ctx *gin.Context, action authz.Action) (model.User, model.Project, bool)
	ProjectMemberActionAllowed(ctx *gin.Context, projectID, userID string, action authz.Action) (bool, bool)
	ProjectRoleActionAllowed(ctx context.Context, user model.User, projectID string, action authz.Action) (bool, error)
	ProjectAuthorizer(ctx context.Context) authz.ProjectAuthorizer
	RuntimeClusterForProjectUse(ctx *gin.Context, user model.User, projectID, clusterID string) (model.RuntimeCluster, bool)
	EnsureBillingAllowsDeployChange(ctx *gin.Context, projectID string) bool
	EnsureBillingAllowsManagedVolumeChange(ctx *gin.Context, projectID string) bool
	AuditWithContext(userID, action, resource string, success bool, message string, ctx context.Context)
	CurrentAccessTokenFromContext(ctx *gin.Context) (model.AccessToken, bool)
	AccessTokenAllows(scopeText, required string) bool
	RequestUsesBearerToken(ctx *gin.Context) bool
	RequireContinuousAuthorizationBinding(ctx *gin.Context, user model.User) (projectapi.ContinuousAuthorizationBinding, bool)
	CurrentInteractiveAuthorizationBinding(ctx *gin.Context, user model.User) (projectapi.ContinuousAuthorizationBinding, bool)
	MonitorContinuousAuthorization(ctx context.Context, binding projectapi.ContinuousAuthorizationBinding, authorizationAllowed func(context.Context, model.User) bool, revoke func()) (<-chan struct{}, bool)
	ProjectContinuousAuthorizationAllowed(ctx context.Context, user model.User, projectID string, action authz.Action) bool
	ResourceCanMutateDuringDelete(status string) bool
}

type Dependencies struct {
	Volumes          *volume.Service
	Clusters         ProjectVolumeClusterService
	Content          VolumeTransferContentService
	TransferMaxBytes int64
	TransferEnabled  bool
}

type Handler struct {
	host                   Host
	volumes                *volume.Service
	volumeClusters         ProjectVolumeClusterService
	volumeContent          VolumeTransferContentService
	volumeTransferMaxBytes int64
	volumeTransferEnabled  bool
}

// Handlers keeps the migrated receiver declarations compact while Handler is
// the domain entry point used by the root API bridge.
type Handlers = Handler

func New(host Host, dependencies Dependencies) *Handler {
	return &Handler{
		host:                   host,
		volumes:                dependencies.Volumes,
		volumeClusters:         dependencies.Clusters,
		volumeContent:          dependencies.Content,
		volumeTransferMaxBytes: dependencies.TransferMaxBytes,
		volumeTransferEnabled:  dependencies.TransferEnabled,
	}
}

func (h *Handler) dbWithContext(ctx context.Context) *gorm.DB {
	return h.host.DBWithContext(ctx)
}

func (h *Handler) authorizeProject(ctx *gin.Context, action authz.Action) (model.User, model.Project, bool) {
	return h.host.AuthorizeProject(ctx, action)
}

func (h *Handler) projectMemberActionAllowed(ctx *gin.Context, projectID, userID string, action authz.Action) (bool, bool) {
	return h.host.ProjectMemberActionAllowed(ctx, projectID, userID, action)
}

func (h *Handler) projectRoleActionAllowed(ctx context.Context, user model.User, projectID string, action authz.Action) (bool, error) {
	return h.host.ProjectRoleActionAllowed(ctx, user, projectID, action)
}

func (h *Handler) projectAuthorizer(ctx context.Context) authz.ProjectAuthorizer {
	return h.host.ProjectAuthorizer(ctx)
}

func (h *Handler) runtimeClusterForProjectUse(ctx *gin.Context, user model.User, projectID, clusterID string) (model.RuntimeCluster, bool) {
	return h.host.RuntimeClusterForProjectUse(ctx, user, projectID, clusterID)
}

func (h *Handler) ensureBillingAllowsDeployChange(ctx *gin.Context, projectID string) bool {
	return h.host.EnsureBillingAllowsDeployChange(ctx, projectID)
}

func (h *Handler) ensureBillingAllowsManagedVolumeChange(ctx *gin.Context, projectID string) bool {
	return h.host.EnsureBillingAllowsManagedVolumeChange(ctx, projectID)
}

func (h *Handler) auditWithContext(userID, action, resource string, success bool, message string, ctx context.Context) {
	h.host.AuditWithContext(userID, action, resource, success, message, ctx)
}

func (h *Handler) currentAccessTokenFromContext(ctx *gin.Context) (model.AccessToken, bool) {
	return h.host.CurrentAccessTokenFromContext(ctx)
}

func (h *Handler) accessTokenAllows(scopeText, required string) bool {
	return h.host.AccessTokenAllows(scopeText, required)
}

func (h *Handler) requestUsesBearerToken(ctx *gin.Context) bool {
	return h.host.RequestUsesBearerToken(ctx)
}

func (h *Handler) requireContinuousAuthorizationBinding(ctx *gin.Context, user model.User) (projectapi.ContinuousAuthorizationBinding, bool) {
	return h.host.RequireContinuousAuthorizationBinding(ctx, user)
}

func (h *Handler) currentInteractiveAuthorizationBinding(ctx *gin.Context, user model.User) (projectapi.ContinuousAuthorizationBinding, bool) {
	return h.host.CurrentInteractiveAuthorizationBinding(ctx, user)
}

func (h *Handler) monitorContinuousAuthorization(ctx context.Context, binding projectapi.ContinuousAuthorizationBinding, authorizationAllowed func(context.Context, model.User) bool, revoke func()) (<-chan struct{}, bool) {
	return h.host.MonitorContinuousAuthorization(ctx, binding, authorizationAllowed, revoke)
}

func (h *Handler) projectContinuousAuthorizationAllowed(ctx context.Context, user model.User, projectID string, action authz.Action) bool {
	return h.host.ProjectContinuousAuthorizationAllowed(ctx, user, projectID, action)
}

func (h *Handler) resourceCanMutateDuringDelete(status string) bool {
	return h.host.ResourceCanMutateDuringDelete(status)
}

type paginationParams = transportapi.PaginationParams

func bindJSON(ctx *gin.Context, value any) bool { return transportapi.BindJSON(ctx, value) }
func writeErrorCode(ctx *gin.Context, status int, code, detail string) {
	transportapi.WriteErrorCode(ctx, status, code, detail)
}
func writeErrorKey(ctx *gin.Context, status int, message, key string) {
	transportapi.WriteErrorKey(ctx, status, message, key)
}
func requestLanguage(ctx *gin.Context) string { return transportapi.RequestLanguage(ctx) }
func markLiveObservationResponse(ctx *gin.Context) {
	transportapi.MarkLiveObservationResponse(ctx)
}
func paginationFromQuery(ctx *gin.Context) paginationParams {
	return transportapi.PaginationFromQuery(ctx)
}
func parsePositiveInt(value string, fallback int) int {
	return transportapi.ParsePositiveInt(value, fallback)
}
func paginateSlice[T any](items []T, pagination paginationParams) []T {
	return transportapi.PaginateSlice(items, pagination)
}
func paginatedResponse[T any](items []T, total int64, pagination paginationParams) transportapi.PaginatedResponseBody[T] {
	return transportapi.PaginatedResponse(items, total, pagination)
}

func runtimeProjectNamespace(project model.Project) string {
	return strings.TrimSpace(project.KubernetesNamespace)
}

func replaceRequestContext(ctx *gin.Context, requestCtx context.Context) func() {
	return projectapi.ReplaceRequestContext(ctx, requestCtx)
}

func writeContinuousAuthorizationRevoked(ctx *gin.Context) {
	projectapi.WriteContinuousAuthorizationRevoked(ctx)
}

func writeProjectAuthorizationError(ctx *gin.Context, err error) {
	projectapi.WriteProjectAuthorizationError(ctx, err)
}
