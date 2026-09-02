package deploymentapi

import (
	"context"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	DeploymentBundleKind           = deploymentBundleKind
	DeploymentBundleSchemaVersion  = deploymentBundleSchemaVersion
	DeploymentBundleMaxBytes       = deploymentBundleMaxBytes
	DeploymentBundleMaxDepth       = deploymentBundleMaxDepth
	DeploymentBundleMaxReferences  = deploymentBundleMaxReferences
	DeploymentBundleSecretMaxBytes = deploymentBundleSecretMaxBytes

	DeploymentBundleStatusReady           = deploymentBundleStatusReady
	DeploymentBundleStatusRequiresMapping = deploymentBundleStatusRequiresMapping
	DeploymentBundleStatusInvalid         = deploymentBundleStatusInvalid

	DeploymentBundleReferenceResolved     = deploymentBundleReferenceResolved
	DeploymentBundleReferenceMissing      = deploymentBundleReferenceMissing
	DeploymentBundleReferenceAmbiguous    = deploymentBundleReferenceAmbiguous
	DeploymentBundleReferenceForbidden    = deploymentBundleReferenceForbidden
	DeploymentBundleReferenceIncompatible = deploymentBundleReferenceIncompatible

	DeploymentBundleReferenceRepositoryBinding = deploymentBundleReferenceRepositoryBinding
	DeploymentBundleReferenceRuntimeCluster    = deploymentBundleReferenceRuntimeCluster
	DeploymentBundleReferenceArtifactRegistry  = deploymentBundleReferenceArtifactRegistry
	DeploymentBundleReferenceBuildVariableSet  = deploymentBundleReferenceBuildVariableSet
	DeploymentBundleReferenceRuntimeConfigSet  = deploymentBundleReferenceRuntimeConfigSet
	DeploymentBundleReferenceHookConfig        = deploymentBundleReferenceHookConfig
	DeploymentBundleReferenceProjectVolume     = deploymentBundleReferenceProjectVolume

	DeploymentBundleSecretBuild       = deploymentBundleSecretBuild
	DeploymentBundleSecretRuntimeEnv  = deploymentBundleSecretRuntimeEnv
	DeploymentBundleSecretRuntimeFile = deploymentBundleSecretRuntimeFile

	MaxDeploymentDataVolumes   = maxDeploymentDataVolumes
	DefaultBuildTimeoutSeconds = defaultBuildTimeoutSeconds
)

var ErrDeploymentStageExists = errDeploymentStageExists
var DeploymentBundleErrorCatalog = deploymentBundleErrorCatalog
var PublicDeploymentStages = append([]string(nil), publicDeploymentStages...)

type DeploymentTargetResponse = deploymentTargetResponse
type DeploymentTargetInput = deploymentTargetInput
type DeploymentTargetEmptyDirInput = deploymentTargetEmptyDirInput
type DeploymentTargetDataVolumeInput = deploymentTargetDataVolumeInput
type DeploymentTargetDataVolumeResponse = deploymentTargetDataVolumeResponse
type DeploymentTargetHookBindingInput = deploymentTargetHookBindingInput
type DeploymentRuntimeConfigRefInput = deploymentRuntimeConfigRefInput
type DeploymentRuntimeConfigRefResponse = deploymentRuntimeConfigRefResponse
type DeploymentVolumeMountChanges = deploymentVolumeMountChanges
type DeploymentVolumeAuditRecord = deploymentVolumeAuditRecord

type DeploymentTargetBundle = deploymentTargetBundle
type DeploymentBundleReference = deploymentBundleReference
type DeploymentBundleReferenceDescriptor = deploymentBundleReferenceDescriptor
type DeploymentBundleSecretRequirement = deploymentBundleSecretRequirement
type DeploymentTargetBundleImportRequest = deploymentTargetBundleImportRequest
type DeploymentTargetBundleOverrides = deploymentTargetBundleOverrides
type DeploymentTargetBundlePreview = deploymentTargetBundlePreview
type DeploymentTargetBundlePreviewSummary = deploymentTargetBundlePreviewSummary
type DeploymentBundleReferenceResolution = deploymentBundleReferenceResolution
type DeploymentBundleReferenceCandidatesRequest = deploymentBundleReferenceCandidatesRequest
type DeploymentBundleReferenceCandidatePage = deploymentBundleReferenceCandidatePage
type DeploymentBundleReferenceCandidate = deploymentBundleReferenceCandidate
type DeploymentTargetBundleImportPlan = deploymentTargetBundleImportPlan
type DeploymentBundleSecretValue = deploymentBundleSecretValue
type DeploymentBundleError = deploymentBundleError
type DeploymentBundleErrorSpec = deploymentBundleErrorSpec
type DeploymentBundleCandidateQuery = deploymentBundleCandidateQuery
type DeploymentBundleCandidate = deploymentBundleCandidate

