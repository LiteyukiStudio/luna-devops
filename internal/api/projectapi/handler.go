package projectapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/api/notificationapi"
	transportapi "github.com/LiteyukiStudio/devops/internal/api/transport"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/dependency"
	"github.com/LiteyukiStudio/devops/internal/model"
	projectservice "github.com/LiteyukiStudio/devops/internal/project"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const currentProjectRoleContextKey = "currentProjectRole"

const defaultProjectBuildConcurrency = 2

// Host exposes only the root capabilities that are not owned by the project
// HTTP domain. It deliberately avoids importing the root api package.
type Host interface {
	DBFor(ctx *gin.Context) *gorm.DB
	DBWithContext(ctx context.Context) *gorm.DB
	CurrentUser(ctx *gin.Context) (model.User, bool)
	EnsurePlatformSystemProject(user model.User, ctx context.Context) (model.Project, error)
	AIConversationProjectID(ctx *gin.Context) string
	EnqueueEnabledProjectAccessKubeGateways(ctx context.Context, userID string, includeGlobal bool) error
	EnqueueResourceCleanup(ctx context.Context, resourceType, resourceID, projectID, actorID string) bool
	AuditWithContext(userID, action, resource string, success bool, message string, ctx context.Context)
	ProjectIDsForUser(ctx context.Context, userID string) []string
	ProjectHasAnotherOwner(ctx context.Context, projectID, memberID string) bool
	RequestUsesBearerToken(ctx *gin.Context) bool
	CurrentAccessTokenFromContext(ctx *gin.Context) (model.AccessToken, bool)
	CurrentSessionFromCookie(ctx *gin.Context) (model.UserSession, bool)
	ContinuousAuthorizationInterval() time.Duration
}

type Handler struct {
	host Host
}

func New(host Host) *Handler {
	return &Handler{host: host}
}

func (h *Handler) dbFor(ctx *gin.Context) *gorm.DB {
	return h.host.DBFor(ctx)
}

func (h *Handler) dbWithContext(ctx context.Context) *gorm.DB {
	return h.host.DBWithContext(ctx)
}

func (h *Handler) currentUser(ctx *gin.Context) (model.User, bool) {
	return h.host.CurrentUser(ctx)
}

func (h *Handler) ensurePlatformSystemProject(user model.User, ctx context.Context) (model.Project, error) {
	return h.host.EnsurePlatformSystemProject(user, ctx)
}

func (h *Handler) enqueueEnabledProjectAccessKubeGateways(ctx context.Context, userID string, includeGlobal bool) error {
	return h.host.EnqueueEnabledProjectAccessKubeGateways(ctx, userID, includeGlobal)
}

func (h *Handler) enqueueResourceCleanup(ctx context.Context, resourceType, resourceID, projectID, actorID string) bool {
	return h.host.EnqueueResourceCleanup(ctx, resourceType, resourceID, projectID, actorID)
}

func (h *Handler) auditWithContext(userID, action, resource string, success bool, message string, ctx context.Context) {
	h.host.AuditWithContext(userID, action, resource, success, message, ctx)
}

func (h *Handler) currentSessionFromCookie(ctx *gin.Context) (model.UserSession, bool) {
	return h.host.CurrentSessionFromCookie(ctx)
}

func (h *Handler) dependencyService(ctx context.Context) *dependency.Service {
	return dependency.NewService(dependency.NewGormRepository(h.dbWithContext(ctx)))
}

func (h *Handler) ensureProjectCanMutate(ctx *gin.Context, project model.Project) bool {
	if resourceCanMutateDuringDelete(project.DeleteStatus) {
		return true
	}
	writeErrorCode(ctx, http.StatusConflict, "project.delete_in_progress", "项目空间正在删除中，请等待资源清理完成")
	return false
}

func deleteStatusCanStart(status string) bool {
	status = strings.TrimSpace(status)
	return status == "" || status == "active" || status == "delete_failed"
}

func resourceCanMutateDuringDelete(status string) bool {
	status = strings.TrimSpace(status)
	return status == "" || status == "active" || status == "delete_failed"
}

func isSystemProject(project model.Project) bool {
	return strings.TrimSpace(project.SystemKey) != ""
}

var errResourceDeleteAlreadyStarted = errors.New("resource deletion has already started")

