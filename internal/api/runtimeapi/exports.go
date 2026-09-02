package runtimeapi

import (
	"context"
	"sync"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	projectservice "github.com/LiteyukiStudio/devops/internal/project"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/resourcepolicy"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	ScopedResourceRuntimeCluster            = scopedResourceRuntimeCluster
	DefaultClusterBuildConcurrency          = defaultClusterBuildConcurrency
	MaxRuntimeClusterPressureBatch          = maxRuntimeClusterPressureBatch
	MaxRuntimeClusterKubeGatewayStatusBatch = maxRuntimeClusterKubeGatewayStatusBatch
	RuntimeEnvironmentValueModePublic       = runtimeEnvironmentValueModePublic
	RuntimeEnvironmentValueModeSecret       = runtimeEnvironmentValueModeSecret
	MaxRuntimeEnvironmentVariables          = maxRuntimeEnvironmentVariables
	MaxRuntimeEnvironmentValueLength        = maxRuntimeEnvironmentValueLength
	RuntimeTerminalResourceCheckTimeout     = runtimeTerminalResourceCheckTimeout
	RuntimeTerminalTicketTTL                = runtimeTerminalTicketTTL
	RuntimeTerminalTicketKeyPrefix          = runtimeTerminalTicketKeyPrefix
	SystemComponentGatewayTrafficProbe      = systemComponentGatewayTrafficProbe
)

var (
	ErrKubeGatewayEnqueue               = errKubeGatewayEnqueue
	ErrRuntimeSecretMutationUnavailable = errRuntimeSecretMutationUnavailable
	RuntimeResourceCategories           = append([]string(nil), runtimeResourceCategories...)
	RuntimeResourceKinds                = append([]string(nil), runtimeResourceKinds...)
)

type RuntimeClusterInput = runtimeClusterInput
type RuntimeClusterKubeGatewayRule = runtimeClusterKubeGatewayRule
type RuntimeClusterKubeGatewayInput = runtimeClusterKubeGatewayInput
type RuntimeClusterKubeGatewayResponse = runtimeClusterKubeGatewayResponse
type RuntimeClusterKubeGatewayStatusResponse = runtimeClusterKubeGatewayStatusResponse
type RuntimeClusterKubeGatewayStatusListResponse = runtimeClusterKubeGatewayStatusListResponse
type RuntimeClusterPressureResource = runtimeClusterPressureResource
type RuntimeClusterPressureDetails = runtimeClusterPressureDetails
type RuntimeClusterPressureResponse = runtimeClusterPressureResponse
type RuntimeClusterPressureListResponse = runtimeClusterPressureListResponse
type ClusterResourceYAMLResponse = clusterResourceYAMLResponse
type ClusterResourceResponse = clusterResourceResponse
type RuntimeConfigFileInput = runtimeConfigFileInput
type ProjectRuntimeConfigSetInput = projectRuntimeConfigSetInput
type ProjectRuntimeConfigSetResponse = projectRuntimeConfigSetResponse
type RuntimeEnvironmentVariableInput = runtimeEnvironmentVariableInput
type RuntimeEnvironmentVariableResponse = runtimeEnvironmentVariableResponse
type RuntimeSecretGeneration = runtimeSecretGeneration
type RuntimeSecretMutationRequestItem = runtimeSecretMutationRequestItem
type RuntimeSecretMutationRequest = runtimeSecretMutationRequest
type RuntimeSecretMutationInput = runtimeSecretMutationInput
type RuntimeSecretMutationResponse = runtimeSecretMutationResponse
type RuntimeSecretMutationOwner = runtimeSecretMutationOwner
type PreparedRuntimeSecretMutation struct {
	Values         map[string]string
	ConfiguredKeys []string
	GeneratedKeys  []string
	ClearKeys      []string
}
type RuntimeTerminalAuthorizationBinding = runtimeTerminalAuthorizationBinding
type RuntimeTerminalTicketResponse = runtimeTerminalTicketResponse
type RuntimeTerminalTicketValue = runtimeTerminalTicketValue
type ReleaseRuntimeTerminalAuthorizationReference = releaseRuntimeTerminalAuthorizationReference
type RuntimeClusterPodTerminalAuthorizationReference = runtimeClusterPodTerminalAuthorizationReference
type SystemComponentInstallInput = systemComponentInstallInput
type SystemComponentInstallResponse = systemComponentInstallResponse
type SystemComponentStatusResponse = systemComponentStatusResponse
type SystemComponentApplicationPlan = systemComponentApplicationPlan
type RuntimeClusterAuditMetadata = runtimeClusterAuditMetadata

