package api

import (
	"context"
	"errors"
	"time"

	"github.com/LiteyukiStudio/devops/internal/api/deploymentapi"
	"github.com/LiteyukiStudio/devops/internal/api/runtimeapi"
	"github.com/LiteyukiStudio/devops/internal/appstore"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/kubeaccess"
	"github.com/LiteyukiStudio/devops/internal/model"
	projectservice "github.com/LiteyukiStudio/devops/internal/project"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/resourcepolicy"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type runtimeHost struct {
	handlers *Handlers
}

func (host runtimeHost) DBFor(ctx *gin.Context) *gorm.DB { return host.handlers.dbFor(ctx) }
func (host runtimeHost) DBWithContext(ctx context.Context) *gorm.DB {
	return host.handlers.dbWithContext(ctx)
}
func (host runtimeHost) CurrentUser(ctx *gin.Context) (model.User, bool) {
	return host.handlers.currentUser(ctx)
}
func (host runtimeHost) RequirePlatformAdmin(ctx *gin.Context) bool {
	return host.handlers.requirePlatformAdmin(ctx)
}
func (host runtimeHost) AuthorizeProject(ctx *gin.Context, action authz.Action) (model.User, model.Project, bool) {
	return host.handlers.authorizeProject(ctx, action)
}
func (host runtimeHost) AuthorizeProjectByID(ctx *gin.Context, projectID string, action authz.Action) (model.User, model.Project, bool) {
	return host.handlers.authorizeProjectByID(ctx, projectID, action)
}
func (host runtimeHost) EnsurePlatformSystemProject(user model.User, ctx context.Context) (model.Project, error) {
	return host.handlers.ensurePlatformSystemProject(user, ctx)
}
func (host runtimeHost) EnsureProjectCanMutate(ctx *gin.Context, project model.Project) bool {
	return host.handlers.ensureProjectCanMutate(ctx, project)
}
func (host runtimeHost) EnsureRuntimeConfigSetCanMutate(ctx *gin.Context, set model.ProjectRuntimeConfigSet) bool {
	return host.handlers.ensureRuntimeConfigSetCanMutate(ctx, set)
}
func (runtimeHost) DeleteStatusCanStart(status string) bool { return deleteStatusCanStart(status) }
func (runtimeHost) ResourceDeleteAlreadyStarted(err error) bool {
	return errors.Is(err, errResourceDeleteAlreadyStarted)
}
func (runtimeHost) MarkResourceDeleting(tx *gorm.DB, resource any, resourceID string) error {
	return markResourceDeleting(tx, resource, resourceID)
}
func (runtimeHost) MarkResourceDeleteFailed(db *gorm.DB, resource any, resourceID, message string) error {
	return markResourceDeleteFailed(db, resource, resourceID, message)
}
func (host runtimeHost) FindProjectForCurrentUserByID(ctx *gin.Context, projectID string) (model.Project, bool) {
	return host.handlers.findProjectForCurrentUserByID(ctx, projectID)
}
func (host runtimeHost) ProjectAuthorizer(ctx context.Context) authz.ProjectAuthorizer {
	return host.handlers.projectAuthorizer(ctx)
}
func (host runtimeHost) ApplyScopedResourceVisibility(ctx *gin.Context, query *gorm.DB, resourceType string, user model.User, projectID string) (*gorm.DB, bool) {
	return host.handlers.applyScopedResourceVisibility(ctx, query, resourceType, user, projectID)
}
func (host runtimeHost) ApplyScopedResourceListVisibility(ctx *gin.Context, query *gorm.DB, resourceType string, user model.User, projectID string, visibility projectservice.ListVisibility) (*gorm.DB, bool) {
	return host.handlers.applyScopedResourceListVisibility(ctx, query, resourceType, user, projectID, visibility)
}
func (host runtimeHost) NormalizeScopedOwnerWithProjects(ctx *gin.Context, user model.User, scope, ownerRef string, projectIDs []string, globalError string) (string, string, []string, bool) {
	return host.handlers.normalizeScopedOwnerWithProjects(ctx, user, scope, ownerRef, projectIDs, globalError)
}
func (host runtimeHost) ReplaceScopedResourceProjectBindings(tx *gorm.DB, resourceType, resourceID string, projectIDs, defaultProjectIDs []string) error {
	return host.handlers.replaceScopedResourceProjectBindings(tx, resourceType, resourceID, projectIDs, defaultProjectIDs)
}
func (host runtimeHost) ScopedResourceProjectIDs(resourceType, resourceID string, ctx context.Context) []string {
	return host.handlers.scopedResourceProjectIDs(resourceType, resourceID, ctx)
}
func (host runtimeHost) ProjectIDsForUser(ctx context.Context, userID string) []string {
	return host.handlers.projectIDsForUser(ctx, userID)
}
func (host runtimeHost) CanInspectScopedResourceConfigByID(user model.User, scope, ownerRef, resourceType, resourceID string, ctx context.Context) (bool, error) {
	return host.handlers.canInspectScopedResourceConfigByID(user, scope, ownerRef, resourceType, resourceID, ctx)
}
func (host runtimeHost) CanManageScopedResourceByID(ctx *gin.Context, user model.User, scope, ownerRef, resourceType, resourceID, errorMessage string) bool {
	return host.handlers.canManageScopedResourceByID(ctx, user, scope, ownerRef, resourceType, resourceID, errorMessage)
}
func (host runtimeHost) CanUseScopedResourceByID(user model.User, scope, ownerRef, resourceType, resourceID string, ctx context.Context) bool {
	return host.handlers.canUseScopedResourceByID(user, scope, ownerRef, resourceType, resourceID, ctx)
}
func (host runtimeHost) CurrentSessionFromCookie(ctx *gin.Context) (model.UserSession, bool) {
	return host.handlers.currentSessionFromCookie(ctx)
}
func (host runtimeHost) AuditWithContext(userID, action, resource string, success bool, message string, ctx context.Context) {
	host.handlers.auditWithContext(userID, action, resource, success, message, ctx)
}
func (host runtimeHost) AuditWithSafeMetadata(userID, action, resource string, success bool, message string, metadata any, ctx context.Context) {
	switch value := metadata.(type) {
	case kubeCredentialAuditMetadata:
		auditWithSafeMetadata(host.handlers, userID, action, resource, success, message, value, ctx)
	case kubeGatewayAuditMetadata:
		auditWithSafeMetadata(host.handlers, userID, action, resource, success, message, value, ctx)
	case runtimeClusterAuditMetadata:
		auditWithSafeMetadata(host.handlers, userID, action, resource, success, message, value, ctx)
	}
}
func (host runtimeHost) EnqueueDeployRun(ctx context.Context, release model.Release) bool {
	return host.handlers.enqueueDeployRun(ctx, release)
}
func (host runtimeHost) EnqueueResourceCleanup(ctx context.Context, resourceType, resourceID, projectID, actorID string) bool {
	return host.handlers.enqueueResourceCleanup(ctx, resourceType, resourceID, projectID, actorID)
}

