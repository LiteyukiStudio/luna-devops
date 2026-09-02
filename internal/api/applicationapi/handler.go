package applicationapi

import (
	"context"
	"strings"

	transportapi "github.com/LiteyukiStudio/devops/internal/api/transport"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/resourceidentifier"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type DeploymentTargetEmptyDirInput struct {
	Medium    string `json:"medium"`
	SizeLimit string `json:"sizeLimit"`
}

type DeploymentTargetDataVolumeInput struct {
	LogicalName     string                         `json:"logicalName"`
	SourceType      string                         `json:"sourceType"`
	ProjectVolumeID string                         `json:"projectVolumeId,omitempty"`
	MountPath       string                         `json:"mountPath,omitempty"`
	DevicePath      string                         `json:"devicePath,omitempty"`
	ReadOnly        bool                           `json:"readOnly,omitempty"`
	EmptyDir        *DeploymentTargetEmptyDirInput `json:"emptyDir,omitempty"`
}

type DeploymentVolumeAuditRecord struct {
	Action   string
	Resource string
	Message  string
}

type DeploymentVolumeMountChanges struct {
	Bound        []model.DeploymentVolumeMount
	Unbound      []model.DeploymentVolumeMount
	HookBindings []model.DeploymentTargetHookBinding
	Attempted    []DeploymentVolumeAuditRecord
}

type deploymentTargetEmptyDirInput = DeploymentTargetEmptyDirInput
type deploymentTargetDataVolumeInput = DeploymentTargetDataVolumeInput
type deploymentVolumeAuditRecord = DeploymentVolumeAuditRecord
type deploymentVolumeMountChanges = DeploymentVolumeMountChanges

const (
	applicationIdentifierMinLength = resourceidentifier.ApplicationMinLength
	applicationIdentifierMaxLength = resourceidentifier.ApplicationMaxLength
	stageIdentifierMinLength       = resourceidentifier.StageMinLength
	stageIdentifierMaxLength       = resourceidentifier.StageMaxLength
	defaultBuildCPURequest         = model.DefaultBuildCPURequest
	defaultBuildMemoryRequest      = model.DefaultBuildMemoryRequest
	defaultBuildTimeoutSeconds     = 1800
)

type Host interface {
	DBFor(ctx *gin.Context) *gorm.DB
	DBWithContext(ctx context.Context) *gorm.DB
	AuthorizeProject(ctx *gin.Context, action authz.Action) (model.User, model.Project, bool)
	EnsureProjectCanMutate(ctx *gin.Context, project model.Project) bool
	EnsureBillingAllowsDeployChange(ctx *gin.Context, projectID string) bool
	AuditWithContext(userID, action, resource string, success bool, message string, ctx context.Context)
	SecretStore() secret.Store
	PublicBaseURL() string
	NotificationEnqueuer() notification.DeliveryEnqueuer
	SyncDeploymentTargetVolumeMounts(ctx context.Context, tx *gorm.DB, target model.DeploymentTarget, inputs []DeploymentTargetDataVolumeInput) (DeploymentVolumeMountChanges, error)
	NextReleaseRevisionFor(tx *gorm.DB, projectID, applicationID, deploymentTargetID string) (int, error)
	AuditDeploymentVolumeMountFailure(ctx context.Context, userID string, changes DeploymentVolumeMountChanges, err error)
	AuditDeploymentVolumeMountChanges(ctx context.Context, userID string, target model.DeploymentTarget, changes DeploymentVolumeMountChanges)
	WriteVolumeError(ctx *gin.Context, err error)
	EnqueueDeployRun(ctx context.Context, release model.Release) bool
	DeploymentTargetVolumeMountsByTarget(ctx context.Context, targets []model.DeploymentTarget) (map[string][]model.DeploymentVolumeMount, error)
	DeploymentTargetResponseFromModel(target model.DeploymentTarget, mounts []model.DeploymentVolumeMount) any
	NormalizePublicStage(value string) (string, bool)
	WriteDeploymentStageInvalid(ctx *gin.Context, path, detail string)
	NormalizeBuildResourceQuantity(ctx *gin.Context, value, fallbackValue, label string) (string, bool)
	RuntimeClusterForProjectUse(ctx *gin.Context, user model.User, projectID, clusterID string) (model.RuntimeCluster, bool)
	RuntimeProjectNamespace(project model.Project) string
	NormalizeDataVolumes(ctx *gin.Context, inputs []DeploymentTargetDataVolumeInput) ([]DeploymentTargetDataVolumeInput, bool)
	NormalizeRuntimeConfigFilesInput(ctx *gin.Context, value string) (string, bool)
	NormalizeRuntimeConfigFilePathInput(ctx *gin.Context, value string) (string, bool)
	IsBuildEnvKey(value string) bool
	ObserveDeploymentTargets(ctx context.Context, project model.Project, targets []model.DeploymentTarget)
	RuntimeClusterForDeploymentTarget(ctx *gin.Context, target model.DeploymentTarget) (model.RuntimeCluster, bool)
	DeploymentTargetNamespace(project model.Project, target model.DeploymentTarget) string
	KubernetesClientForDeploymentTargetObservation(project model.Project, target model.DeploymentTarget, ctx context.Context) (*kubeprovider.Client, string, string)
	DeploymentTargetResourceName(target model.DeploymentTarget) string
	EnqueueApplicationDelete(ctx context.Context, app model.Application, actorID string, deleteData bool) bool
}