func RuntimeTerminalMemoryTicketStore() *sync.Map { return &runtimeTerminalMemoryTickets }

func NormalizedKubeCredentialDays(value int) int { return normalizedKubeCredentialDays(value) }

func CanUseRuntimeClusterForProject(user model.User, cluster model.RuntimeCluster, projectID string, boundProjectIDs []string) bool {
	return canUseRuntimeClusterForProject(user, cluster, projectID, boundProjectIDs)
}

func (h *Handler) FilterClusterResourceSnapshots(ctx *gin.Context, user model.User, items []kubeprovider.ResourceSnapshot, visibility projectservice.ListVisibility, projectID string) []kubeprovider.ResourceSnapshot {
	return h.filterClusterResourceSnapshots(ctx, user, items, visibility, projectID)
}

func DeploymentTargetNamespace(project model.Project, target model.DeploymentTarget) string {
	return deploymentTargetNamespace(project, target)
}
func RuntimeProjectNamespace(project model.Project) string { return runtimeProjectNamespace(project) }

func RuntimeClusterForDeploymentTargetDB(db *gorm.DB, target model.DeploymentTarget) (model.RuntimeCluster, error) {
	return runtimeClusterForDeploymentTargetDB(db, target)
}

func RuntimeClusterResourcePolicy(input RuntimeClusterInput) resourcepolicy.Policy {
	return runtimeClusterResourcePolicy(input)
}

func FlattenKubeconfig(kubeconfig string) (string, error) { return flattenKubeconfig(kubeconfig) }
func NormalizeRuntimeClusterType(value string) string     { return normalizeRuntimeClusterType(value) }
func NormalizeGatewayRootDomain(value, fallbackValue string) string {
	return normalizeGatewayRootDomain(value, fallbackValue)
}
func NormalizeGatewayDomainSuffixes(values []string, legacyValue, fallbackValue string) []string {
	return normalizeGatewayDomainSuffixes(values, legacyValue, fallbackValue)
}
func NormalizeGatewayDomainSuffixList(values []string) []string {
	return normalizeGatewayDomainSuffixList(values)
}
func NormalizeGatewayDomainSuffixValue(value string) string {
	return normalizeGatewayDomainSuffixValue(value)
}
func EncodeGatewayDomainSuffixes(values []string) string { return encodeGatewayDomainSuffixes(values) }
func DecodeGatewayDomainSuffixes(raw, legacyValue, fallbackValue string) []string {
	return decodeGatewayDomainSuffixes(raw, legacyValue, fallbackValue)
}
func NormalizeGatewayPublicScheme(value string) string { return normalizeGatewayPublicScheme(value) }
func NormalizePort(value, fallbackValue int) int       { return normalizePort(value, fallbackValue) }
func NormalizeGatewayPublicPort(value int, scheme string) int {
	return normalizeGatewayPublicPort(value, scheme)
}
func NormalizeGatewayProvider(value string) string { return normalizeGatewayProvider(value) }
func NormalizeGatewayControllerType(value string) string {
	return normalizeGatewayControllerType(value)
}
func NormalizeGatewayExternalTLSMode(value string) string {
	return normalizeGatewayExternalTLSMode(value)
}
func NormalizeGatewayCertIssuerKind(value string) string {
	return normalizeGatewayCertIssuerKind(value)
}
func DNSLabelName(value string) string { return dnsLabelName(value) }
func NormalizeGatewayForwardedHeadersMode(value string) string {
	return normalizeGatewayForwardedHeadersMode(value)
}
func ValidateTrustedProxyCIDRs(value string) error { return validateTrustedProxyCIDRs(value) }

func NormalizeRuntimeClusterKubeGatewayRules(input []RuntimeClusterKubeGatewayRule) []RuntimeClusterKubeGatewayRule {
	return normalizeRuntimeClusterKubeGatewayRules(input)
}
func DecodeRuntimeClusterKubeGatewayRules(raw string) ([]RuntimeClusterKubeGatewayRule, error) {
	return decodeRuntimeClusterKubeGatewayRules(raw)
}