type DeploymentKubernetesAdvancedInput = deploymentKubernetesAdvancedInput
type DeploymentAutoScalingInput = deploymentAutoScalingInput
type DeploymentTargetRuntimeSecretsSummary = deploymentTargetRuntimeSecretsSummary
type DeploymentMetricsAuthorizationReference = deploymentMetricsAuthorizationReference
type DeploymentTargetMetricsResponse = deploymentTargetMetricsResponse
type ReleaseInput = releaseInput
type ReleaseImageCandidateOutput = releaseImageCandidateOutput
type ReleaseImageCandidatesOutput = releaseImageCandidatesOutput
type ReleaseRuntimeExecInput = releaseRuntimeExecInput

func NormalizeStage(value string) string               { return normalizeStage(value) }
func NormalizePublicStage(value string) (string, bool) { return normalizePublicStage(value) }
func WriteDeploymentStageInvalid(ctx *gin.Context, path, detail string) {
	writeDeploymentStageInvalid(ctx, path, detail)
}
func WriteDeploymentStageConflict(ctx *gin.Context, deleteStatus string) {
	writeDeploymentStageConflict(ctx, deleteStatus)
}
func NormalizeDataVolumes(ctx *gin.Context, raw []DeploymentTargetDataVolumeInput) ([]DeploymentTargetDataVolumeInput, bool) {
	return normalizeDataVolumes(ctx, raw)
}
func RuntimeConfigFilePaths(value string) []string { return runtimeConfigFilePaths(value) }
func RuntimeDataPathConflicts(mountPath string, configValues ...string) bool {
	return runtimeDataPathConflicts(mountPath, configValues...)
}
func NormalizeDeploymentSourceType(value string) string { return normalizeDeploymentSourceType(value) }
func NormalizeBuildTimeoutSeconds(ctx *gin.Context, value int) (int, bool) {
	return normalizeBuildTimeoutSeconds(ctx, value)
}
func NormalizeBuildTimeoutSecondsValue(value int) int {
	return normalizeBuildTimeoutSecondsValue(value)
}
func NormalizeDeploymentServicePorts(ctx *gin.Context, input []model.DeploymentServicePort, fallbackPort int) ([]model.DeploymentServicePort, bool) {
	return normalizeDeploymentServicePorts(ctx, input, fallbackPort)
}
func NormalizeDeploymentServicePortName(value string, port, index int) string {
	return normalizeDeploymentServicePortName(value, port, index)
}
func NormalizeBuildResourceQuantity(ctx *gin.Context, value, fallbackValue, label string) (string, bool) {
	return normalizeBuildResourceQuantity(ctx, value, fallbackValue, label)
}
func NormalizeBuildResourceQuantityValue(value, fallbackValue, label string) (string, error) {
	return normalizeBuildResourceQuantityValue(value, fallbackValue, label)
}
func NormalizeDeploymentKubernetesAdvanced(ctx *gin.Context, input DeploymentTargetInput) (DeploymentKubernetesAdvancedInput, bool) {
	return normalizeDeploymentKubernetesAdvanced(ctx, input)
}
func RuntimeConfigRefInputs(input DeploymentTargetInput) []DeploymentRuntimeConfigRefInput {
	return runtimeConfigRefInputs(input)
}
func NormalizeSecretRefsInput(value string) string { return normalizeSecretRefsInput(value) }
func NormalizeDeploymentAutoScaling(ctx *gin.Context, input DeploymentTargetInput, replicas int) (DeploymentAutoScalingInput, bool) {
	return normalizeDeploymentAutoScaling(ctx, input, replicas)
}
func NormalizeWebConsoleOverride(value *bool) *bool { return normalizeWebConsoleOverride(value) }
func RuntimeWebConsoleEnabled(project model.Project, target model.DeploymentTarget) bool {
	return runtimeWebConsoleEnabled(project, target)
}
func EnsureRuntimeWebConsoleEnabled(ctx *gin.Context, project model.Project, target model.DeploymentTarget) bool {
	return ensureRuntimeWebConsoleEnabled(ctx, project, target)
}