func markResourceDeleting(tx *gorm.DB, resource any, resourceID string) error {
	startedAt := time.Now()
	result := tx.Model(resource).Where("id = ? and delete_status in ?", resourceID, []string{"", "active", "delete_failed"}).Updates(map[string]any{
		"delete_status":      "deleting",
		"delete_message":     "",
		"delete_started_at":  &startedAt,
		"delete_finished_at": nil,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errResourceDeleteAlreadyStarted
	}
	return nil
}

func markResourceDeleteFailed(db *gorm.DB, resource any, resourceID, message string) error {
	finishedAt := time.Now()
	return db.Model(resource).Where("id = ?", resourceID).Updates(map[string]any{
		"delete_status":      "delete_failed",
		"delete_message":     strings.TrimSpace(message),
		"delete_finished_at": &finishedAt,
	}).Error
}

func writeKubeGatewayEnqueueError(ctx *gin.Context) {
	writeErrorCode(ctx, http.StatusServiceUnavailable, "kube_gateway.enqueue_failed", "kubernetes gateway reconciliation could not be queued")
}

func writeDependencyError(ctx *gin.Context, err error) {
	code := dependency.ErrorCode(err)
	status := http.StatusInternalServerError
	switch code {
	case dependency.CodeNotFound:
		status = http.StatusNotFound
	case dependency.CodeEnvConflict, dependency.CodeTopologyDuplicate:
		status = http.StatusConflict
	case dependency.CodeInvalidInput, dependency.CodeCrossProject, dependency.CodeCrossCluster,
		dependency.CodeSourceTargetSame, dependency.CodePortNotFound, dependency.CodeReservedEnv:
		status = http.StatusBadRequest
	}
	if code == "" {
		code = "dependency_operation_failed"
	}
	writeErrorCode(ctx, status, code, err.Error())
}

type paginationParams = transportapi.PaginationParams
type paginatedResponseBody[T any] = transportapi.PaginatedResponseBody[T]

func bindJSON(ctx *gin.Context, value any) bool { return transportapi.BindJSON(ctx, value) }
func writeError(ctx *gin.Context, status int, message string) {
	transportapi.WriteError(ctx, status, message)
}
func writeErrorCode(ctx *gin.Context, status int, code, detail string) {
	transportapi.WriteErrorCode(ctx, status, code, detail)
}
func writeErrorKey(ctx *gin.Context, status int, message, key string) {
	transportapi.WriteErrorKey(ctx, status, message, key)
}
func requestLanguage(ctx *gin.Context) string {
	return transportapi.RequestLanguage(ctx)
}
func paginationFromQueryWithSort(ctx *gin.Context, allowedFields map[string]string, defaultField string) paginationParams {
	return transportapi.PaginationFromQueryWithSort(ctx, allowedFields, defaultField)
}
func paginatedResponse[T any](items []T, total int64, pagination paginationParams) paginatedResponseBody[T] {
	return transportapi.PaginatedResponse(items, total, pagination)
}
func orderByClause(pagination paginationParams, allowedFields map[string]string, defaultColumn string) string {
	return transportapi.OrderByClause(pagination, allowedFields, defaultColumn)
}
func applySearch(ctx *gin.Context, query *gorm.DB, columns ...string) *gorm.DB {
	return transportapi.ApplySearch(ctx, query, columns...)
}
func fallback(value, defaultValue string) string {
	return transportapi.Fallback(value, defaultValue)
}
func normalizeStringList(values []string) []string {
	return transportapi.NormalizeStringList(values)
}

func normalizeBuildConcurrency(value int, defaultValue int) int {
	if value > 0 {
		return value
	}
	return defaultValue
}

var defaultInboxBroker = notificationapi.DefaultInboxBroker()

type projectMemberInboxInput = notificationapi.ProjectMemberInboxInput

func publishProjectMemberInbox(ctx context.Context, tx *gorm.DB, input projectMemberInboxInput) error {
	return notificationapi.PublishProjectMemberInbox(ctx, tx, input)
}

func writeInboxError(ctx *gin.Context, err error) {
	notificationapi.WriteInboxError(ctx, err)
}

// Root compatibility exports. The root api package keeps its established
// private contracts while their implementation lives in this domain package.
func (h *Handler) AuthorizeProject(ctx *gin.Context, action authz.Action) (model.User, model.Project, bool) {
	return h.authorizeProject(ctx, action)
}

func (h *Handler) AuthorizeProjectByID(ctx *gin.Context, projectID string, action authz.Action) (model.User, model.Project, bool) {
	return h.authorizeProjectByID(ctx, projectID, action)
}

func (h *Handler) ProjectActionAllowed(ctx context.Context, subject authz.ProjectSubject, projectID string, action authz.Action) (bool, error) {
	return h.projectActionAllowed(ctx, subject, projectID, action)
}

func (h *Handler) ProjectRoleActionAllowed(ctx context.Context, user model.User, projectID string, action authz.Action) (bool, error) {
	return h.projectRoleActionAllowed(ctx, user, projectID, action)
}

func (h *Handler) ProjectMemberActionAllowed(ctx *gin.Context, projectID, userID string, action authz.Action) (bool, bool) {
	return h.projectMemberActionAllowed(ctx, projectID, userID, action)
}

func (h *Handler) ProjectAuthorizer(ctx context.Context) authz.ProjectAuthorizer {
	return h.projectAuthorizer(ctx)
}

func (h *Handler) ProjectIDsForUser(ctx context.Context, userID string) []string {
	return h.projectIDsForUser(ctx, userID)
}

func (h *Handler) FindProjectForCurrentUserByID(ctx *gin.Context, projectID string) (model.Project, bool) {
	return h.findProjectForCurrentUserByID(ctx, projectID)
}

func (h *Handler) FindProject(ctx *gin.Context) (model.Project, bool) {
	return h.findProject(ctx)
}

func (h *Handler) NormalizeScopedOwnerWithProjects(ctx *gin.Context, user model.User, rawScope, rawOwnerRef string, rawProjectIDs []string, globalError string) (string, string, []string, bool) {
	return h.normalizeScopedOwnerWithProjects(ctx, user, rawScope, rawOwnerRef, rawProjectIDs, globalError)
}

func (h *Handler) NormalizeCredentialScopeWithinParent(ctx *gin.Context, user model.User, rawScope string, rawProjectIDs []string, parentScope string, parentProjectIDs []string, globalError string) (string, string, []string, bool) {
	return h.normalizeCredentialScopeWithinParent(ctx, user, rawScope, rawProjectIDs, parentScope, parentProjectIDs, globalError)
}

func (h *Handler) CanManageScopedResourceByID(ctx *gin.Context, user model.User, scope, ownerRef, resourceType, resourceID, errorMessage string) bool {
	return h.canManageScopedResourceByID(ctx, user, scope, ownerRef, resourceType, resourceID, errorMessage)
}

func (h *Handler) CanInspectScopedResourceConfigByID(user model.User, scope, ownerRef, resourceType, resourceID string, ctx context.Context) (bool, error) {
	return h.canInspectScopedResourceConfigByID(user, scope, ownerRef, resourceType, resourceID, ctx)
}

func (h *Handler) CanUseScopedResourceByID(user model.User, scope, ownerRef, resourceType, resourceID string, ctx context.Context) bool {
	return h.canUseScopedResourceByID(user, scope, ownerRef, resourceType, resourceID, ctx)
}

func (h *Handler) ApplyScopedResourceVisibility(ctx *gin.Context, query *gorm.DB, resourceType string, user model.User, projectID string) (*gorm.DB, bool) {
	return h.applyScopedResourceVisibility(ctx, query, resourceType, user, projectID)
}

func (h *Handler) ApplyScopedResourceListVisibility(ctx *gin.Context, query *gorm.DB, resourceType string, user model.User, projectID string, visibility projectservice.ListVisibility) (*gorm.DB, bool) {
	return h.applyScopedResourceListVisibility(ctx, query, resourceType, user, projectID, visibility)
}

func (h *Handler) ApplyScopedResourceVisibilityForUser(query *gorm.DB, resourceType string, user model.User, ctx context.Context) *gorm.DB {
	return h.applyScopedResourceVisibilityForUser(query, resourceType, user, ctx)
}

func (h *Handler) ApplyScopedResourceVisibilityForProject(query *gorm.DB, resourceType string, user model.User, projectID string, ctx context.Context) *gorm.DB {
	return h.applyScopedResourceVisibilityForProject(query, resourceType, user, projectID, ctx)
}

func (h *Handler) ReplaceScopedResourceProjectBindings(tx *gorm.DB, resourceType, resourceID string, projectIDs, defaultProjectIDs []string) error {
	return h.replaceScopedResourceProjectBindings(tx, resourceType, resourceID, projectIDs, defaultProjectIDs)
}

func (h *Handler) ScopedResourceProjectIDs(resourceType, resourceID string, ctx context.Context) []string {
	return h.scopedResourceProjectIDs(resourceType, resourceID, ctx)
}

func (h *Handler) ScopedResourceProjectIDsResult(resourceType, resourceID string, ctx context.Context) ([]string, error) {
	return h.scopedResourceProjectIDsResult(resourceType, resourceID, ctx)
}

func (h *Handler) ScopedResourceProjectIDMap(resourceType string, resourceIDs []string, ctx context.Context) map[string][]string {
	return h.scopedResourceProjectIDMap(resourceType, resourceIDs, ctx)
}

func (h *Handler) ScopedResourceDefaultProjectIDMap(resourceType string, resourceIDs []string, ctx context.Context) map[string][]string {
	return h.scopedResourceDefaultProjectIDMap(resourceType, resourceIDs, ctx)
}

func (h *Handler) DecideInboxAction(ctx context.Context, user model.User, requestID, decision string, expectedVersion int64) error {
	return h.decideInboxAction(ctx, user, requestID, decision, expectedVersion)
}