func RuntimeClusterPressureFromSnapshot(clusterID string, snapshot kubeprovider.ClusterPressureSnapshot, detailed bool) RuntimeClusterPressureResponse {
	return runtimeClusterPressureFromSnapshot(clusterID, snapshot, detailed)
}
func RuntimeClusterPressureScore(cpuRequest, memoryRequest float64, cpuUsage, memoryUsage *float64) float64 {
	return runtimeClusterPressureScore(cpuRequest, memoryRequest, cpuUsage, memoryUsage)
}
func RuntimeClusterPressureLevel(score float64) string { return runtimeClusterPressureLevel(score) }
func UnavailableRuntimeClusterPressure(clusterID, code string) RuntimeClusterPressureResponse {
	return unavailableRuntimeClusterPressure(clusterID, code)
}
func RuntimeClusterPressureIDs(raw []string) ([]string, bool) { return runtimeClusterPressureIDs(raw) }

func ValidRuntimeResourceCategory(value string) bool { return validRuntimeResourceCategory(value) }
func ValidRuntimeResourceKind(value string) bool     { return validRuntimeResourceKind(value) }
func WriteRuntimeResourceArgumentError(ctx *gin.Context, code, path string, allowedValues []string) {
	writeRuntimeResourceArgumentError(ctx, code, path, allowedValues)
}
func GroupWorkloadPodResponses(items []ClusterResourceResponse) []ClusterResourceResponse {
	return groupWorkloadPodResponses(items)
}

func NormalizeRuntimeConfigFilesInput(ctx *gin.Context, value string) (string, bool) {
	return normalizeRuntimeConfigFilesInput(ctx, value)
}
func NormalizeRuntimeConfigFilePathInput(ctx *gin.Context, value string) (string, bool) {
	return normalizeRuntimeConfigFilePathInput(ctx, value)
}
func ParseRuntimeFileContentInput(ctx *gin.Context, value, errorMessage string) (map[string]string, bool) {
	return parseRuntimeFileContentInput(ctx, value, errorMessage)
}
func ProjectRuntimeConfigSetResponses(sets []model.ProjectRuntimeConfigSet) []ProjectRuntimeConfigSetResponse {
	return projectRuntimeConfigSetResponses(sets)
}
func ProjectRuntimeConfigSetResponseFor(set model.ProjectRuntimeConfigSet) ProjectRuntimeConfigSetResponse {
	return projectRuntimeConfigSetResponseFor(set)
}
func ProjectRuntimeConfigSetSecretMutationOwner(setID, projectID string) RuntimeSecretMutationOwner {
	return projectRuntimeConfigSetSecretMutationOwner(setID, projectID)
}

func NormalizePublicEnvironmentVariables(ctx *gin.Context, items []RuntimeEnvironmentVariableInput) (map[string]string, bool) {
	return normalizePublicEnvironmentVariables(ctx, items)
}
func RuntimeEnvironmentVariables(publicRaw, secretRaw string) []RuntimeEnvironmentVariableResponse {
	return runtimeEnvironmentVariables(publicRaw, secretRaw)
}
func PublicEnvironmentVariableInputs(raw string) []RuntimeEnvironmentVariableInput {
	return publicEnvironmentVariableInputs(raw)
}
func RuntimeSecretKeys(raw string) []string           { return runtimeSecretKeys(raw) }
func SetRuntimeSecretNoStoreHeaders(ctx *gin.Context) { setRuntimeSecretNoStoreHeaders(ctx) }

func PrepareRuntimeSecretMutation(input RuntimeSecretMutationInput) (PreparedRuntimeSecretMutation, error) {
	prepared, err := prepareRuntimeSecretMutation(input)
	if err != nil {
		return PreparedRuntimeSecretMutation{}, err
	}
	return PreparedRuntimeSecretMutation{
		Values:         prepared.values,
		ConfiguredKeys: prepared.configuredKey,
		GeneratedKeys:  prepared.generatedKey,
		ClearKeys:      prepared.clearKey,
	}, nil
}
func RuntimeSecretMutationInputFromRequest(ctx *gin.Context, request RuntimeSecretMutationRequest) (RuntimeSecretMutationInput, bool) {
	return runtimeSecretMutationInputFromRequest(ctx, request)
}
func SecretEnvironmentVariables(keys []string) []RuntimeEnvironmentVariableResponse {
	return secretEnvironmentVariables(keys)
}
func DecodeRuntimeSecretRefs(raw string) (map[string]string, error) {
	return decodeRuntimeSecretRefs(raw)
}
func ValidateRuntimeSecretMutation(ctx *gin.Context, input *RuntimeSecretMutationInput) bool {
	return validateRuntimeSecretMutation(ctx, input)
}
func WriteRuntimeSecretMutationError(ctx *gin.Context, ownerType string, err error) {
	writeRuntimeSecretMutationError(ctx, ownerType, err)
}