func DeploymentTargetResponses(targets []model.DeploymentTarget, mountsByTarget map[string][]model.DeploymentVolumeMount) []DeploymentTargetResponse {
	return deploymentTargetResponses(targets, mountsByTarget)
}
func DeploymentTargetPageQuery(query *gorm.DB, pagination paginationParams) *gorm.DB {
	return deploymentTargetPageQuery(query, pagination)
}
func DeploymentTargetResponseFromModel(target model.DeploymentTarget, mounts ...[]model.DeploymentVolumeMount) DeploymentTargetResponse {
	return deploymentTargetResponseFromModel(target, mounts...)
}
func DeploymentTargetEnvironmentProfile(target model.DeploymentTarget) model.Environment {
	return deploymentTargetEnvironmentProfile(target)
}
func DeploymentTargetDataVolumeResponses(mounts []model.DeploymentVolumeMount) []DeploymentTargetDataVolumeResponse {
	return deploymentTargetDataVolumeResponses(mounts)
}
func DeploymentRuntimeConfigRefsResponse(target model.DeploymentTarget) []DeploymentRuntimeConfigRefResponse {
	return deploymentRuntimeConfigRefsResponse(target)
}
func RuntimeProjectNamespace(project model.Project) string { return runtimeProjectNamespace(project) }
func DeploymentTargetResourceName(target model.DeploymentTarget) string {
	return deploymentTargetResourceName(target)
}
func ShortResourceID(value string) string               { return shortResourceID(value) }
func RuntimeIDResourceName(prefix, value string) string { return runtimeIDResourceName(prefix, value) }
func RuntimeShortID(value string) string                { return runtimeShortID(value) }
func RuntimeDNSLabel(value string) string               { return runtimeDNSLabel(value) }

func SyncDeploymentTargetVolumeMounts(ctx context.Context, tx *gorm.DB, target model.DeploymentTarget, inputs []DeploymentTargetDataVolumeInput) (DeploymentVolumeMountChanges, error) {
	return syncDeploymentTargetVolumeMounts(ctx, tx, target, inputs)
}
func DeploymentVolumeFailureAuditRecords(changes DeploymentVolumeMountChanges, err error) []DeploymentVolumeAuditRecord {
	return deploymentVolumeFailureAuditRecords(changes, err)
}
func DeploymentVolumeAuditRecords(target model.DeploymentTarget, changes DeploymentVolumeMountChanges) []DeploymentVolumeAuditRecord {
	return deploymentVolumeAuditRecords(target, changes)
}

func DeploymentTargetMatchesBuildRun(target model.DeploymentTarget, run model.BuildRun) bool {
	return deploymentTargetMatchesBuildRun(target, run)
}
func MatchesDeploymentPattern(patterns, value string) bool {
	return matchesDeploymentPattern(patterns, value)
}
func NextReleaseRevisionFor(tx *gorm.DB, projectID, applicationID, deploymentTargetID string) (int, error) {
	return nextReleaseRevisionFor(tx, projectID, applicationID, deploymentTargetID)
}
func ReleaseFromInput(projectID, userID string, input ReleaseInput, releaseID string) model.Release {
	return releaseFromInput(projectID, userID, input, releaseID)
}
func RollbackReleaseFromTarget(source, target model.Release, userID string, revision int) model.Release {
	return rollbackReleaseFromTarget(source, target, userID, revision)
}
func NormalizeReleaseType(value string) string { return normalizeReleaseType(value) }