type apiKubectlGatewayTaskEnqueuer interface {
	EnqueueKubectlGateway(context.Context, tasks.KubectlGatewayPayload) (*asynq.TaskInfo, error)
}

func (host runtimeHost) EnqueueKubectlGateway(ctx context.Context, clusterID string) error {
	taskClient, ok := host.handlers.taskClient.(apiKubectlGatewayTaskEnqueuer)
	if !ok || taskClient == nil {
		return runtimeapi.ErrKubeGatewayEnqueue
	}
	if _, err := taskClient.EnqueueKubectlGateway(ctx, tasks.KubectlGatewayPayload{ClusterID: clusterID}); err != nil {
		return errors.Join(runtimeapi.ErrKubeGatewayEnqueue, err)
	}
	return nil
}
func (host runtimeHost) ObserveDeploymentTarget(ctx context.Context, project model.Project, target model.DeploymentTarget) model.DeploymentTarget {
	return host.handlers.observeDeploymentTarget(ctx, project, target)
}
func (host runtimeHost) MonitorContinuousAuthorization(ctx context.Context, binding runtimeTerminalAuthorizationBinding, authorizationAllowed func(context.Context, model.User) bool, revoke func()) (<-chan struct{}, bool) {
	return host.handlers.monitorContinuousAuthorization(ctx, binding, authorizationAllowed, revoke)
}
func (host runtimeHost) ContinuousAuthorizationActive(ctx context.Context, binding runtimeTerminalAuthorizationBinding, authorizationAllowed func(context.Context, model.User) bool) bool {
	return host.handlers.continuousAuthorizationActive(ctx, binding, authorizationAllowed)
}
func (host runtimeHost) SecretStore() secret.Store { return host.handlers.secrets }
func (host runtimeHost) KubeAccessService() *kubeaccess.Service {
	return host.handlers.kubeAccess
}
func (host runtimeHost) RuntimeTerminalRedis() redis.UniversalClient {
	if host.handlers.rateLimiter == nil {
		return nil
	}
	return host.handlers.rateLimiter.redis
}
func (host runtimeHost) Mode() string { return host.handlers.mode }
func (host runtimeHost) AllowedOrigin(origin string) bool {
	return containsString(host.handlers.config.AllowedOrigins, origin)
}
func (host runtimeHost) TaskQueueAvailable() bool { return host.handlers.taskClient != nil }
func (runtimeHost) TemplateApplicationIcon(template appstore.Template) string {
	return templateApplicationIcon(template)
}
func (runtimeHost) ShortID(value string) string { return shortID(value) }
func (runtimeHost) NextReleaseRevision(tx *gorm.DB, projectID, applicationID, deploymentTargetID string) (int, error) {
	return nextReleaseRevisionFor(tx, projectID, applicationID, deploymentTargetID)
}
func (runtimeHost) DeploymentTargetResponse(target model.DeploymentTarget) any {
	return deploymentTargetResponseFromModel(target)
}
func (host runtimeHost) LegacyGatewayRootDomain() string {
	return host.handlers.legacyGatewayRootDomain()
}

