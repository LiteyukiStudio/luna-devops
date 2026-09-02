package buildapi

import (
	"context"
	"net/http"

	"github.com/LiteyukiStudio/devops/internal/buildenv"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/variables"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	DefaultClusterBuildConcurrency  = defaultClusterBuildConcurrency
	DefaultProjectBuildConcurrency  = defaultProjectBuildConcurrency
	BuildPushCredentialRequiredCode = buildPushCredentialRequiredCode
)

var (
	ErrBuildRunNotCancelable = errBuildRunNotCancelable
	ErrBuildQueueUnavailable = errBuildQueueUnavailable
)

type BuildEnvironmentConfigInput = buildEnvironmentConfigInput
type BuildEnvironmentConfigResponse = buildEnvironmentConfigResponse
type BuildRunInput = buildRunInput
type BuildTemplatePreviewInput = buildTemplatePreviewInput
type BuildVariableSetInput = buildVariableSetInput
type BuildVariableSetResponse = buildVariableSetResponse
type BuildRunRequestError = buildRunRequestError

func (e buildRunRequestError) Status() int              { return e.status }
func (e buildRunRequestError) Code() string             { return e.code }
func (e buildRunRequestError) Message() string          { return e.message }
func (e buildRunRequestError) PublicMessageKey() string { return e.publicMessageKey }

func NormalizeBuildConcurrency(value, defaultValue int) int {
	return normalizeBuildConcurrency(value, defaultValue)
}

func RegistryAuthEndpointForBuilder(endpoint string) string {
	return registryAuthEndpointForBuilder(endpoint)
}

func BuildImageRef(registry model.ArtifactRegistry, run model.BuildRun) string {
	return buildImageRef(registry, run)
}

func BuildTargetImageRepository(registry model.ArtifactRegistry, project model.Project, application model.Application) string {
	return buildTargetImageRepository(registry, project, application)
}

func BuildTargetImageRepositoryForCredential(registry model.ArtifactRegistry, credential model.RegistryCredential, project model.Project, application model.Application, target model.DeploymentTarget) string {
	return buildTargetImageRepositoryForCredential(registry, credential, project, application, target)
}

func BuildTargetImageTagTemplateForCredential(credential model.RegistryCredential) string {
	return buildTargetImageTagTemplateForCredential(credential)
}

func BuildStaticTargetImageTagForCredential(registry model.ArtifactRegistry, credential model.RegistryCredential, project model.Project, application model.Application, target model.DeploymentTarget) string {
	return buildStaticTargetImageTagForCredential(registry, credential, project, application, target)
}

func RepositoryWithoutRegistryHost(registry model.ArtifactRegistry, repository string) string {
	return repositoryWithoutRegistryHost(registry, repository)
}

func NormalizeImageRepositoryTemplate(value string) string {
	return normalizeImageRepositoryTemplate(value)
}

func NormalizeImageTagTemplate(value string) string { return normalizeImageTagTemplate(value) }

func IsDefaultImageRepository(registry model.ArtifactRegistry, project model.Project, application model.Application, repository string) bool {
	return isDefaultImageRepository(registry, project, application, repository)
}

func BuildImageNamePrefix(registry model.ArtifactRegistry, repository string) string {
	return buildImageNamePrefix(registry, repository)
}

func IsDockerHubRegistry(registry model.ArtifactRegistry) bool { return isDockerHubRegistry(registry) }
func HasRegistryHost(repository string) bool                   { return hasRegistryHost(repository) }

func RenderBuildTagTemplate(template string, ctx variables.Context) string {
	return renderBuildTagTemplate(template, ctx)
}

func SanitizeImageTag(value string) string { return sanitizeImageTag(value) }
func DNSSafeSegment(value string) string   { return dnsSafeSegment(value) }
func RegistryImageHost(endpoint string) string {
	return registryImageHost(endpoint)
}

func BuildRunPageQuery(query *gorm.DB, pagination paginationParams) *gorm.DB {
	return buildRunPageQuery(query, pagination)
}

func BuildRunStatusAllowed(status string) bool          { return buildRunStatusAllowed(status) }
func BuildRunCancelable(status string) bool             { return buildRunCancelable(status) }
func BuildRunTerminal(status string) bool               { return buildRunTerminal(status) }
func BuildRunTriggerAllowed(value string) bool          { return buildRunTriggerAllowed(value) }
func BuildRunActorName(user model.User) string          { return buildRunActorName(user) }
func SplitTargetImageRef(value string) (string, string) { return splitTargetImageRef(value) }