func DeploymentBundleDigest(bundle DeploymentTargetBundle) (string, error) {
	return deploymentBundleDigest(bundle)
}
func BindDeploymentBundleJSON(ctx *gin.Context, destination *DeploymentTargetBundleImportRequest) bool {
	return bindDeploymentBundleJSON(ctx, destination)
}
func ValidateDeploymentBundleJSON(payload []byte) error { return validateDeploymentBundleJSON(payload) }
func DeploymentBundleConfiguration(target model.DeploymentTarget, mounts []model.DeploymentVolumeMount, buildEnvironment model.BuildEnvironmentConfig) (DeploymentTargetInput, error) {
	return deploymentBundleConfiguration(target, mounts, buildEnvironment)
}
func ValidateDeploymentTargetBundle(bundle DeploymentTargetBundle) error {
	return validateDeploymentTargetBundle(bundle)
}
func ValidateResolvedDeploymentBundle(input DeploymentTargetInput, references []DeploymentBundleReference, resolved map[string]string) error {
	return validateResolvedDeploymentBundle(input, references, resolved)
}
func DeploymentBundleVolumeDestinationCompatible(projectNamespace string, input DeploymentTargetInput, projectVolume model.ProjectVolume) bool {
	return deploymentBundleVolumeDestinationCompatible(projectNamespace, input, projectVolume)
}
func DeploymentBundleErrorCode(err error) string { return deploymentBundleErrorCode(err) }
func DeploymentBundleErrorSpecFor(code string) (DeploymentBundleErrorSpec, bool) {
	return deploymentBundleErrorSpecFor(code)
}
func WriteDeploymentBundleError(ctx *gin.Context, err error) { writeDeploymentBundleError(ctx, err) }
func DeploymentBundleOperationError(err error) error         { return deploymentBundleOperationError(err) }
func DeploymentBundleFilenamePart(value string) string       { return deploymentBundleFilenamePart(value) }
func EncodeStringMap(values map[string]string) string        { return encodeStringMap(values) }
func DeploymentBundleSecretValues(requirements []DeploymentBundleSecretRequirement, values map[string]string, required bool) ([]DeploymentBundleSecretValue, error) {
	return deploymentBundleSecretValues(requirements, values, required)
}
func DeploymentBundleReferenceDescriptorMatches(source, candidate DeploymentBundleReferenceDescriptor) bool {
	return deploymentBundleReferenceDescriptorMatches(source, candidate)
}
func ApplyDeploymentBundleResolution(input *DeploymentTargetInput, reference DeploymentBundleReference, resolvedID string) error {
	return applyDeploymentBundleResolution(input, reference, resolvedID)
}
func UniqueStrings(values []string) []string { return uniqueStrings(values) }
func NormalizeDeploymentBundleCandidateQuery(query DeploymentBundleCandidateQuery) DeploymentBundleCandidateQuery {
	return normalizeDeploymentBundleCandidateQuery(query)
}
func AppendCompatibleDeploymentBundleMatches(matches, candidates []DeploymentBundleCandidate) []DeploymentBundleCandidate {
	return appendCompatibleDeploymentBundleMatches(matches, candidates)
}

func DeploymentTargetMetricsResponseFromSnapshot(snapshot kubeprovider.RuntimeMetricsSnapshot, target model.DeploymentTarget) DeploymentTargetMetricsResponse {
	return deploymentTargetMetricsResponseFromSnapshot(snapshot, target)
}
func DeploymentTargetMetricsStatus(available bool) string {
	return deploymentTargetMetricsStatus(available)
}
func QuantityMilliValue(value string) int64      { return quantityMilliValue(value) }
func QuantityValue(value string) int64           { return quantityValue(value) }
func UsagePercent(usage, capacity int64) float64 { return usagePercent(usage, capacity) }
func DeploymentObservationFromSnapshot(snapshot kubeprovider.DeploymentSnapshot) string {
	return deploymentObservationFromSnapshot(snapshot)
}
func UnavailableDeploymentTarget(target model.DeploymentTarget, code string) model.DeploymentTarget {
	return unavailableDeploymentTarget(target, code)
}