func (h *Handlers) runtimeAPI() *runtimeapi.Handler { return runtimeapi.New(runtimeHost{handlers: h}) }

func (h *Handlers) CreateKubeCredential(ctx *gin.Context) { h.runtimeAPI().CreateKubeCredential(ctx) }
func (h *Handlers) ListKubeCredentials(ctx *gin.Context)  { h.runtimeAPI().ListKubeCredentials(ctx) }
func (h *Handlers) ListKubeCredentialBindings(ctx *gin.Context) {
	h.runtimeAPI().ListKubeCredentialBindings(ctx)
}
func (h *Handlers) RevokeKubeCredential(ctx *gin.Context) { h.runtimeAPI().RevokeKubeCredential(ctx) }
func (h *Handlers) ListRuntimeClusters(ctx *gin.Context)  { h.runtimeAPI().ListRuntimeClusters(ctx) }
func (h *Handlers) CreateRuntimeCluster(ctx *gin.Context) { h.runtimeAPI().CreateRuntimeCluster(ctx) }
func (h *Handlers) UpdateRuntimeCluster(ctx *gin.Context) { h.runtimeAPI().UpdateRuntimeCluster(ctx) }
func (h *Handlers) DeleteRuntimeCluster(ctx *gin.Context) { h.runtimeAPI().DeleteRuntimeCluster(ctx) }
func (h *Handlers) TestRuntimeCluster(ctx *gin.Context)   { h.runtimeAPI().TestRuntimeCluster(ctx) }
func (h *Handlers) DeleteRuntimeClusterResource(ctx *gin.Context) {
	h.runtimeAPI().DeleteRuntimeClusterResource(ctx)
}
func (h *Handlers) GetRuntimeClusterKubeGateway(ctx *gin.Context) {
	h.runtimeAPI().GetRuntimeClusterKubeGateway(ctx)
}
func (h *Handlers) UpdateRuntimeClusterKubeGateway(ctx *gin.Context) {
	h.runtimeAPI().UpdateRuntimeClusterKubeGateway(ctx)
}
func (h *Handlers) ObserveRuntimeClusterKubeGatewayStatus(ctx *gin.Context) {
	h.runtimeAPI().ObserveRuntimeClusterKubeGatewayStatus(ctx)
}
func (h *Handlers) ObserveRuntimeClusterPressure(ctx *gin.Context) {
	h.runtimeAPI().ObserveRuntimeClusterPressure(ctx)
}
func (h *Handlers) ListRuntimeClusterResources(ctx *gin.Context) {
	h.runtimeAPI().ListRuntimeClusterResources(ctx)
}
func (h *Handlers) GetRuntimeClusterResourceYAML(ctx *gin.Context) {
	h.runtimeAPI().GetRuntimeClusterResourceYAML(ctx)
}
func (h *Handlers) ListRuntimeClusterResourceEvents(ctx *gin.Context) {
	h.runtimeAPI().ListRuntimeClusterResourceEvents(ctx)
}
func (h *Handlers) StreamRuntimeClusterPodTerminal(ctx *gin.Context) {
	h.runtimeAPI().StreamRuntimeClusterPodTerminal(ctx)
}
func (h *Handlers) AuthorizeRuntimeClusterPodTerminal(ctx *gin.Context) {
	h.runtimeAPI().AuthorizeRuntimeClusterPodTerminal(ctx)
}
func (h *Handlers) ListProjectRuntimeConfigSets(ctx *gin.Context) {
	h.runtimeAPI().ListProjectRuntimeConfigSets(ctx)
}
func (h *Handlers) CreateProjectRuntimeConfigSet(ctx *gin.Context) {
	h.runtimeAPI().CreateProjectRuntimeConfigSet(ctx)
}
func (h *Handlers) UpdateProjectRuntimeConfigSet(ctx *gin.Context) {
	h.runtimeAPI().UpdateProjectRuntimeConfigSet(ctx)
}
func (h *Handlers) DeleteProjectRuntimeConfigSet(ctx *gin.Context) {
	h.runtimeAPI().DeleteProjectRuntimeConfigSet(ctx)
}
func (h *Handlers) UpdateProjectRuntimeConfigSetRuntimeSecrets(ctx *gin.Context) {
	h.runtimeAPI().UpdateProjectRuntimeConfigSetRuntimeSecrets(ctx)
}
func (h *Handlers) ListSystemComponents(ctx *gin.Context) { h.runtimeAPI().ListSystemComponents(ctx) }
func (h *Handlers) InstallSystemAppTemplate(ctx *gin.Context) {
	h.runtimeAPI().InstallSystemAppTemplate(ctx)
}

