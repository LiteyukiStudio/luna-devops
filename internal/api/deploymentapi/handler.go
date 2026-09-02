package deploymentapi

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/api/applicationapi"
	"github.com/LiteyukiStudio/devops/internal/api/buildapi"
	"github.com/LiteyukiStudio/devops/internal/api/projectapi"
	runtimeapi "github.com/LiteyukiStudio/devops/internal/api/runtimeapi"
	transportapi "github.com/LiteyukiStudio/devops/internal/api/transport"
	"github.com/LiteyukiStudio/devops/internal/api/volumeapi"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	registryprovider "github.com/LiteyukiStudio/devops/internal/provider/registry"
	"github.com/LiteyukiStudio/devops/internal/resourceidentifier"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/LiteyukiStudio/devops/internal/security"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Host exposes the cross-domain capabilities needed by deployment and release
// HTTP handlers without introducing a dependency on the root api package.
type Host interface {
	DBFor(ctx *gin.Context) *gorm.DB
	DBWithContext(ctx context.Context) *gorm.DB
	SecretStore() secret.Store
	AllowedOrigin(origin string) bool
	EnqueueDeployRun(ctx context.Context, release model.Release) bool
	ApplyScopedResourceVisibilityForProject(query *gorm.DB, resourceType string, user model.User, projectID string, ctx context.Context) *gorm.DB
	RegistryPushCredentialForProject(user model.User, registry model.ArtifactRegistry, projectID string, ctx context.Context) (model.RegistryCredential, bool)
	CanUseScopedResourceByID(user model.User, scope, ownerRef, resourceType, resourceID string, ctx context.Context) bool
	MutateDeploymentTargetRuntimeSecrets(ctx *gin.Context, user model.User, project model.Project, application model.Application, target model.DeploymentTarget)
	AuthorizeProject(ctx *gin.Context, action authz.Action) (model.User, model.Project, bool)
	FindApplication(ctx *gin.Context) (model.Application, bool)
	AuditWithContext(userID, action, resource string, success bool, message string, ctx context.Context)
	FindBuildEnvironmentConfig(db *gorm.DB, scope, scopeRef string) (model.BuildEnvironmentConfig, error)
	EnsureProjectCanMutate(ctx *gin.Context, project model.Project) bool
	EnsureBillingAllowsDeployChange(ctx *gin.Context, projectID string) bool
	CanManageBuildEnvironmentProject(ctx *gin.Context, user model.User, projectID string) bool
	DeploymentBuildEnvironmentFromInput(ctx *gin.Context, user model.User, projectID, targetID string, input DeploymentTargetInput, existing *model.BuildEnvironmentConfig) (*model.BuildEnvironmentConfig, bool)
	EnsureDeploymentTargetCanMutate(ctx *gin.Context, target model.DeploymentTarget) bool
	EnsureNoIncomingServiceBindings(ctx *gin.Context, projectID, targetApplicationID, targetDeploymentTargetID string) bool
	DeleteStatusCanStart(status string) bool
	MarkResourceDeleting(tx *gorm.DB, resource any, resourceID string) error
	MarkDeploymentTargetGatewayRoutesDeleting(tx *gorm.DB, target model.DeploymentTarget) error
	MarkResourceDeleteFailed(db *gorm.DB, resource any, resourceID, message string) error
	MarkDeploymentTargetGatewayRoutesDeleteFailed(db *gorm.DB, target model.DeploymentTarget, message string) error
	IsResourceDeleteAlreadyStarted(err error) bool
	EnqueueResourceCleanup(ctx context.Context, resourceType, resourceID, projectID, actorID string) bool
	RuntimeClusterForProjectUse(ctx *gin.Context, user model.User, projectID, clusterID string) (model.RuntimeCluster, bool)
	RuntimeSecretFilesFromInput(ctx *gin.Context, user model.User, ownerID, value string, existing map[string]string) (map[string]string, bool)
	RuntimeClusterForEnvironment(ctx *gin.Context, environment model.Environment) (model.RuntimeCluster, bool)
	RuntimeClusterForDeploymentTarget(ctx *gin.Context, target model.DeploymentTarget) (model.RuntimeCluster, bool)
	RuntimeClusterForDeploymentTargetValue(target model.DeploymentTarget, ctx context.Context) (model.RuntimeCluster, error)
	RequireContinuousAuthorizationBinding(ctx *gin.Context, user model.User) (projectapi.ContinuousAuthorizationBinding, bool)
	MonitorContinuousAuthorization(ctx context.Context, binding projectapi.ContinuousAuthorizationBinding, authorizationAllowed func(context.Context, model.User) bool, revoke func()) (<-chan struct{}, bool)
	ProjectContinuousAuthorizationAllowed(ctx context.Context, user model.User, projectID string, action authz.Action) bool
	ResourceCanMutateDuringDelete(status string) bool
	RegistryCredentialInput(ctx context.Context, user model.User, registry model.ArtifactRegistry) registryprovider.Credential
	EgressPolicyForUser(user model.User, ctx context.Context) security.EgressPolicy
	RequireRuntimeTerminalAuthorization(ctx *gin.Context, user model.User) (runtimeapi.RuntimeTerminalAuthorizationBinding, bool)
	ConsumeRuntimeTerminalTicket(ctx context.Context, ticket string) (runtimeapi.RuntimeTerminalTicketValue, bool, error)
	IssueRuntimeTerminalTicket(ctx context.Context, authorization runtimeapi.RuntimeTerminalAuthorizationBinding, resourceKind string, resource any) (string, time.Time, error)
	ContinuousAuthorizationActive(ctx context.Context, binding runtimeapi.RuntimeTerminalAuthorizationBinding, authorizationAllowed func(context.Context, model.User) bool) bool
	ReleaseRuntimeTerminalAuthorizationAllowed(ctx context.Context, user model.User, reference runtimeapi.ReleaseRuntimeTerminalAuthorizationReference) bool
	FindProject(ctx *gin.Context) (model.Project, bool)
	ProjectRoleActionAllowed(ctx context.Context, user model.User, projectID string, action authz.Action) (bool, error)
}