func RuntimeExecAuditMessage(command string, result kubeprovider.RuntimeExecResult) string {
	return runtimeExecAuditMessage(command, result)
}
func RequireRuntimeTerminalTicketForBearer(ctx *gin.Context, ticket string) bool {
	return requireRuntimeTerminalTicketForBearer(ctx, ticket)
}
func OAuthAccessTokenSubject(tokenID string) string { return oauthAccessTokenSubject(tokenID) }
func RuntimeClusterPodTerminalReference(cluster model.RuntimeCluster, snapshot kubeprovider.ResourceSnapshot) RuntimeClusterPodTerminalAuthorizationReference {
	return runtimeClusterPodTerminalReference(cluster, snapshot)
}
func SameRuntimeClusterPodTerminalResource(reference RuntimeClusterPodTerminalAuthorizationReference, snapshot kubeprovider.ResourceSnapshot) bool {
	return sameRuntimeClusterPodTerminalResource(reference, snapshot)
}
func RuntimeTerminalResourceFingerprint(resourceKind string, resource any) string {
	return runtimeTerminalResourceFingerprint(resourceKind, resource)
}

func HasReadySystemComponent(items []model.SystemComponentInstallation, componentID string) bool {
	return hasReadySystemComponent(items, componentID)
}
func SystemComponentProbeEnv(cluster model.RuntimeCluster, componentID, mode, apiBaseURL, traefikMetricsURL string) string {
	return systemComponentProbeEnv(cluster, componentID, mode, apiBaseURL, traefikMetricsURL)
}
func ValidateOptionalHTTPURL(value, label string) error { return validateOptionalHTTPURL(value, label) }
func UnavailableSystemComponent(item model.SystemComponentInstallation, code string) model.SystemComponentInstallation {
	return unavailableSystemComponent(item, code)
}

func RuntimeClusterSafeAuditMetadata(cluster model.RuntimeCluster, kubeconfigUpdated bool) RuntimeClusterAuditMetadata {
	return runtimeClusterSafeAuditMetadata(cluster, kubeconfigUpdated)
}
func RuntimeClusterPersistenceAuditMessage(err error) string {
	return runtimeClusterPersistenceAuditMessage(err)
}