type runtimeClusterInput = runtimeapi.RuntimeClusterInput
type runtimeClusterKubeGatewayRule = runtimeapi.RuntimeClusterKubeGatewayRule
type runtimeClusterKubeGatewayInput = runtimeapi.RuntimeClusterKubeGatewayInput
type runtimeClusterKubeGatewayResponse = runtimeapi.RuntimeClusterKubeGatewayResponse
type runtimeClusterKubeGatewayStatusResponse = runtimeapi.RuntimeClusterKubeGatewayStatusResponse
type runtimeClusterKubeGatewayStatusListResponse = runtimeapi.RuntimeClusterKubeGatewayStatusListResponse
type runtimeClusterPressureResource = runtimeapi.RuntimeClusterPressureResource
type runtimeClusterPressureDetails = runtimeapi.RuntimeClusterPressureDetails
type runtimeClusterPressureResponse = runtimeapi.RuntimeClusterPressureResponse
type runtimeClusterPressureListResponse = runtimeapi.RuntimeClusterPressureListResponse
type clusterResourceYAMLResponse = runtimeapi.ClusterResourceYAMLResponse
type clusterResourceResponse = runtimeapi.ClusterResourceResponse
type runtimeConfigFileInput = runtimeapi.RuntimeConfigFileInput
type projectRuntimeConfigSetInput = runtimeapi.ProjectRuntimeConfigSetInput
type projectRuntimeConfigSetResponse = runtimeapi.ProjectRuntimeConfigSetResponse
type runtimeEnvironmentVariableInput = runtimeapi.RuntimeEnvironmentVariableInput
type runtimeEnvironmentVariableResponse = runtimeapi.RuntimeEnvironmentVariableResponse
type runtimeSecretGeneration = runtimeapi.RuntimeSecretGeneration
type runtimeSecretMutationRequestItem = runtimeapi.RuntimeSecretMutationRequestItem
type runtimeSecretMutationRequest = runtimeapi.RuntimeSecretMutationRequest
type runtimeSecretMutationInput = runtimeapi.RuntimeSecretMutationInput
type runtimeSecretMutationResponse = runtimeapi.RuntimeSecretMutationResponse
type runtimeSecretMutationOwner = runtimeapi.RuntimeSecretMutationOwner
type preparedRuntimeSecretMutation struct {
	values        map[string]string
	configuredKey []string
	generatedKey  []string
	clearKey      []string
}
type runtimeTerminalTicketResponse = runtimeapi.RuntimeTerminalTicketResponse
type releaseRuntimeTerminalAuthorizationReference = runtimeapi.ReleaseRuntimeTerminalAuthorizationReference
type runtimeClusterPodTerminalAuthorizationReference = runtimeapi.RuntimeClusterPodTerminalAuthorizationReference
type systemComponentInstallInput = runtimeapi.SystemComponentInstallInput
type systemComponentInstallResponse = runtimeapi.SystemComponentInstallResponse
type systemComponentStatusResponse = runtimeapi.SystemComponentStatusResponse

type runtimeTerminalTicketValue runtimeapi.RuntimeTerminalTicketValue

func (value runtimeTerminalTicketValue) matches(resourceKind string, resource any) bool {
	return runtimeapi.RuntimeTerminalTicketValue(value).Matches(resourceKind, resource)
}

const (
	runtimeEnvironmentValueModePublic   = runtimeapi.RuntimeEnvironmentValueModePublic
	runtimeEnvironmentValueModeSecret   = runtimeapi.RuntimeEnvironmentValueModeSecret
	maxRuntimeEnvironmentVariables      = runtimeapi.MaxRuntimeEnvironmentVariables
	maxRuntimeEnvironmentValueLength    = runtimeapi.MaxRuntimeEnvironmentValueLength
	maxRuntimeClusterPressureBatch      = runtimeapi.MaxRuntimeClusterPressureBatch
	runtimeTerminalResourceCheckTimeout = runtimeapi.RuntimeTerminalResourceCheckTimeout
	runtimeTerminalTicketTTL            = runtimeapi.RuntimeTerminalTicketTTL
	runtimeTerminalTicketKeyPrefix      = runtimeapi.RuntimeTerminalTicketKeyPrefix
	systemComponentGatewayTrafficProbe  = runtimeapi.SystemComponentGatewayTrafficProbe
)