type Handler struct {
	host    Host
	secrets secret.Store
}

type Handlers = Handler

func New(host Host) *Handler {
	return &Handler{host: host, secrets: host.SecretStore()}
}

func (h *Handler) dbFor(ctx *gin.Context) *gorm.DB { return h.host.DBFor(ctx) }
func (h *Handler) dbWithContext(ctx context.Context) *gorm.DB {
	return h.host.DBWithContext(ctx)
}
func (h *Handler) authorizeProject(ctx *gin.Context, action authz.Action) (model.User, model.Project, bool) {
	return h.host.AuthorizeProject(ctx, action)
}
func (h *Handler) ensureProjectCanMutate(ctx *gin.Context, project model.Project) bool {
	return h.host.EnsureProjectCanMutate(ctx, project)
}
func (h *Handler) ensureBillingAllowsDeployChange(ctx *gin.Context, projectID string) bool {
	return h.host.EnsureBillingAllowsDeployChange(ctx, projectID)
}
func (h *Handler) auditWithContext(userID, action, resource string, success bool, message string, ctx context.Context) {
	h.host.AuditWithContext(userID, action, resource, success, message, ctx)
}
func (h *Handler) runtimeClusterForProjectUse(ctx *gin.Context, user model.User, projectID, clusterID string) (model.RuntimeCluster, bool) {
	return h.host.RuntimeClusterForProjectUse(ctx, user, projectID, clusterID)
}
func (h *Handler) runtimeClusterForDeploymentTarget(ctx *gin.Context, target model.DeploymentTarget) (model.RuntimeCluster, bool) {
	return h.host.RuntimeClusterForDeploymentTarget(ctx, target)
}
func (h *Handler) deploymentTargetVolumeMountsByTarget(ctx context.Context, targets []model.DeploymentTarget) (map[string][]model.DeploymentVolumeMount, error) {
	return h.host.DeploymentTargetVolumeMountsByTarget(ctx, targets)
}
func (h *Handler) auditDeploymentVolumeMountFailure(ctx context.Context, userID string, changes DeploymentVolumeMountChanges, err error) {
	h.host.AuditDeploymentVolumeMountFailure(ctx, userID, changes, err)
}
func (h *Handler) auditDeploymentVolumeMountChanges(ctx context.Context, userID string, target model.DeploymentTarget, changes DeploymentVolumeMountChanges) {
	h.host.AuditDeploymentVolumeMountChanges(ctx, userID, target, changes)
}
func (h *Handler) enqueueDeployRun(ctx context.Context, release model.Release) bool {
	return h.host.EnqueueDeployRun(ctx, release)
}

type paginationParams = transportapi.PaginationParams

func bindJSON(ctx *gin.Context, value any) bool { return transportapi.BindJSON(ctx, value) }
func writeError(ctx *gin.Context, status int, message string) {
	transportapi.WriteError(ctx, status, message)
}
func writeErrorCode(ctx *gin.Context, status int, code, detail string) {
	transportapi.WriteErrorCode(ctx, status, code, detail)
}
func paginationFromQueryWithSort(ctx *gin.Context, allowed map[string]string, fallback string) paginationParams {
	return transportapi.PaginationFromQueryWithSort(ctx, allowed, fallback)
}
func paginationFromQuery(ctx *gin.Context) paginationParams {
	return transportapi.PaginationFromQuery(ctx)
}
func paginatedResponse[T any](items []T, total int64, pagination paginationParams) transportapi.PaginatedResponseBody[T] {
	return transportapi.PaginatedResponse(items, total, pagination)
}
func orderByClause(pagination paginationParams, allowed map[string]string, fallback string) string {
	return transportapi.OrderByClause(pagination, allowed, fallback)
}
func applySearch(ctx *gin.Context, query *gorm.DB, columns ...string) *gorm.DB {
	return transportapi.ApplySearch(ctx, query, columns...)
}
func markLiveObservationResponse(ctx *gin.Context) { transportapi.MarkLiveObservationResponse(ctx) }
func errorEnvelope(ctx *gin.Context, status int, code string) gin.H {
	return transportapi.ErrorEnvelope(ctx, status, code)
}
func isDevelopmentRequest(ctx *gin.Context) bool { return transportapi.IsDevelopmentRequest(ctx) }
func fallbackInt(value, fallbackValue int) int {
	return transportapi.FallbackInt(value, fallbackValue)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func optionalStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