func BuildLogStreamOffset(ctx *gin.Context) int { return buildLogStreamOffset(ctx) }
func BuildJobTerminal(status string) bool       { return buildJobTerminal(status) }
func WriteSSE(writer http.ResponseWriter, event, idValue string, data any) {
	writeSSE(writer, event, idValue, data)
}
func FlushSSE(writer http.ResponseWriter) { flushSSE(writer) }

func BuildRunBadRequest(message string) error { return buildRunBadRequest(message) }
func BuildRunConflict(code, message string) error {
	return buildRunConflict(code, message)
}
func BuildRunPublicConflict(code, message string) error {
	return buildRunPublicConflict(code, message)
}
func FirstPositiveInt(values ...int) int { return firstPositiveInt(values...) }
func WriteBuildRunRequestError(ctx *gin.Context, err error) {
	writeBuildRunRequestError(ctx, err)
}

func NormalizeBuildVariables(ctx *gin.Context, input map[string]string) (map[string]string, bool) {
	return normalizeBuildVariables(ctx, input)
}

func NormalizeBuildArgsInput(ctx *gin.Context, raw string) (string, bool) {
	return normalizeBuildArgsInput(ctx, raw)
}

func NormalizeBuildArgsInputValue(raw string) string { return normalizeBuildArgsInputValue(raw) }
func ParseBuildArgsInput(raw string) (map[string]string, error) {
	return parseBuildArgsInput(raw)
}
func ValidateBuildArgs(values map[string]string) (map[string]string, error) {
	return validateBuildArgs(values)
}
func IsBuildEnvKey(value string) bool { return isBuildEnvKey(value) }
func EncodeBuildVariableSetIDs(ids []string) string {
	return encodeBuildVariableSetIDs(ids)
}
func BuildVariableSetIDs(raw string) []string { return buildVariableSetIDs(raw) }
func RemoveBuildVariableSetID(raw, setID string) string {
	return removeBuildVariableSetID(raw, setID)
}
func NormalizeBuildSelectorList(values []string) []string {
	return normalizeBuildSelectorList(values)
}
func BuilderHasLabels(rawLabels string, requiredLabels []string) bool {
	return builderHasLabels(rawLabels, requiredLabels)
}
func BuilderAllowsRun(rawScopes, projectID, userID string) bool {
	return builderAllowsRun(rawScopes, projectID, userID)
}
func BuilderVisibleToUser(rawScopes, userID string, projectIDs []string) bool {
	return builderVisibleToUser(rawScopes, userID, projectIDs)
}
func ApplyBuildVariableSetValues(output map[string]string, set model.BuildVariableSet, resolveSecret func(string) string) {
	applyBuildVariableSetValues(output, set, resolveSecret)
}
func DecodeSecretRefs(raw string) map[string]string { return decodeSecretRefs(raw) }
func BuildVariableSetVariableCount(raw string) int  { return buildVariableSetVariableCount(raw) }
func BuildEnvironmentConfigResponseFromModel(config model.BuildEnvironmentConfig) BuildEnvironmentConfigResponse {
	return buildEnvironmentConfigResponseFromModel(config)
}

func (h *Handler) DeploymentBuildEnvironmentFromInput(ctx *gin.Context, user model.User, projectID, targetID string, input DeploymentBuildEnvironmentInput, existing *model.BuildEnvironmentConfig) (*model.BuildEnvironmentConfig, bool) {
	return h.deploymentBuildEnvironmentFromInput(ctx, user, projectID, targetID, input, existing)
}

func (h *Handler) AuthorizeBuildEnvironmentConfig(ctx *gin.Context, user model.User) (string, string, string, bool) {
	return h.authorizeBuildEnvironmentConfig(ctx, user)
}

func (h *Handler) BuildEnvironmentConfigFromInput(ctx *gin.Context, user model.User, scope, scopeRef string, input BuildEnvironmentConfigInput, existingSecretRefs map[string]string) (model.BuildEnvironmentConfig, bool) {
	return h.buildEnvironmentConfigFromInput(ctx, user, scope, scopeRef, input, existingSecretRefs)
}

func (h *Handler) BuildEnvironmentSecretRefsFromInput(ctx *gin.Context, user model.User, scope, scopeRef string, input, existing map[string]string) (map[string]string, bool) {
	return h.buildEnvironmentSecretRefsFromInput(ctx, user, scope, scopeRef, input, existing)
}

func (h *Handler) CanManageBuildEnvironmentProject(ctx *gin.Context, user model.User, projectID string) bool {
	return h.canManageBuildEnvironmentProject(ctx, user, projectID)
}

func (h *Handler) FindBuildEnvironmentConfig(db *gorm.DB, scope, scopeRef string) (model.BuildEnvironmentConfig, error) {
	return h.findBuildEnvironmentConfig(db, scope, scopeRef)
}