var (
	errKubeGatewayEnqueue               = runtimeapi.ErrKubeGatewayEnqueue
	errRuntimeSecretMutationUnavailable = runtimeapi.ErrRuntimeSecretMutationUnavailable
	runtimeResourceCategories           = append([]string(nil), runtimeapi.RuntimeResourceCategories...)
	runtimeResourceKinds                = append([]string(nil), runtimeapi.RuntimeResourceKinds...)
	runtimeTerminalMemoryTickets        = runtimeapi.RuntimeTerminalMemoryTicketStore()
)

func normalizedKubeCredentialDays(value int) int {
	return runtimeapi.NormalizedKubeCredentialDays(value)
}
func runtimeProjectNamespace(project model.Project) string {
	return runtimeapi.RuntimeProjectNamespace(project)
}
func canUseRuntimeClusterForProject(user model.User, cluster model.RuntimeCluster, projectID string, boundProjectIDs []string) bool {
	return runtimeapi.CanUseRuntimeClusterForProject(user, cluster, projectID, boundProjectIDs)
}
func deploymentTargetNamespace(project model.Project, target model.DeploymentTarget) string {
	return runtimeapi.DeploymentTargetNamespace(project, target)
}
func runtimeClusterForDeploymentTargetDB(db *gorm.DB, target model.DeploymentTarget) (model.RuntimeCluster, error) {
	return runtimeapi.RuntimeClusterForDeploymentTargetDB(db, target)
}
func runtimeClusterResourcePolicy(input runtimeClusterInput) resourcepolicy.Policy {
	return runtimeapi.RuntimeClusterResourcePolicy(input)
}
func flattenKubeconfig(value string) (string, error) { return runtimeapi.FlattenKubeconfig(value) }
func normalizeRuntimeClusterType(value string) string {
	return runtimeapi.NormalizeRuntimeClusterType(value)
}
func normalizeGatewayRootDomain(value, fallbackValue string) string {
	return runtimeapi.NormalizeGatewayRootDomain(value, fallbackValue)
}
func normalizeGatewayDomainSuffixes(values []string, legacyValue, fallbackValue string) []string {
	return runtimeapi.NormalizeGatewayDomainSuffixes(values, legacyValue, fallbackValue)
}
func normalizeGatewayDomainSuffixList(values []string) []string {
	return runtimeapi.NormalizeGatewayDomainSuffixList(values)
}
func normalizeGatewayDomainSuffixValue(value string) string {
	return runtimeapi.NormalizeGatewayDomainSuffixValue(value)
}
func encodeGatewayDomainSuffixes(values []string) string {
	return runtimeapi.EncodeGatewayDomainSuffixes(values)
}
func decodeGatewayDomainSuffixes(raw, legacyValue, fallbackValue string) []string {
	return runtimeapi.DecodeGatewayDomainSuffixes(raw, legacyValue, fallbackValue)
}
func normalizeGatewayPublicScheme(value string) string {
	return runtimeapi.NormalizeGatewayPublicScheme(value)
}
func normalizePort(value, fallbackValue int) int {
	return runtimeapi.NormalizePort(value, fallbackValue)
}
func normalizeGatewayPublicPort(value int, scheme string) int {
	return runtimeapi.NormalizeGatewayPublicPort(value, scheme)
}
func normalizeGatewayProvider(value string) string { return runtimeapi.NormalizeGatewayProvider(value) }
func normalizeGatewayControllerType(value string) string {
	return runtimeapi.NormalizeGatewayControllerType(value)
}
func normalizeGatewayExternalTLSMode(value string) string {
	return runtimeapi.NormalizeGatewayExternalTLSMode(value)
}
func normalizeGatewayCertIssuerKind(value string) string {
	return runtimeapi.NormalizeGatewayCertIssuerKind(value)
}
func dnsLabelName(value string) string { return runtimeapi.DNSLabelName(value) }
func normalizeGatewayForwardedHeadersMode(value string) string {
	return runtimeapi.NormalizeGatewayForwardedHeadersMode(value)
}
func validateTrustedProxyCIDRs(value string) error {
	return runtimeapi.ValidateTrustedProxyCIDRs(value)
}