func (h *Handler) RuntimeClusterForEnvironment(ctx *gin.Context, environment model.Environment) (model.RuntimeCluster, bool) {
	return h.runtimeClusterForEnvironment(ctx, environment)
}
func (h *Handler) RuntimeClusterForDeploymentTarget(ctx *gin.Context, target model.DeploymentTarget) (model.RuntimeCluster, bool) {
	return h.runtimeClusterForDeploymentTarget(ctx, target)
}
func (h *Handler) RuntimeClusterForProjectUse(ctx *gin.Context, user model.User, projectID, clusterID string) (model.RuntimeCluster, bool) {
	return h.runtimeClusterForProjectUse(ctx, user, projectID, clusterID)
}
func (h *Handler) RuntimeClusterResponseForUser(user model.User, cluster model.RuntimeCluster, ctx context.Context) (model.RuntimeCluster, error) {
	return h.runtimeClusterResponseForUser(user, cluster, ctx)
}
func (h *Handler) DefaultRuntimeClusterID(ctx context.Context) string {
	return h.defaultRuntimeClusterID(ctx)
}
func (h *Handler) ObserveRuntimeClusters(ctx context.Context, clusters []model.RuntimeCluster) {
	h.observeRuntimeClusters(ctx, clusters)
}
func (h *Handler) AllowRuntimeClusterConnectionChange(ctx *gin.Context, existing model.RuntimeCluster, input RuntimeClusterInput) bool {
	return h.allowRuntimeClusterConnectionChange(ctx, existing, input)
}
func (h *Handler) PersistRuntimeClusterKubeGatewayDesired(ctx context.Context, clusterID string, enabled bool, encodedRules string) error {
	return h.persistRuntimeClusterKubeGatewayDesired(ctx, clusterID, enabled, encodedRules)
}
func (h *Handler) EnqueueEnabledProjectAccessKubeGateways(ctx context.Context, userID string, includeGlobal bool) error {
	return h.enqueueEnabledProjectAccessKubeGateways(ctx, userID, includeGlobal)
}
func (h *Handler) RuntimeClusterKubeGatewayManagerAndSpec(ctx context.Context, cluster model.RuntimeCluster) (*kubeprovider.KubectlGatewayManager, kubeprovider.GatewayAccessSpec, error) {
	return h.runtimeClusterKubeGatewayManagerAndSpec(ctx, cluster)
}
func (h *Handler) RequireKubeGatewayReady(ctx context.Context, cluster model.RuntimeCluster, project model.Project) error {
	return (apiKubeGatewayReadiness{handlers: h}).RequireReady(ctx, cluster, project)
}
func (h *Handler) RuntimeSecretFilesFromInput(ctx *gin.Context, user model.User, ownerID, value string, existing map[string]string) (map[string]string, bool) {
	return h.runtimeSecretFilesFromInput(ctx, user, ownerID, value, existing)
}
func (h *Handler) MutateRuntimeSecrets(ctx context.Context, user model.User, prepared PreparedRuntimeSecretMutation, owner RuntimeSecretMutationOwner) (RuntimeSecretMutationResponse, error) {
	return h.mutateRuntimeSecrets(ctx, user, preparedRuntimeSecretMutation{
		values:        prepared.Values,
		configuredKey: prepared.ConfiguredKeys,
		generatedKey:  prepared.GeneratedKeys,
		clearKey:      prepared.ClearKeys,
	}, owner)
}
func (h *Handler) RequireRuntimeTerminalAuthorization(ctx *gin.Context, user model.User) (RuntimeTerminalAuthorizationBinding, bool) {
	return h.requireRuntimeTerminalAuthorization(ctx, user)
}
func (h *Handler) CurrentInteractiveSubject(ctx *gin.Context, user model.User) (string, bool) {
	return h.currentInteractiveSubject(ctx, user)
}
func (h *Handler) CurrentInteractiveAuthorizationBinding(ctx *gin.Context, user model.User) (RuntimeTerminalAuthorizationBinding, bool) {
	return h.currentInteractiveAuthorizationBinding(ctx, user)
}
func (h *Handler) ReleaseRuntimeTerminalAuthorizationAllowed(ctx context.Context, user model.User, reference ReleaseRuntimeTerminalAuthorizationReference) bool {
	return h.releaseRuntimeTerminalAuthorizationAllowed(ctx, user, reference)
}
func (h *Handler) RuntimeClusterPodTerminalAuthorizationAllowed(ctx context.Context, user model.User, client *kubeprovider.Client, reference RuntimeClusterPodTerminalAuthorizationReference) bool {
	return h.runtimeClusterPodTerminalAuthorizationAllowed(ctx, user, client, reference)
}
func (h *Handler) IssueRuntimeTerminalTicket(ctx context.Context, authorization RuntimeTerminalAuthorizationBinding, resourceKind string, resource any) (string, time.Time, error) {
	return h.issueRuntimeTerminalTicket(ctx, authorization, resourceKind, resource)
}
func (h *Handler) ConsumeRuntimeTerminalTicket(ctx context.Context, ticket string) (RuntimeTerminalTicketValue, bool, error) {
	return h.consumeRuntimeTerminalTicket(ctx, ticket)
}
func (value runtimeTerminalTicketValue) Matches(resourceKind string, resource any) bool {
	return value.matches(resourceKind, resource)
}
func (h *Handler) MonitorContinuousAuthorization(ctx context.Context, binding RuntimeTerminalAuthorizationBinding, authorizationAllowed func(context.Context, model.User) bool, revoke func()) (<-chan struct{}, bool) {
	return h.monitorContinuousAuthorization(ctx, binding, authorizationAllowed, revoke)
}
func (h *Handler) ContinuousAuthorizationActive(ctx context.Context, binding RuntimeTerminalAuthorizationBinding, authorizationAllowed func(context.Context, model.User) bool) bool {
	return h.continuousAuthorizationActive(ctx, binding, authorizationAllowed)
}
func (h *Handler) SystemComponentForBearerToken(token, componentID string, ctx context.Context) (model.SystemComponentInstallation, bool) {
	return h.systemComponentForBearerToken(token, componentID, ctx)
}
func (h *Handler) ObserveSystemComponentInstallations(ctx context.Context, items []model.SystemComponentInstallation) {
	h.observeSystemComponentInstallations(ctx, items)
}
