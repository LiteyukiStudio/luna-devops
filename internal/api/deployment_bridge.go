package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/LiteyukiStudio/devops/internal/api/deploymentapi"
	"github.com/LiteyukiStudio/devops/internal/api/projectapi"
	"github.com/LiteyukiStudio/devops/internal/api/runtimeapi"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	registryprovider "github.com/LiteyukiStudio/devops/internal/provider/registry"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/LiteyukiStudio/devops/internal/security"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type deploymentHost struct{ handlers *Handlers }

func (host deploymentHost) DBFor(ctx *gin.Context) *gorm.DB { return host.handlers.dbFor(ctx) }
func (host deploymentHost) DBWithContext(ctx context.Context) *gorm.DB {
	return host.handlers.dbWithContext(ctx)
}
func (host deploymentHost) SecretStore() secret.Store { return host.handlers.secrets }
func (host deploymentHost) AllowedOrigin(origin string) bool {
	return containsString(host.handlers.config.AllowedOrigins, origin)
}
func (host deploymentHost) EnqueueDeployRun(ctx context.Context, release model.Release) bool {
	return host.handlers.enqueueDeployRun(ctx, release)
}
func (host deploymentHost) ApplyScopedResourceVisibilityForProject(query *gorm.DB, resourceType string, user model.User, projectID string, ctx context.Context) *gorm.DB {
	return host.handlers.applyScopedResourceVisibilityForProject(query, resourceType, user, projectID, ctx)
}
func (host deploymentHost) RegistryPushCredentialForProject(user model.User, registry model.ArtifactRegistry, projectID string, ctx context.Context) (model.RegistryCredential, bool) {
	return host.handlers.registryPushCredentialForProject(user, registry, projectID, ctx)
}
func (host deploymentHost) CanUseScopedResourceByID(user model.User, scope, ownerRef, resourceType, resourceID string, ctx context.Context) bool {
	return host.handlers.canUseScopedResourceByID(user, scope, ownerRef, resourceType, resourceID, ctx)
}
func (host deploymentHost) MutateDeploymentTargetRuntimeSecrets(ctx *gin.Context, user model.User, project model.Project, application model.Application, target model.DeploymentTarget) {
	var input runtimeapi.RuntimeSecretMutationRequest
	if !bindJSON(ctx, &input) {
		return
	}
	mutationInput, ok := runtimeapi.RuntimeSecretMutationInputFromRequest(ctx, input)
	if !ok || !validateRuntimeSecretMutation(ctx, &mutationInput) {
		return
	}
	prepared, err := runtimeapi.PrepareRuntimeSecretMutation(mutationInput)
	if err != nil {
		writeRuntimeSecretMutationError(ctx, "deployment_target", err)
		return
	}
	response, err := host.handlers.runtimeAPI().MutateRuntimeSecrets(ctx.Request.Context(), user, prepared, deploymentTargetRuntimeSecretMutationOwner(target.ID, project.ID, application.ID))
	if err != nil {
		writeRuntimeSecretMutationError(ctx, "deployment_target", err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}
func (host deploymentHost) AuthorizeProject(ctx *gin.Context, action authz.Action) (model.User, model.Project, bool) {
	return host.handlers.authorizeProject(ctx, action)
}
func (host deploymentHost) FindApplication(ctx *gin.Context) (model.Application, bool) {
	return host.handlers.findApplication(ctx)
}
func (host deploymentHost) AuditWithContext(userID, action, resource string, success bool, message string, ctx context.Context) {
	host.handlers.auditWithContext(userID, action, resource, success, message, ctx)
}
func (host deploymentHost) FindBuildEnvironmentConfig(db *gorm.DB, scope, scopeRef string) (model.BuildEnvironmentConfig, error) {
	return host.handlers.findBuildEnvironmentConfig(db, scope, scopeRef)
}
func (host deploymentHost) EnsureProjectCanMutate(ctx *gin.Context, project model.Project) bool {
	return host.handlers.ensureProjectCanMutate(ctx, project)
}
func (host deploymentHost) EnsureBillingAllowsDeployChange(ctx *gin.Context, projectID string) bool {
	return host.handlers.ensureBillingAllowsDeployChange(ctx, projectID)
}
func (host deploymentHost) CanManageBuildEnvironmentProject(ctx *gin.Context, user model.User, projectID string) bool {
	return host.handlers.canManageBuildEnvironmentProject(ctx, user, projectID)
}
func (host deploymentHost) DeploymentBuildEnvironmentFromInput(ctx *gin.Context, user model.User, projectID, targetID string, input deploymentapi.DeploymentTargetInput, existing *model.BuildEnvironmentConfig) (*model.BuildEnvironmentConfig, bool) {
	return host.handlers.deploymentBuildEnvironmentFromInput(ctx, user, projectID, targetID, input, existing)
}
func (host deploymentHost) EnsureDeploymentTargetCanMutate(ctx *gin.Context, target model.DeploymentTarget) bool {
	return host.handlers.ensureDeploymentTargetCanMutate(ctx, target)
}
func (host deploymentHost) EnsureNoIncomingServiceBindings(ctx *gin.Context, projectID, targetApplicationID, targetDeploymentTargetID string) bool {
	return host.handlers.ensureNoIncomingServiceBindings(ctx, projectID, targetApplicationID, targetDeploymentTargetID)
}
func (deploymentHost) DeleteStatusCanStart(status string) bool { return deleteStatusCanStart(status) }
func (deploymentHost) MarkResourceDeleting(tx *gorm.DB, resource any, resourceID string) error {
	return markResourceDeleting(tx, resource, resourceID)
}
func (deploymentHost) MarkDeploymentTargetGatewayRoutesDeleting(tx *gorm.DB, target model.DeploymentTarget) error {
	return markDeploymentTargetGatewayRoutesDeleting(tx, target)
}
func (deploymentHost) MarkResourceDeleteFailed(db *gorm.DB, resource any, resourceID, message string) error {
	return markResourceDeleteFailed(db, resource, resourceID, message)
}
func (deploymentHost) MarkDeploymentTargetGatewayRoutesDeleteFailed(db *gorm.DB, target model.DeploymentTarget, message string) error {
	return markDeploymentTargetGatewayRoutesDeleteFailed(db, target, message)
}
func (deploymentHost) IsResourceDeleteAlreadyStarted(err error) bool {
	return errors.Is(err, errResourceDeleteAlreadyStarted)
}
func (host deploymentHost) EnqueueResourceCleanup(ctx context.Context, resourceType, resourceID, projectID, actorID string) bool {
	return host.handlers.enqueueResourceCleanup(ctx, resourceType, resourceID, projectID, actorID)
}
func (host deploymentHost) RuntimeClusterForProjectUse(ctx *gin.Context, user model.User, projectID, clusterID string) (model.RuntimeCluster, bool) {
	return host.handlers.runtimeAPI().RuntimeClusterForProjectUse(ctx, user, projectID, clusterID)
}
func (host deploymentHost) RuntimeSecretFilesFromInput(ctx *gin.Context, user model.User, ownerID, value string, existing map[string]string) (map[string]string, bool) {
	return host.handlers.runtimeAPI().RuntimeSecretFilesFromInput(ctx, user, ownerID, value, existing)
}
func (host deploymentHost) RuntimeClusterForEnvironment(ctx *gin.Context, environment model.Environment) (model.RuntimeCluster, bool) {
	return host.handlers.runtimeAPI().RuntimeClusterForEnvironment(ctx, environment)
}
func (host deploymentHost) RuntimeClusterForDeploymentTarget(ctx *gin.Context, target model.DeploymentTarget) (model.RuntimeCluster, bool) {
	return host.handlers.runtimeAPI().RuntimeClusterForDeploymentTarget(ctx, target)
}
func (host deploymentHost) RuntimeClusterForDeploymentTargetValue(target model.DeploymentTarget, ctx context.Context) (model.RuntimeCluster, error) {
	return runtimeapi.RuntimeClusterForDeploymentTargetDB(host.handlers.dbWithContext(ctx), target)
}
func (host deploymentHost) RequireContinuousAuthorizationBinding(ctx *gin.Context, user model.User) (projectapi.ContinuousAuthorizationBinding, bool) {
	return host.handlers.requireContinuousAuthorizationBinding(ctx, user)
}
func (host deploymentHost) MonitorContinuousAuthorization(ctx context.Context, binding projectapi.ContinuousAuthorizationBinding, authorizationAllowed func(context.Context, model.User) bool, revoke func()) (<-chan struct{}, bool) {
	return host.handlers.monitorContinuousAuthorization(ctx, binding, authorizationAllowed, revoke)
}
func (host deploymentHost) ProjectContinuousAuthorizationAllowed(ctx context.Context, user model.User, projectID string, action authz.Action) bool {
	return host.handlers.projectContinuousAuthorizationAllowed(ctx, user, projectID, action)
}
func (deploymentHost) ResourceCanMutateDuringDelete(status string) bool {
	return resourceCanMutateDuringDelete(status)
}
func (host deploymentHost) RegistryCredentialInput(ctx context.Context, user model.User, registry model.ArtifactRegistry) registryprovider.Credential {
	return host.handlers.registryCredentialInput(ctx, user, registry)
}
func (host deploymentHost) EgressPolicyForUser(user model.User, ctx context.Context) security.EgressPolicy {
	return host.handlers.egressPolicyForUser(user, ctx)
}
func (host deploymentHost) RequireRuntimeTerminalAuthorization(ctx *gin.Context, user model.User) (runtimeapi.RuntimeTerminalAuthorizationBinding, bool) {
	return host.handlers.runtimeAPI().RequireRuntimeTerminalAuthorization(ctx, user)
}
func (host deploymentHost) ConsumeRuntimeTerminalTicket(ctx context.Context, ticket string) (runtimeapi.RuntimeTerminalTicketValue, bool, error) {
	return host.handlers.runtimeAPI().ConsumeRuntimeTerminalTicket(ctx, ticket)
}
func (host deploymentHost) IssueRuntimeTerminalTicket(ctx context.Context, authorization runtimeapi.RuntimeTerminalAuthorizationBinding, resourceKind string, resource any) (string, time.Time, error) {
	return host.handlers.runtimeAPI().IssueRuntimeTerminalTicket(ctx, authorization, resourceKind, resource)
}
func (host deploymentHost) ContinuousAuthorizationActive(ctx context.Context, binding runtimeapi.RuntimeTerminalAuthorizationBinding, authorizationAllowed func(context.Context, model.User) bool) bool {
	return host.handlers.runtimeAPI().ContinuousAuthorizationActive(ctx, binding, authorizationAllowed)
}
func (host deploymentHost) ReleaseRuntimeTerminalAuthorizationAllowed(ctx context.Context, user model.User, reference runtimeapi.ReleaseRuntimeTerminalAuthorizationReference) bool {
	return host.handlers.runtimeAPI().ReleaseRuntimeTerminalAuthorizationAllowed(ctx, user, reference)
}
func (host deploymentHost) FindProject(ctx *gin.Context) (model.Project, bool) {
	return host.handlers.findProject(ctx)
}
func (host deploymentHost) ProjectRoleActionAllowed(ctx context.Context, user model.User, projectID string, action authz.Action) (bool, error) {
	return host.handlers.projectRoleActionAllowed(ctx, user, projectID, action)
}

func (h *Handlers) deploymentAPI() *deploymentapi.Handler {
	return deploymentapi.New(deploymentHost{handlers: h})
}

func (h *Handlers) ExportDeploymentTargetBundle(ctx *gin.Context) {
	h.deploymentAPI().ExportDeploymentTargetBundle(ctx)
}
func (h *Handlers) PreviewDeploymentTargetBundleImport(ctx *gin.Context) {
	h.deploymentAPI().PreviewDeploymentTargetBundleImport(ctx)
}
func (h *Handlers) ListDeploymentTargetBundleReferenceCandidates(ctx *gin.Context) {
	h.deploymentAPI().ListDeploymentTargetBundleReferenceCandidates(ctx)
}
func (h *Handlers) ImportDeploymentTargetBundle(ctx *gin.Context) {
	h.deploymentAPI().ImportDeploymentTargetBundle(ctx)
}
func (h *Handlers) ListDeploymentTargets(ctx *gin.Context) {
	h.deploymentAPI().ListDeploymentTargets(ctx)
}
func (h *Handlers) CreateDeploymentTarget(ctx *gin.Context) {
	h.deploymentAPI().CreateDeploymentTarget(ctx)
}
func (h *Handlers) UpdateDeploymentTarget(ctx *gin.Context) {
	h.deploymentAPI().UpdateDeploymentTarget(ctx)
}
func (h *Handlers) RestartDeploymentTarget(ctx *gin.Context) {
	h.deploymentAPI().RestartDeploymentTarget(ctx)
}
func (h *Handlers) DeleteDeploymentTarget(ctx *gin.Context) {
	h.deploymentAPI().DeleteDeploymentTarget(ctx)
}
func (h *Handlers) StreamDeploymentTargetMetrics(ctx *gin.Context) {
	h.deploymentAPI().StreamDeploymentTargetMetrics(ctx)
}
func (h *Handlers) GetDeploymentTargetRuntimeSecretsSummary(ctx *gin.Context) {
	h.deploymentAPI().GetDeploymentTargetRuntimeSecretsSummary(ctx)
}
func (h *Handlers) UpdateDeploymentTargetRuntimeSecrets(ctx *gin.Context) {
	h.deploymentAPI().UpdateDeploymentTargetRuntimeSecrets(ctx)
}
func (h *Handlers) ListReleases(ctx *gin.Context)    { h.deploymentAPI().ListReleases(ctx) }
func (h *Handlers) GetRelease(ctx *gin.Context)      { h.deploymentAPI().GetRelease(ctx) }
func (h *Handlers) CreateRelease(ctx *gin.Context)   { h.deploymentAPI().CreateRelease(ctx) }
func (h *Handlers) RollbackRelease(ctx *gin.Context) { h.deploymentAPI().RollbackRelease(ctx) }
func (h *Handlers) GetReleaseLogs(ctx *gin.Context)  { h.deploymentAPI().GetReleaseLogs(ctx) }
func (h *Handlers) ListReleaseImageCandidates(ctx *gin.Context) {
	h.deploymentAPI().ListReleaseImageCandidates(ctx)
}
func (h *Handlers) GetReleaseRuntimeLogs(ctx *gin.Context) {
	h.deploymentAPI().GetReleaseRuntimeLogs(ctx)
}
func (h *Handlers) ExecReleaseRuntimeCommand(ctx *gin.Context) {
	h.deploymentAPI().ExecReleaseRuntimeCommand(ctx)
}
func (h *Handlers) StreamReleaseRuntimeTerminal(ctx *gin.Context) {
	h.deploymentAPI().StreamReleaseRuntimeTerminal(ctx)
}
func (h *Handlers) AuthorizeReleaseRuntimeTerminal(ctx *gin.Context) {
	h.deploymentAPI().AuthorizeReleaseRuntimeTerminal(ctx)
}

type deploymentTargetResponse = deploymentapi.DeploymentTargetResponse
type deploymentTargetInput = deploymentapi.DeploymentTargetInput
type deploymentTargetEmptyDirInput = deploymentapi.DeploymentTargetEmptyDirInput
type deploymentTargetDataVolumeInput = deploymentapi.DeploymentTargetDataVolumeInput
type deploymentTargetDataVolumeResponse = deploymentapi.DeploymentTargetDataVolumeResponse
type deploymentTargetHookBindingInput = deploymentapi.DeploymentTargetHookBindingInput
type deploymentRuntimeConfigRefInput = deploymentapi.DeploymentRuntimeConfigRefInput
type deploymentRuntimeConfigRefResponse = deploymentapi.DeploymentRuntimeConfigRefResponse
type deploymentVolumeMountChanges = deploymentapi.DeploymentVolumeMountChanges
type deploymentVolumeAuditRecord = deploymentapi.DeploymentVolumeAuditRecord
type releaseInput = deploymentapi.ReleaseInput
type releaseRuntimeExecInput = deploymentapi.ReleaseRuntimeExecInput
type deploymentTargetBundle = deploymentapi.DeploymentTargetBundle
type deploymentBundleReference = deploymentapi.DeploymentBundleReference
type deploymentBundleReferenceDescriptor = deploymentapi.DeploymentBundleReferenceDescriptor
type deploymentBundleSecretRequirement = deploymentapi.DeploymentBundleSecretRequirement
type deploymentTargetBundleImportRequest = deploymentapi.DeploymentTargetBundleImportRequest
type deploymentTargetBundleOverrides = deploymentapi.DeploymentTargetBundleOverrides
type deploymentTargetBundlePreview = deploymentapi.DeploymentTargetBundlePreview
type deploymentTargetBundlePreviewSummary = deploymentapi.DeploymentTargetBundlePreviewSummary
type deploymentBundleReferenceResolution = deploymentapi.DeploymentBundleReferenceResolution
type deploymentBundleReferenceCandidatesRequest = deploymentapi.DeploymentBundleReferenceCandidatesRequest
type deploymentBundleReferenceCandidatePage = deploymentapi.DeploymentBundleReferenceCandidatePage
type deploymentBundleReferenceCandidate = deploymentapi.DeploymentBundleReferenceCandidate
type deploymentTargetBundleImportPlan = deploymentapi.DeploymentTargetBundleImportPlan
type deploymentBundleSecretValue = deploymentapi.DeploymentBundleSecretValue
type deploymentBundleError = deploymentapi.DeploymentBundleError
type deploymentBundleErrorSpec = deploymentapi.DeploymentBundleErrorSpec
type deploymentBundleCandidateQuery = deploymentapi.DeploymentBundleCandidateQuery
type deploymentBundleCandidate = deploymentapi.DeploymentBundleCandidate
type deploymentKubernetesAdvancedInput = deploymentapi.DeploymentKubernetesAdvancedInput
type deploymentAutoScalingInput = deploymentapi.DeploymentAutoScalingInput
type deploymentTargetMetricsResponse = deploymentapi.DeploymentTargetMetricsResponse

const (
	deploymentBundleKind           = deploymentapi.DeploymentBundleKind
	deploymentBundleSchemaVersion  = deploymentapi.DeploymentBundleSchemaVersion
	deploymentBundleMaxBytes       = deploymentapi.DeploymentBundleMaxBytes
	deploymentBundleMaxDepth       = deploymentapi.DeploymentBundleMaxDepth
	deploymentBundleMaxReferences  = deploymentapi.DeploymentBundleMaxReferences
	deploymentBundleSecretMaxBytes = deploymentapi.DeploymentBundleSecretMaxBytes

	deploymentBundleStatusReady           = deploymentapi.DeploymentBundleStatusReady
	deploymentBundleStatusRequiresMapping = deploymentapi.DeploymentBundleStatusRequiresMapping
	deploymentBundleStatusInvalid         = deploymentapi.DeploymentBundleStatusInvalid

	deploymentBundleReferenceResolved     = deploymentapi.DeploymentBundleReferenceResolved
	deploymentBundleReferenceMissing      = deploymentapi.DeploymentBundleReferenceMissing
	deploymentBundleReferenceAmbiguous    = deploymentapi.DeploymentBundleReferenceAmbiguous
	deploymentBundleReferenceForbidden    = deploymentapi.DeploymentBundleReferenceForbidden
	deploymentBundleReferenceIncompatible = deploymentapi.DeploymentBundleReferenceIncompatible

	deploymentBundleReferenceRepositoryBinding = deploymentapi.DeploymentBundleReferenceRepositoryBinding
	deploymentBundleReferenceRuntimeCluster    = deploymentapi.DeploymentBundleReferenceRuntimeCluster
	deploymentBundleReferenceArtifactRegistry  = deploymentapi.DeploymentBundleReferenceArtifactRegistry
	deploymentBundleReferenceBuildVariableSet  = deploymentapi.DeploymentBundleReferenceBuildVariableSet
	deploymentBundleReferenceRuntimeConfigSet  = deploymentapi.DeploymentBundleReferenceRuntimeConfigSet
	deploymentBundleReferenceHookConfig        = deploymentapi.DeploymentBundleReferenceHookConfig
	deploymentBundleReferenceProjectVolume     = deploymentapi.DeploymentBundleReferenceProjectVolume

	deploymentBundleSecretBuild       = deploymentapi.DeploymentBundleSecretBuild
	deploymentBundleSecretRuntimeEnv  = deploymentapi.DeploymentBundleSecretRuntimeEnv
	deploymentBundleSecretRuntimeFile = deploymentapi.DeploymentBundleSecretRuntimeFile
	maxDeploymentDataVolumes          = deploymentapi.MaxDeploymentDataVolumes
	defaultBuildTimeoutSeconds        = deploymentapi.DefaultBuildTimeoutSeconds
)

var errDeploymentStageExists = deploymentapi.ErrDeploymentStageExists
var deploymentBundleErrorCatalog = deploymentapi.DeploymentBundleErrorCatalog
var publicDeploymentStages = append([]string(nil), deploymentapi.PublicDeploymentStages...)

func normalizeStage(value string) string { return deploymentapi.NormalizeStage(value) }
func normalizePublicStage(value string) (string, bool) {
	return deploymentapi.NormalizePublicStage(value)
}
func writeDeploymentStageInvalid(ctx *gin.Context, path, detail string) {
	deploymentapi.WriteDeploymentStageInvalid(ctx, path, detail)
}
func writeDeploymentStageConflict(ctx *gin.Context, deleteStatus string) {
	deploymentapi.WriteDeploymentStageConflict(ctx, deleteStatus)
}
func normalizeDataVolumes(ctx *gin.Context, raw []deploymentTargetDataVolumeInput) ([]deploymentTargetDataVolumeInput, bool) {
	return deploymentapi.NormalizeDataVolumes(ctx, raw)
}
func normalizeDeploymentSourceType(value string) string {
	return deploymentapi.NormalizeDeploymentSourceType(value)
}
func normalizeDeploymentServicePortName(value string, port, index int) string {
	return deploymentapi.NormalizeDeploymentServicePortName(value, port, index)
}
func normalizeDeploymentKubernetesAdvanced(ctx *gin.Context, input deploymentTargetInput) (deploymentKubernetesAdvancedInput, bool) {
	return deploymentapi.NormalizeDeploymentKubernetesAdvanced(ctx, input)
}
func runtimeConfigRefInputs(input deploymentTargetInput) []deploymentRuntimeConfigRefInput {
	return deploymentapi.RuntimeConfigRefInputs(input)
}
func normalizeSecretRefsInput(value string) string {
	return deploymentapi.NormalizeSecretRefsInput(value)
}
func normalizeBuildTimeoutSeconds(ctx *gin.Context, value int) (int, bool) {
	return deploymentapi.NormalizeBuildTimeoutSeconds(ctx, value)
}
func normalizeBuildTimeoutSecondsValue(value int) int {
	return deploymentapi.NormalizeBuildTimeoutSecondsValue(value)
}
func normalizeBuildResourceQuantity(ctx *gin.Context, value, fallbackValue, label string) (string, bool) {
	return deploymentapi.NormalizeBuildResourceQuantity(ctx, value, fallbackValue, label)
}
func normalizeBuildResourceQuantityValue(value, fallbackValue, label string) (string, error) {
	return deploymentapi.NormalizeBuildResourceQuantityValue(value, fallbackValue, label)
}
func normalizeWebConsoleOverride(value *bool) *bool {
	return deploymentapi.NormalizeWebConsoleOverride(value)
}
func runtimeWebConsoleEnabled(project model.Project, target model.DeploymentTarget) bool {
	return deploymentapi.RuntimeWebConsoleEnabled(project, target)
}
func ensureRuntimeWebConsoleEnabled(ctx *gin.Context, project model.Project, target model.DeploymentTarget) bool {
	return deploymentapi.EnsureRuntimeWebConsoleEnabled(ctx, project, target)
}
func deploymentTargetResponses(targets []model.DeploymentTarget, mountsByTarget map[string][]model.DeploymentVolumeMount) []deploymentTargetResponse {
	return deploymentapi.DeploymentTargetResponses(targets, mountsByTarget)
}
func deploymentTargetPageQuery(query *gorm.DB, pagination paginationParams) *gorm.DB {
	return deploymentapi.DeploymentTargetPageQuery(query, pagination)
}
func deploymentTargetResponseFromModel(target model.DeploymentTarget, mounts ...[]model.DeploymentVolumeMount) deploymentTargetResponse {
	return deploymentapi.DeploymentTargetResponseFromModel(target, mounts...)
}
func deploymentTargetEnvironmentProfile(target model.DeploymentTarget) model.Environment {
	return deploymentapi.DeploymentTargetEnvironmentProfile(target)
}
func deploymentTargetDataVolumeResponses(mounts []model.DeploymentVolumeMount) []deploymentTargetDataVolumeResponse {
	return deploymentapi.DeploymentTargetDataVolumeResponses(mounts)
}
func deploymentRuntimeConfigRefsResponse(target model.DeploymentTarget) []deploymentRuntimeConfigRefResponse {
	return deploymentapi.DeploymentRuntimeConfigRefsResponse(target)
}
func deploymentTargetResourceName(target model.DeploymentTarget) string {
	return deploymentapi.DeploymentTargetResourceName(target)
}
func shortResourceID(value string) string { return deploymentapi.ShortResourceID(value) }
func runtimeIDResourceName(prefix, value string) string {
	return deploymentapi.RuntimeIDResourceName(prefix, value)
}
func runtimeShortID(value string) string  { return deploymentapi.RuntimeShortID(value) }
func runtimeDNSLabel(value string) string { return deploymentapi.RuntimeDNSLabel(value) }
func syncDeploymentTargetVolumeMounts(ctx context.Context, tx *gorm.DB, target model.DeploymentTarget, inputs []deploymentTargetDataVolumeInput) (deploymentVolumeMountChanges, error) {
	return deploymentapi.SyncDeploymentTargetVolumeMounts(ctx, tx, target, inputs)
}
func deploymentVolumeFailureAuditRecords(changes deploymentVolumeMountChanges, err error) []deploymentVolumeAuditRecord {
	return deploymentapi.DeploymentVolumeFailureAuditRecords(changes, err)
}
func deploymentVolumeAuditRecords(target model.DeploymentTarget, changes deploymentVolumeMountChanges) []deploymentVolumeAuditRecord {
	return deploymentapi.DeploymentVolumeAuditRecords(target, changes)
}
func deploymentTargetMatchesBuildRun(target model.DeploymentTarget, run model.BuildRun) bool {
	return deploymentapi.DeploymentTargetMatchesBuildRun(target, run)
}
func matchesDeploymentPattern(patterns, value string) bool {
	return deploymentapi.MatchesDeploymentPattern(patterns, value)
}
func nextReleaseRevisionFor(tx *gorm.DB, projectID, applicationID, deploymentTargetID string) (int, error) {
	return deploymentapi.NextReleaseRevisionFor(tx, projectID, applicationID, deploymentTargetID)
}
func releaseFromInput(projectID, userID string, input releaseInput, releaseID string) model.Release {
	return deploymentapi.ReleaseFromInput(projectID, userID, input, releaseID)
}
func rollbackReleaseFromTarget(source, target model.Release, userID string, revision int) model.Release {
	return deploymentapi.RollbackReleaseFromTarget(source, target, userID, revision)
}
func normalizeReleaseType(value string) string { return deploymentapi.NormalizeReleaseType(value) }
func bindDeploymentBundleJSON(ctx *gin.Context, destination *deploymentTargetBundleImportRequest) bool {
	return deploymentapi.BindDeploymentBundleJSON(ctx, destination)
}
func validateDeploymentBundleJSON(payload []byte) error {
	return deploymentapi.ValidateDeploymentBundleJSON(payload)
}
func deploymentBundleConfiguration(target model.DeploymentTarget, mounts []model.DeploymentVolumeMount, buildEnvironment model.BuildEnvironmentConfig) (deploymentTargetInput, error) {
	return deploymentapi.DeploymentBundleConfiguration(target, mounts, buildEnvironment)
}
func validateResolvedDeploymentBundle(input deploymentTargetInput, references []deploymentBundleReference, resolved map[string]string) error {
	return deploymentapi.ValidateResolvedDeploymentBundle(input, references, resolved)
}
func validateDeploymentTargetBundle(bundle deploymentTargetBundle) error {
	return deploymentapi.ValidateDeploymentTargetBundle(bundle)
}
func deploymentBundleDigest(bundle deploymentTargetBundle) (string, error) {
	return deploymentapi.DeploymentBundleDigest(bundle)
}
func deploymentBundleErrorCode(err error) string { return deploymentapi.DeploymentBundleErrorCode(err) }
func writeDeploymentBundleError(ctx *gin.Context, err error) {
	deploymentapi.WriteDeploymentBundleError(ctx, err)
}
func deploymentBundleErrorSpecFor(code string) (deploymentBundleErrorSpec, bool) {
	return deploymentapi.DeploymentBundleErrorSpecFor(code)
}
func deploymentBundleFilenamePart(value string) string {
	return deploymentapi.DeploymentBundleFilenamePart(value)
}
func deploymentBundleVolumeDestinationCompatible(projectNamespace string, input deploymentTargetInput, projectVolume model.ProjectVolume) bool {
	return deploymentapi.DeploymentBundleVolumeDestinationCompatible(projectNamespace, input, projectVolume)
}
func normalizeDeploymentBundleCandidateQuery(query deploymentBundleCandidateQuery) deploymentBundleCandidateQuery {
	return deploymentapi.NormalizeDeploymentBundleCandidateQuery(query)
}
func appendCompatibleDeploymentBundleMatches(matches, candidates []deploymentBundleCandidate) []deploymentBundleCandidate {
	return deploymentapi.AppendCompatibleDeploymentBundleMatches(matches, candidates)
}
func deploymentBundleReferenceDescriptorMatches(source, candidate deploymentBundleReferenceDescriptor) bool {
	return deploymentapi.DeploymentBundleReferenceDescriptorMatches(source, candidate)
}
func deploymentTargetMetricsResponseFromSnapshot(snapshot kubeprovider.RuntimeMetricsSnapshot, target model.DeploymentTarget) deploymentTargetMetricsResponse {
	return deploymentapi.DeploymentTargetMetricsResponseFromSnapshot(snapshot, target)
}
func deploymentObservationFromSnapshot(snapshot kubeprovider.DeploymentSnapshot) string {
	return deploymentapi.DeploymentObservationFromSnapshot(snapshot)
}

func (h *Handlers) createDeploymentTarget(target model.DeploymentTarget, dataVolumes []deploymentTargetDataVolumeInput, hookInputs []deploymentTargetHookBindingInput, buildEnvironment *model.BuildEnvironmentConfig, ctx context.Context) (deploymentVolumeMountChanges, error) {
	return h.deploymentAPI().CreateDeploymentTargetModel(target, dataVolumes, hookInputs, buildEnvironment, ctx)
}
func (h *Handlers) saveDeploymentTarget(target model.DeploymentTarget, dataVolumes []deploymentTargetDataVolumeInput, hookInputs []deploymentTargetHookBindingInput, buildEnvironment *model.BuildEnvironmentConfig, ctx context.Context) (deploymentVolumeMountChanges, error) {
	return h.deploymentAPI().SaveDeploymentTargetModel(target, dataVolumes, hookInputs, buildEnvironment, ctx)
}
func (h *Handlers) attachDeploymentTargetHookBindings(targets []model.DeploymentTarget, ctx context.Context) error {
	return h.deploymentAPI().AttachDeploymentTargetHookBindings(targets, ctx)
}
func (h *Handlers) deploymentTargetWithHookBindings(target model.DeploymentTarget, ctx context.Context) (model.DeploymentTarget, error) {
	return h.deploymentAPI().DeploymentTargetWithHookBindings(target, ctx)
}
func (h *Handlers) deploymentTargetVolumeMountsByTarget(ctx context.Context, targets []model.DeploymentTarget) (map[string][]model.DeploymentVolumeMount, error) {
	return h.deploymentAPI().DeploymentTargetVolumeMountsByTarget(ctx, targets)
}
func (h *Handlers) auditDeploymentVolumeMountChanges(ctx context.Context, userID string, target model.DeploymentTarget, changes deploymentVolumeMountChanges) {
	h.deploymentAPI().AuditDeploymentVolumeMountChanges(ctx, userID, target, changes)
}
func (h *Handlers) auditDeploymentVolumeMountFailure(ctx context.Context, userID string, changes deploymentVolumeMountChanges, err error) {
	h.deploymentAPI().AuditDeploymentVolumeMountFailure(ctx, userID, changes, err)
}
func (h *Handlers) observeDeploymentTargets(ctx context.Context, project model.Project, targets []model.DeploymentTarget) {
	h.deploymentAPI().ObserveDeploymentTargets(ctx, project, targets)
}
func (h *Handlers) observeDeploymentTarget(ctx context.Context, project model.Project, target model.DeploymentTarget) model.DeploymentTarget {
	return h.deploymentAPI().ObserveDeploymentTarget(ctx, project, target)
}
func (h *Handlers) kubernetesClientForDeploymentTargetObservation(project model.Project, target model.DeploymentTarget, ctx context.Context) (*kubeprovider.Client, string, string) {
	return h.deploymentAPI().KubernetesClientForDeploymentTargetObservation(project, target, ctx)
}
func (h *Handlers) findRelease(ctx *gin.Context) (model.Release, bool) {
	return h.deploymentAPI().FindRelease(ctx)
}
func (h *Handlers) buildDeploymentTargetImportPlan(ctx *gin.Context, user model.User, project model.Project, application model.Application, request deploymentTargetBundleImportRequest, requireSecrets bool) (deploymentTargetBundleImportPlan, error) {
	return h.deploymentAPI().BuildDeploymentTargetImportPlan(ctx, user, project, application, request, requireSecrets)
}
func (h *Handlers) deploymentBundleCandidates(ctx context.Context, user model.User, project model.Project, application model.Application, reference deploymentBundleReference, query deploymentBundleCandidateQuery) (deploymentBundleReferenceCandidatePage, []deploymentBundleCandidate, error) {
	return h.deploymentAPI().DeploymentBundleCandidates(ctx, user, project, application, reference, query)
}
func (h *Handlers) resolveDeploymentBundleReference(ctx *gin.Context, user model.User, project model.Project, application model.Application, reference deploymentBundleReference, mappedID string) (deploymentBundleReferenceResolution, []deploymentBundleCandidate, error) {
	return h.deploymentAPI().ResolveDeploymentBundleReference(ctx, user, project, application, reference, mappedID)
}
func (h *Handlers) enqueueAutoDeploymentsForBuildRun(ctx context.Context, run model.BuildRun) {
	h.deploymentAPI().EnqueueAutoDeploymentsForBuildRun(ctx, run)
}

func (h *Handlers) enqueueDeployRun(ctx context.Context, release model.Release) bool {
	if h.taskClient == nil {
		return false
	}
	_, err := h.taskClient.EnqueueDeployRun(ctx, tasks.DeployRunPayload{
		ReleaseID: release.ID, ProjectID: release.ProjectID, ActorID: release.CreatedBy,
	})
	return err == nil
}

func deploymentTargetRuntimeSecretMutationOwner(targetID, projectID, applicationID string) runtimeapi.RuntimeSecretMutationOwner {
	return runtimeapi.RuntimeSecretMutationOwner{
		ResourceID: targetID, ResourcePrefix: "deployment_target:" + targetID + ":runtime", AuditAction: "deployment_target.runtime_secrets.update",
		LoadRefs: func(tx *gorm.DB) (string, error) {
			var current model.DeploymentTarget
			err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id", "secret_refs").First(&current, "id = ? and project_id = ? and application_id = ? and delete_status in ?", targetID, projectID, applicationID, []string{"", "active", "delete_failed"}).Error
			return current.SecretRefs, err
		},
		SaveRefs: func(tx *gorm.DB, encoded string) error {
			return tx.Model(&model.DeploymentTarget{}).Where("id = ? and project_id = ? and application_id = ?", targetID, projectID, applicationID).Update("secret_refs", encoded).Error
		},
		EncodeRefs: deploymentapi.EncodeStringMap,
	}
}