func runtimeClusterPressureFromSnapshot(clusterID string, snapshot kubeprovider.ClusterPressureSnapshot, detailed bool) runtimeClusterPressureResponse {
	return runtimeapi.RuntimeClusterPressureFromSnapshot(clusterID, snapshot, detailed)
}
func runtimeClusterPressureScore(cpuRequest, memoryRequest float64, cpuUsage, memoryUsage *float64) float64 {
	return runtimeapi.RuntimeClusterPressureScore(cpuRequest, memoryRequest, cpuUsage, memoryUsage)
}
func runtimeClusterPressureLevel(score float64) string {
	return runtimeapi.RuntimeClusterPressureLevel(score)
}
func unavailableRuntimeClusterPressure(clusterID, code string) runtimeClusterPressureResponse {
	return runtimeapi.UnavailableRuntimeClusterPressure(clusterID, code)
}
func runtimeClusterPressureIDs(raw []string) ([]string, bool) {
	return runtimeapi.RuntimeClusterPressureIDs(raw)
}
func validRuntimeResourceCategory(value string) bool {
	return runtimeapi.ValidRuntimeResourceCategory(value)
}
func validRuntimeResourceKind(value string) bool { return runtimeapi.ValidRuntimeResourceKind(value) }
func writeRuntimeResourceArgumentError(ctx *gin.Context, code, path string, allowedValues []string) {
	runtimeapi.WriteRuntimeResourceArgumentError(ctx, code, path, allowedValues)
}
func groupWorkloadPodResponses(items []clusterResourceResponse) []clusterResourceResponse {
	return runtimeapi.GroupWorkloadPodResponses(items)
}

func normalizePublicEnvironmentVariables(ctx *gin.Context, items []runtimeEnvironmentVariableInput) (map[string]string, bool) {
	return runtimeapi.NormalizePublicEnvironmentVariables(ctx, items)
}
func runtimeEnvironmentVariables(publicRaw, secretRaw string) []runtimeEnvironmentVariableResponse {
	return runtimeapi.RuntimeEnvironmentVariables(publicRaw, secretRaw)
}
func publicEnvironmentVariableInputs(raw string) []runtimeEnvironmentVariableInput {
	return runtimeapi.PublicEnvironmentVariableInputs(raw)
}
func runtimeSecretKeys(raw string) []string { return runtimeapi.RuntimeSecretKeys(raw) }
func setRuntimeSecretNoStoreHeaders(ctx *gin.Context) {
	runtimeapi.SetRuntimeSecretNoStoreHeaders(ctx)
}
func prepareRuntimeSecretMutation(input runtimeSecretMutationInput) (preparedRuntimeSecretMutation, error) {
	prepared, err := runtimeapi.PrepareRuntimeSecretMutation(input)
	if err != nil {
		return preparedRuntimeSecretMutation{}, err
	}
	return preparedRuntimeSecretMutation{
		values:        prepared.Values,
		configuredKey: prepared.ConfiguredKeys,
		generatedKey:  prepared.GeneratedKeys,
		clearKey:      prepared.ClearKeys,
	}, nil
}
func runtimeSecretMutationInputFromRequest(ctx *gin.Context, request runtimeSecretMutationRequest) (runtimeSecretMutationInput, bool) {
	return runtimeapi.RuntimeSecretMutationInputFromRequest(ctx, request)
}
func secretEnvironmentVariables(keys []string) []runtimeEnvironmentVariableResponse {
	return runtimeapi.SecretEnvironmentVariables(keys)
}
func decodeRuntimeSecretRefs(raw string) (map[string]string, error) {
	return runtimeapi.DecodeRuntimeSecretRefs(raw)
}
func validateRuntimeSecretMutation(ctx *gin.Context, input *runtimeSecretMutationInput) bool {
	return runtimeapi.ValidateRuntimeSecretMutation(ctx, input)
}
func writeRuntimeSecretMutationError(ctx *gin.Context, ownerType string, err error) {
	runtimeapi.WriteRuntimeSecretMutationError(ctx, ownerType, err)
}
func encodeStringMap(values map[string]string) string { return deploymentapi.EncodeStringMap(values) }
func normalizeRuntimeConfigFilesInput(ctx *gin.Context, value string) (string, bool) {
	return runtimeapi.NormalizeRuntimeConfigFilesInput(ctx, value)
}
func normalizeRuntimeConfigFilePathInput(ctx *gin.Context, value string) (string, bool) {
	return runtimeapi.NormalizeRuntimeConfigFilePathInput(ctx, value)
}
func decodeRuntimeClusterKubeGatewayRules(raw string) ([]runtimeClusterKubeGatewayRule, error) {
	return runtimeapi.DecodeRuntimeClusterKubeGatewayRules(raw)
}
func projectRuntimeConfigSetSecretMutationOwner(setID, projectID string) runtimeSecretMutationOwner {
	return runtimeapi.ProjectRuntimeConfigSetSecretMutationOwner(setID, projectID)
}
func runtimeExecAuditMessage(command string, result kubeprovider.RuntimeExecResult) string {
	return runtimeapi.RuntimeExecAuditMessage(command, result)
}
func requireRuntimeTerminalTicketForBearer(ctx *gin.Context, ticket string) bool {
	return runtimeapi.RequireRuntimeTerminalTicketForBearer(ctx, ticket)
}
func oauthAccessTokenSubject(tokenID string) string {
	return runtimeapi.OAuthAccessTokenSubject(tokenID)
}
func runtimeClusterPodTerminalReference(cluster model.RuntimeCluster, snapshot kubeprovider.ResourceSnapshot) runtimeClusterPodTerminalAuthorizationReference {
	return runtimeapi.RuntimeClusterPodTerminalReference(cluster, snapshot)
}
func sameRuntimeClusterPodTerminalResource(reference runtimeClusterPodTerminalAuthorizationReference, snapshot kubeprovider.ResourceSnapshot) bool {
	return runtimeapi.SameRuntimeClusterPodTerminalResource(reference, snapshot)
}
func runtimeTerminalResourceFingerprint(resourceKind string, resource any) string {
	return runtimeapi.RuntimeTerminalResourceFingerprint(resourceKind, resource)
}
func hasReadySystemComponent(items []model.SystemComponentInstallation, componentID string) bool {
	return runtimeapi.HasReadySystemComponent(items, componentID)
}
func systemComponentProbeEnv(cluster model.RuntimeCluster, componentID, mode, apiBaseURL, traefikMetricsURL string) string {
	return runtimeapi.SystemComponentProbeEnv(cluster, componentID, mode, apiBaseURL, traefikMetricsURL)
}
func validateOptionalHTTPURL(value, label string) error {
	return runtimeapi.ValidateOptionalHTTPURL(value, label)
}
func unavailableSystemComponent(item model.SystemComponentInstallation, code string) model.SystemComponentInstallation {
	return runtimeapi.UnavailableSystemComponent(item, code)
}
func runtimeClusterSafeAuditMetadata(cluster model.RuntimeCluster, kubeconfigUpdated bool) runtimeClusterAuditMetadata {
	return runtimeapi.RuntimeClusterSafeAuditMetadata(cluster, kubeconfigUpdated)
}
func runtimeClusterPersistenceAuditMessage(err error) string {
	return runtimeapi.RuntimeClusterPersistenceAuditMessage(err)
}