func (h *Handler) PrepareBuildRunRequest(user model.User, run *model.BuildRun, ctx context.Context) error {
	return h.prepareBuildRunRequest(user, run, ctx)
}

func (h *Handler) QueueBuildRun(ctx context.Context, user model.User, run model.BuildRun) (model.BuildRun, error) {
	return h.queueBuildRun(ctx, user, run)
}

func (h *Handler) CreateQueuedBuildRun(ctx *gin.Context, user model.User, run model.BuildRun, targetImageRef string, statusCode int) {
	h.createQueuedBuildRun(ctx, user, run, targetImageRef, statusCode)
}

func (h *Handler) ValidateBuildRunRequest(ctx *gin.Context, user model.User, run *model.BuildRun) bool {
	return h.validateBuildRunRequest(ctx, user, run)
}

func (h *Handler) MarkBuildRunDispatchFailed(run model.BuildRun, job model.BuildJob, message string, ctx context.Context) {
	h.markBuildRunDispatchFailed(run, job, message, ctx)
}

func (h *Handler) FindBuildRun(ctx *gin.Context) (model.BuildRun, bool) {
	return h.findBuildRun(ctx)
}

func (h *Handler) BuildRunFromInput(projectID string, user model.User, input BuildRunInput) model.BuildRun {
	return h.buildRunFromInput(projectID, user, input)
}

func (h *Handler) WriteBuildLogStreamChunk(ctx *gin.Context, job model.BuildJob, offset int) (int, bool, error) {
	return h.writeBuildLogStreamChunk(ctx, job, offset)
}

func (h *Handler) BuildVariablesForRun(ctx *gin.Context, user model.User, projectID string, setIDs []string) (map[string]string, bool) {
	return h.buildVariablesForRun(ctx, user, projectID, setIDs)
}

func (h *Handler) BuildVariablesForRunByIDs(db *gorm.DB, user model.User, projectID string, setIDs []string, ctx context.Context) (map[string]string, error) {
	return h.buildVariablesForRunByIDs(db, user, projectID, setIDs, ctx)
}

func (h *Handler) BuildEnvironmentSnapshotForRun(db *gorm.DB, user model.User, run model.BuildRun, ctx context.Context) (buildenv.Snapshot, error) {
	return h.buildEnvironmentSnapshotForRun(db, user, run, ctx)
}

func (h *Handler) BuildVariableSetResponseForUser(user model.User, set model.BuildVariableSet, ctx context.Context) (BuildVariableSetResponse, error) {
	return h.buildVariableSetResponseForUser(user, set, ctx)
}

func (h *Handler) BuildVariableSetResponsesForUser(user model.User, sets []model.BuildVariableSet, ctx context.Context) ([]BuildVariableSetResponse, error) {
	return h.buildVariableSetResponsesForUser(user, sets, ctx)
}

func (h *Handler) BuildVariableSetFromInput(ctx *gin.Context, user model.User, input BuildVariableSetInput, setID string, existingSecretRefs map[string]string) (model.BuildVariableSet, bool) {
	return h.buildVariableSetFromInput(ctx, user, input, setID, existingSecretRefs)
}

func (h *Handler) SaveBuildVariableSet(set model.BuildVariableSet, ctx context.Context) error {
	return h.saveBuildVariableSet(set, ctx)
}

func (h *Handler) AttachBuildVariableSetProjects(sets []model.BuildVariableSet, ctx context.Context) {
	h.attachBuildVariableSetProjects(sets, ctx)
}

func (h *Handler) BuildVariableSecretRefsFromInput(ctx *gin.Context, user model.User, setID string, input, existing map[string]string) (map[string]string, bool) {
	return h.buildVariableSecretRefsFromInput(ctx, user, setID, input, existing)
}

func (h *Handler) BuildVariableSetsForRun(db *gorm.DB, user model.User, projectID string, setIDs []string, ctx context.Context) ([]model.BuildVariableSet, error) {
	return h.buildVariableSetsForRun(db, user, projectID, setIDs, ctx)
}

func (h *Handler) BuildVariableSetAccessible(user model.User, projectID string, set model.BuildVariableSet, ctx context.Context) bool {
	return h.buildVariableSetAccessible(user, projectID, set, ctx)
}

func BuildVariableSetModelIDs(sets []model.BuildVariableSet) []string {
	return buildVariableSetModelIDs(sets)
}

func (h *Handler) DeploymentTargetForBuildRun(app model.Application, targetID string, ctx context.Context) (model.DeploymentTarget, error) {
	return h.deploymentTargetForBuildRun(app, targetID, ctx)
}

func (h *Handler) DeploymentTargetForRun(ctx *gin.Context, app model.Application, targetID string) (model.DeploymentTarget, bool) {
	return h.deploymentTargetForRun(ctx, app, targetID)
}