func (h *Handler) CreateDeploymentTargetModel(target model.DeploymentTarget, dataVolumes []DeploymentTargetDataVolumeInput, hookInputs []DeploymentTargetHookBindingInput, buildEnvironment *model.BuildEnvironmentConfig, ctx context.Context) (DeploymentVolumeMountChanges, error) {
	return h.createDeploymentTarget(target, dataVolumes, hookInputs, buildEnvironment, ctx)
}
func (h *Handler) SaveDeploymentTargetModel(target model.DeploymentTarget, dataVolumes []DeploymentTargetDataVolumeInput, hookInputs []DeploymentTargetHookBindingInput, buildEnvironment *model.BuildEnvironmentConfig, ctx context.Context) (DeploymentVolumeMountChanges, error) {
	return h.saveDeploymentTarget(target, dataVolumes, hookInputs, buildEnvironment, ctx)
}
func (h *Handler) AttachDeploymentTargetHookBindings(targets []model.DeploymentTarget, ctx context.Context) error {
	return h.attachDeploymentTargetHookBindings(targets, ctx)
}
func (h *Handler) DeploymentTargetWithHookBindings(target model.DeploymentTarget, ctx context.Context) (model.DeploymentTarget, error) {
	return h.deploymentTargetWithHookBindings(target, ctx)
}
func (h *Handler) DeploymentTargetVolumeMountsByTarget(ctx context.Context, targets []model.DeploymentTarget) (map[string][]model.DeploymentVolumeMount, error) {
	return h.deploymentTargetVolumeMountsByTarget(ctx, targets)
}
func (h *Handler) AuditDeploymentVolumeMountChanges(ctx context.Context, userID string, target model.DeploymentTarget, changes DeploymentVolumeMountChanges) {
	h.auditDeploymentVolumeMountChanges(ctx, userID, target, changes)
}
func (h *Handler) AuditDeploymentVolumeMountFailure(ctx context.Context, userID string, changes DeploymentVolumeMountChanges, err error) {
	h.auditDeploymentVolumeMountFailure(ctx, userID, changes, err)
}
func (h *Handler) ObserveDeploymentTargets(ctx context.Context, project model.Project, targets []model.DeploymentTarget) {
	h.observeDeploymentTargets(ctx, project, targets)
}
func (h *Handler) ObserveDeploymentTarget(ctx context.Context, project model.Project, target model.DeploymentTarget) model.DeploymentTarget {
	return h.observeDeploymentTarget(ctx, project, target)
}
func (h *Handler) KubernetesClientForDeploymentTargetObservation(project model.Project, target model.DeploymentTarget, ctx context.Context) (*kubeprovider.Client, string, string) {
	return h.kubernetesClientForDeploymentTargetObservation(project, target, ctx)
}
func (h *Handler) BuildDeploymentTargetBundle(ctx context.Context, project model.Project, application model.Application, target model.DeploymentTarget) (DeploymentTargetBundle, error) {
	return h.buildDeploymentTargetBundle(ctx, project, application, target)
}
func (h *Handler) BuildDeploymentTargetImportPlan(ctx *gin.Context, user model.User, project model.Project, application model.Application, request DeploymentTargetBundleImportRequest, requireSecrets bool) (DeploymentTargetBundleImportPlan, error) {
	return h.buildDeploymentTargetImportPlan(ctx, user, project, application, request, requireSecrets)
}
func (h *Handler) DeploymentBundleCandidates(ctx context.Context, user model.User, project model.Project, application model.Application, reference DeploymentBundleReference, query DeploymentBundleCandidateQuery) (DeploymentBundleReferenceCandidatePage, []DeploymentBundleCandidate, error) {
	return h.deploymentBundleCandidates(ctx, user, project, application, reference, query)
}
func (h *Handler) ResolveDeploymentBundleReference(ctx *gin.Context, user model.User, project model.Project, application model.Application, reference DeploymentBundleReference, mappedID string) (DeploymentBundleReferenceResolution, []DeploymentBundleCandidate, error) {
	return h.resolveDeploymentBundleReference(ctx, user, project, application, reference, mappedID)
}
func (h *Handler) FindRelease(ctx *gin.Context) (model.Release, bool) { return h.findRelease(ctx) }
func (h *Handler) EnqueueAutoDeploymentsForBuildRun(ctx context.Context, run model.BuildRun) {
	h.enqueueAutoDeploymentsForBuildRun(ctx, run)
}