func (h *Handlers) runtimeClusterForEnvironment(ctx *gin.Context, environment model.Environment) (model.RuntimeCluster, bool) {
	return h.runtimeAPI().RuntimeClusterForEnvironment(ctx, environment)
}
func (h *Handlers) runtimeClusterForDeploymentTarget(ctx *gin.Context, target model.DeploymentTarget) (model.RuntimeCluster, bool) {
	return h.runtimeAPI().RuntimeClusterForDeploymentTarget(ctx, target)
}
func (h *Handlers) runtimeClusterForProjectUse(ctx *gin.Context, user model.User, projectID, clusterID string) (model.RuntimeCluster, bool) {
	return h.runtimeAPI().RuntimeClusterForProjectUse(ctx, user, projectID, clusterID)
}
func (h *Handlers) runtimeClusterResponseForUser(user model.User, cluster model.RuntimeCluster, ctx context.Context) (model.RuntimeCluster, error) {
	return h.runtimeAPI().RuntimeClusterResponseForUser(user, cluster, ctx)
}
func (h *Handlers) defaultRuntimeClusterID(ctx context.Context) string {
	return h.runtimeAPI().DefaultRuntimeClusterID(ctx)
}
func (h *Handlers) observeRuntimeClusters(ctx context.Context, clusters []model.RuntimeCluster) {
	h.runtimeAPI().ObserveRuntimeClusters(ctx, clusters)
}
func (h *Handlers) filterClusterResourceSnapshots(ctx *gin.Context, user model.User, items []kubeprovider.ResourceSnapshot, visibility projectservice.ListVisibility, projectID string) []kubeprovider.ResourceSnapshot {
	return h.runtimeAPI().FilterClusterResourceSnapshots(ctx, user, items, visibility, projectID)
}
func (h *Handlers) allowRuntimeClusterConnectionChange(ctx *gin.Context, existing model.RuntimeCluster, input runtimeClusterInput) bool {
	return h.runtimeAPI().AllowRuntimeClusterConnectionChange(ctx, existing, input)
}
func (h *Handlers) persistRuntimeClusterKubeGatewayDesired(ctx context.Context, clusterID string, enabled bool, encodedRules string) error {
	return h.runtimeAPI().PersistRuntimeClusterKubeGatewayDesired(ctx, clusterID, enabled, encodedRules)
}
func (h *Handlers) enqueueEnabledProjectAccessKubeGateways(ctx context.Context, userID string, includeGlobal bool) error {
	return h.runtimeAPI().EnqueueEnabledProjectAccessKubeGateways(ctx, userID, includeGlobal)
}
func (h *Handlers) runtimeClusterKubeGatewayManagerAndSpec(ctx context.Context, cluster model.RuntimeCluster) (*kubeprovider.KubectlGatewayManager, kubeprovider.GatewayAccessSpec, error) {
	return h.runtimeAPI().RuntimeClusterKubeGatewayManagerAndSpec(ctx, cluster)
}
func (h *Handlers) runtimeSecretFilesFromInput(ctx *gin.Context, user model.User, ownerID, value string, existing map[string]string) (map[string]string, bool) {
	return h.runtimeAPI().RuntimeSecretFilesFromInput(ctx, user, ownerID, value, existing)
}
func (h *Handlers) mutateRuntimeSecrets(ctx context.Context, user model.User, prepared preparedRuntimeSecretMutation, owner runtimeSecretMutationOwner) (runtimeSecretMutationResponse, error) {
	return h.runtimeAPI().MutateRuntimeSecrets(ctx, user, runtimeapi.PreparedRuntimeSecretMutation{
		Values:         prepared.values,
		ConfiguredKeys: prepared.configuredKey,
		GeneratedKeys:  prepared.generatedKey,
		ClearKeys:      prepared.clearKey,
	}, owner)
}
func (h *Handlers) requireRuntimeTerminalAuthorization(ctx *gin.Context, user model.User) (runtimeTerminalAuthorizationBinding, bool) {
	return h.runtimeAPI().RequireRuntimeTerminalAuthorization(ctx, user)
}
func (h *Handlers) currentInteractiveSubject(ctx *gin.Context, user model.User) (string, bool) {
	return h.runtimeAPI().CurrentInteractiveSubject(ctx, user)
}
func (h *Handlers) currentInteractiveAuthorizationBinding(ctx *gin.Context, user model.User) (runtimeTerminalAuthorizationBinding, bool) {
	return h.runtimeAPI().CurrentInteractiveAuthorizationBinding(ctx, user)
}
func (h *Handlers) releaseRuntimeTerminalAuthorizationAllowed(ctx context.Context, user model.User, reference releaseRuntimeTerminalAuthorizationReference) bool {
	return h.runtimeAPI().ReleaseRuntimeTerminalAuthorizationAllowed(ctx, user, reference)
}
func (h *Handlers) runtimeClusterPodTerminalAuthorizationAllowed(ctx context.Context, user model.User, client *kubeprovider.Client, reference runtimeClusterPodTerminalAuthorizationReference) bool {
	return h.runtimeAPI().RuntimeClusterPodTerminalAuthorizationAllowed(ctx, user, client, reference)
}
func (h *Handlers) issueRuntimeTerminalTicket(ctx context.Context, authorization runtimeTerminalAuthorizationBinding, resourceKind string, resource any) (string, time.Time, error) {
	return h.runtimeAPI().IssueRuntimeTerminalTicket(ctx, authorization, resourceKind, resource)
}
func (h *Handlers) consumeRuntimeTerminalTicket(ctx context.Context, ticket string) (runtimeTerminalTicketValue, bool, error) {
	value, ok, err := h.runtimeAPI().ConsumeRuntimeTerminalTicket(ctx, ticket)
	return runtimeTerminalTicketValue(value), ok, err
}
func (h *Handlers) systemComponentForBearerToken(token, componentID string, ctx context.Context) (model.SystemComponentInstallation, bool) {
	return h.runtimeAPI().SystemComponentForBearerToken(token, componentID, ctx)
}
func (h *Handlers) observeSystemComponentInstallations(ctx context.Context, items []model.SystemComponentInstallation) {
	h.runtimeAPI().ObserveSystemComponentInstallations(ctx, items)
}

type apiKubeGatewayReadiness struct {
	handlers *Handlers
}

func (readiness apiKubeGatewayReadiness) RequireReady(ctx context.Context, cluster model.RuntimeCluster, project model.Project) error {
	return readiness.handlers.runtimeAPI().RequireKubeGatewayReady(ctx, cluster, project)
}