type Handler struct {
	host    Host
	secrets secret.Store
}

type Handlers = Handler
type continuousAuthorizationBinding = projectapi.ContinuousAuthorizationBinding
type runtimeTerminalAuthorizationBinding = runtimeapi.RuntimeTerminalAuthorizationBinding
type runtimeTerminalTicketValue = runtimeapi.RuntimeTerminalTicketValue
type runtimeTerminalTicketResponse = runtimeapi.RuntimeTerminalTicketResponse
type releaseRuntimeTerminalAuthorizationReference = runtimeapi.ReleaseRuntimeTerminalAuthorizationReference

func New(host Host) *Handler {
	return &Handler{host: host, secrets: host.SecretStore()}
}

func (h *Handler) dbFor(ctx *gin.Context) *gorm.DB { return h.host.DBFor(ctx) }
func (h *Handler) dbWithContext(ctx context.Context) *gorm.DB {
	return h.host.DBWithContext(ctx)
}

type paginationParams = transportapi.PaginationParams

const (
	defaultPageSize                = transportapi.DefaultPageSize
	maxPageSize                    = transportapi.MaxPageSize
	scopedResourceRuntimeCluster   = "runtime_cluster"
	scopedResourceArtifactRegistry = "artifact_registry"
	scopedResourceBuildVariableSet = "build_variable_set"
	stageIdentifierMinLength       = resourceidentifier.StageMinLength
	stageIdentifierMaxLength       = resourceidentifier.StageMaxLength
	sessionCookieName              = "lyd_session"
)

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
func writeArgumentErrorCode(ctx *gin.Context, status int, code, detail, path string, allowedValues []string, retryable bool) {
	transportapi.WriteArgumentErrorCode(ctx, status, code, detail, path, allowedValues, retryable)
}
func requestLanguage(ctx *gin.Context) string { return transportapi.RequestLanguage(ctx) }
func paginationFromQuery(ctx *gin.Context) paginationParams {
	return transportapi.PaginationFromQuery(ctx)
}
func paginationFromQueryWithSort(ctx *gin.Context, allowedFields map[string]string, defaultField string) paginationParams {
	return transportapi.PaginationFromQueryWithSort(ctx, allowedFields, defaultField)
}
func paginatedResponse[T any](items []T, total int64, pagination paginationParams) transportapi.PaginatedResponseBody[T] {
	return transportapi.PaginatedResponse(items, total, pagination)
}
func paginateSlice[T any](items []T, pagination paginationParams) []T {
	return transportapi.PaginateSlice(items, pagination)
}
func orderByClause(pagination paginationParams, allowedFields map[string]string, defaultColumn string) string {
	return transportapi.OrderByClause(pagination, allowedFields, defaultColumn)
}
func applySearch(ctx *gin.Context, query *gorm.DB, columns ...string) *gorm.DB {
	return transportapi.ApplySearch(ctx, query, columns...)
}
func markLiveObservationResponse(ctx *gin.Context) { transportapi.MarkLiveObservationResponse(ctx) }
func writeSSE(writer http.ResponseWriter, event, idValue string, data any) {
	buildapi.WriteSSE(writer, event, idValue, data)
}
func flushSSE(writer http.ResponseWriter) { buildapi.FlushSSE(writer) }
func terminalDisconnectedMessage(ctx *gin.Context, detail string) []byte {
	return transportapi.TerminalDisconnectedMessage(ctx, detail)
}

