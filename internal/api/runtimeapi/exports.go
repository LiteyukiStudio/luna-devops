package runtimeapi

import (
	"context"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	projectservice "github.com/LiteyukiStudio/devops/internal/project"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	DefaultClusterBuildConcurrency = defaultClusterBuildConcurrency

	RuntimeEnvironmentValueModePublic = runtimeEnvironmentValueModePublic
	RuntimeEnvironmentValueModeSecret = runtimeEnvironmentValueModeSecret
	MaxRuntimeEnvironmentVariables    = maxRuntimeEnvironmentVariables
	MaxRuntimeEnvironmentValueLength  = maxRuntimeEnvironmentValueLength

	SystemComponentGatewayTrafficProbe = systemComponentGatewayTrafficProbe
)

type RuntimeConfigFileInput = runtimeConfigFileInput

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

type RuntimeClusterAuditMetadata = runtimeClusterAuditMetadata

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

func FlattenKubeconfig(kubeconfig string) (string, error) { return flattenKubeconfig(kubeconfig) }
func NormalizeGatewayPublicScheme(value string) string    { return normalizeGatewayPublicScheme(value) }
func NormalizePort(value, fallbackValue int) int          { return normalizePort(value, fallbackValue) }

func NormalizeRuntimeConfigFilesInput(ctx *gin.Context, value string) (string, bool) {
	return normalizeRuntimeConfigFilesInput(ctx, value)
}
func NormalizeRuntimeConfigFilePathInput(ctx *gin.Context, value string) (string, bool) {
	return normalizeRuntimeConfigFilePathInput(ctx, value)
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

func (h *Handler) RuntimeClusterForDeploymentTarget(ctx *gin.Context, target model.DeploymentTarget) (model.RuntimeCluster, bool) {
	return h.runtimeClusterForDeploymentTarget(ctx, target)
}
func (h *Handler) RuntimeClusterForProjectUse(ctx *gin.Context, user model.User, projectID, clusterID string) (model.RuntimeCluster, bool) {
	return h.runtimeClusterForProjectUse(ctx, user, projectID, clusterID)
}

func (h *Handler) DefaultRuntimeClusterID(ctx context.Context) string {
	return h.defaultRuntimeClusterID(ctx)
}
func (h *Handler) ObserveRuntimeClusters(ctx context.Context, clusters []model.RuntimeCluster) {
	h.observeRuntimeClusters(ctx, clusters)
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