func normalizeStringList(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func (h *Handler) applyScopedResourceVisibilityForProject(query *gorm.DB, resourceType string, user model.User, projectID string, ctx context.Context) *gorm.DB {
	return h.host.ApplyScopedResourceVisibilityForProject(query, resourceType, user, projectID, ctx)
}

func (h *Handler) registryPushCredentialForProject(user model.User, registry model.ArtifactRegistry, projectID string, ctx context.Context) (model.RegistryCredential, bool) {
	return h.host.RegistryPushCredentialForProject(user, registry, projectID, ctx)
}

func (h *Handler) canUseScopedResourceByID(user model.User, scope, ownerRef, resourceType, resourceID string, ctx context.Context) bool {
	return h.host.CanUseScopedResourceByID(user, scope, ownerRef, resourceType, resourceID, ctx)
}

func (h *Handler) authorizeProject(ctx *gin.Context, action authz.Action) (model.User, model.Project, bool) {
	return h.host.AuthorizeProject(ctx, action)
}

func (h *Handler) findApplication(ctx *gin.Context) (model.Application, bool) {
	return h.host.FindApplication(ctx)
}

func (h *Handler) auditWithContext(userID, action, resource string, success bool, message string, ctx context.Context) {
	h.host.AuditWithContext(userID, action, resource, success, message, ctx)
}

func (h *Handler) findBuildEnvironmentConfig(db *gorm.DB, scope, scopeRef string) (model.BuildEnvironmentConfig, error) {
	return h.host.FindBuildEnvironmentConfig(db, scope, scopeRef)
}

func (h *Handler) ensureProjectCanMutate(ctx *gin.Context, project model.Project) bool {
	return h.host.EnsureProjectCanMutate(ctx, project)
}

func (h *Handler) ensureBillingAllowsDeployChange(ctx *gin.Context, projectID string) bool {
	return h.host.EnsureBillingAllowsDeployChange(ctx, projectID)
}

func (h *Handler) canManageBuildEnvironmentProject(ctx *gin.Context, user model.User, projectID string) bool {
	return h.host.CanManageBuildEnvironmentProject(ctx, user, projectID)
}

func (h *Handler) deploymentBuildEnvironmentFromInput(ctx *gin.Context, user model.User, projectID, targetID string, input deploymentTargetInput, existing *model.BuildEnvironmentConfig) (*model.BuildEnvironmentConfig, bool) {
	return h.host.DeploymentBuildEnvironmentFromInput(ctx, user, projectID, targetID, input, existing)
}

func (h *Handler) ensureDeploymentTargetCanMutate(ctx *gin.Context, target model.DeploymentTarget) bool {
	return h.host.EnsureDeploymentTargetCanMutate(ctx, target)
}

func (h *Handler) ensureNoIncomingServiceBindings(ctx *gin.Context, projectID, targetApplicationID, targetDeploymentTargetID string) bool {
	return h.host.EnsureNoIncomingServiceBindings(ctx, projectID, targetApplicationID, targetDeploymentTargetID)
}

func (h *Handler) enqueueResourceCleanup(ctx context.Context, resourceType, resourceID, projectID, actorID string) bool {
	return h.host.EnqueueResourceCleanup(ctx, resourceType, resourceID, projectID, actorID)
}

func (h *Handler) runtimeClusterForProjectUse(ctx *gin.Context, user model.User, projectID, clusterID string) (model.RuntimeCluster, bool) {
	return h.host.RuntimeClusterForProjectUse(ctx, user, projectID, clusterID)
}

func (h *Handler) runtimeSecretFilesFromInput(ctx *gin.Context, user model.User, ownerID, value string, existing map[string]string) (map[string]string, bool) {
	return h.host.RuntimeSecretFilesFromInput(ctx, user, ownerID, value, existing)
}

func (h *Handler) runtimeClusterForEnvironment(ctx *gin.Context, environment model.Environment) (model.RuntimeCluster, bool) {
	return h.host.RuntimeClusterForEnvironment(ctx, environment)
}

func (h *Handler) runtimeClusterForDeploymentTarget(ctx *gin.Context, target model.DeploymentTarget) (model.RuntimeCluster, bool) {
	return h.host.RuntimeClusterForDeploymentTarget(ctx, target)
}

func (h *Handler) runtimeClusterForDeploymentTargetValue(target model.DeploymentTarget, ctx context.Context) (model.RuntimeCluster, error) {
	return h.host.RuntimeClusterForDeploymentTargetValue(target, ctx)
}

func deploymentTargetNamespace(project model.Project, target model.DeploymentTarget) string {
	return runtimeapi.DeploymentTargetNamespace(project, target)
}

func (h *Handler) requireContinuousAuthorizationBinding(ctx *gin.Context, user model.User) (continuousAuthorizationBinding, bool) {
	return h.host.RequireContinuousAuthorizationBinding(ctx, user)
}

func (h *Handler) monitorContinuousAuthorization(ctx context.Context, binding continuousAuthorizationBinding, authorizationAllowed func(context.Context, model.User) bool, revoke func()) (<-chan struct{}, bool) {
	return h.host.MonitorContinuousAuthorization(ctx, binding, authorizationAllowed, revoke)
}

func (h *Handler) projectContinuousAuthorizationAllowed(ctx context.Context, user model.User, projectID string, action authz.Action) bool {
	return h.host.ProjectContinuousAuthorizationAllowed(ctx, user, projectID, action)
}

func (h *Handler) registryCredentialInput(ctx context.Context, user model.User, registry model.ArtifactRegistry) registryprovider.Credential {
	return h.host.RegistryCredentialInput(ctx, user, registry)
}

func (h *Handler) egressPolicyForUser(user model.User, ctx context.Context) security.EgressPolicy {
	return h.host.EgressPolicyForUser(user, ctx)
}

func (h *Handler) requireRuntimeTerminalAuthorization(ctx *gin.Context, user model.User) (runtimeTerminalAuthorizationBinding, bool) {
	return h.host.RequireRuntimeTerminalAuthorization(ctx, user)
}

func (h *Handler) consumeRuntimeTerminalTicket(ctx context.Context, ticket string) (runtimeTerminalTicketValue, bool, error) {
	return h.host.ConsumeRuntimeTerminalTicket(ctx, ticket)
}

func (h *Handler) issueRuntimeTerminalTicket(ctx context.Context, authorization runtimeTerminalAuthorizationBinding, resourceKind string, resource any) (string, time.Time, error) {
	return h.host.IssueRuntimeTerminalTicket(ctx, authorization, resourceKind, resource)
}

func (h *Handler) continuousAuthorizationActive(ctx context.Context, binding runtimeTerminalAuthorizationBinding, authorizationAllowed func(context.Context, model.User) bool) bool {
	return h.host.ContinuousAuthorizationActive(ctx, binding, authorizationAllowed)
}

func (h *Handler) releaseRuntimeTerminalAuthorizationAllowed(ctx context.Context, user model.User, reference releaseRuntimeTerminalAuthorizationReference) bool {
	return h.host.ReleaseRuntimeTerminalAuthorizationAllowed(ctx, user, reference)
}

func (h *Handler) findProject(ctx *gin.Context) (model.Project, bool) {
	return h.host.FindProject(ctx)
}

func (h *Handler) projectRoleActionAllowed(ctx context.Context, user model.User, projectID string, action authz.Action) (bool, error) {
	return h.host.ProjectRoleActionAllowed(ctx, user, projectID, action)
}

func writeProjectAuthorizationError(ctx *gin.Context, err error) {
	projectapi.WriteProjectAuthorizationError(ctx, err)
}

func replaceRequestContext(ctx *gin.Context, requestCtx context.Context) func() {
	return projectapi.ReplaceRequestContext(ctx, requestCtx)
}

func writeContinuousAuthorizationRevoked(ctx *gin.Context) {
	projectapi.WriteContinuousAuthorizationRevoked(ctx)
}

func secretEnvironmentVariables(keys []string) []runtimeEnvironmentVariableResponse {
	return runtimeapi.SecretEnvironmentVariables(keys)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func applicationCanMutate(application model.Application) bool {
	return applicationapi.ApplicationCanMutate(application)
}

func buildVariableSetIDs(raw string) []string       { return buildapi.BuildVariableSetIDs(raw) }
func decodeSecretRefs(raw string) map[string]string { return buildapi.DecodeSecretRefs(raw) }
func isBuildEnvKey(value string) bool               { return buildapi.IsBuildEnvKey(value) }
func normalizeBuildVariables(ctx *gin.Context, input map[string]string) (map[string]string, bool) {
	return buildapi.NormalizeBuildVariables(ctx, input)
}
func splitTargetImageRef(value string) (string, string) { return buildapi.SplitTargetImageRef(value) }
func normalizeBuildArgsInput(ctx *gin.Context, raw string) (string, bool) {
	return buildapi.NormalizeBuildArgsInput(ctx, raw)
}
func normalizeHookPhase(value string) string       { return projectapi.NormalizeHookPhase(value) }
func writeVolumeError(ctx *gin.Context, err error) { volumeapi.WriteVolumeError(ctx, err) }
func fallback(value, fallbackValue string) string {
	return transportapi.Fallback(value, fallbackValue)
}
func fallbackInt(value, fallbackValue int) int {
	return transportapi.FallbackInt(value, fallbackValue)
}
func normalizeBuildSelectorList(values []string) []string {
	return buildapi.NormalizeBuildSelectorList(values)
}
func encodeBuildVariableSetIDs(ids []string) string { return buildapi.EncodeBuildVariableSetIDs(ids) }
func normalizeBuildConcurrencyPolicy(value string) string {
	return applicationapi.NormalizeBuildConcurrencyPolicy(value)
}
func buildTargetImageRepositoryForCredential(registry model.ArtifactRegistry, credential model.RegistryCredential, project model.Project, application model.Application, target model.DeploymentTarget) string {
	return buildapi.BuildTargetImageRepositoryForCredential(registry, credential, project, application, target)
}
func repositoryWithoutRegistryHost(registry model.ArtifactRegistry, repository string) string {
	return buildapi.RepositoryWithoutRegistryHost(registry, repository)
}
func buildStaticTargetImageTagForCredential(registry model.ArtifactRegistry, credential model.RegistryCredential, project model.Project, application model.Application, target model.DeploymentTarget) string {
	return buildapi.BuildStaticTargetImageTagForCredential(registry, credential, project, application, target)
}
func buildImageRef(registry model.ArtifactRegistry, run model.BuildRun) string {
	return buildapi.BuildImageRef(registry, run)
}
func buildImageNamePrefix(registry model.ArtifactRegistry, repository string) string {
	return buildapi.BuildImageNamePrefix(registry, repository)
}
func volumeAuditErrorCode(err error) string { return volumeapi.VolumeAuditErrorCode(err) }
func runtimeExecAuditMessage(command string, result kubeprovider.RuntimeExecResult) string {
	return runtimeapi.RuntimeExecAuditMessage(command, result)
}
func requireRuntimeTerminalTicketForBearer(ctx *gin.Context, ticket string) bool {
	return runtimeapi.RequireRuntimeTerminalTicketForBearer(ctx, ticket)
}
func isDefaultImageRepository(registry model.ArtifactRegistry, project model.Project, application model.Application, repository string) bool {
	return buildapi.IsDefaultImageRepository(registry, project, application, repository)
}
