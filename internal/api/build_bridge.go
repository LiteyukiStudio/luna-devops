package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/api/buildapi"
	"github.com/LiteyukiStudio/devops/internal/api/projectapi"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/buildenv"
	"github.com/LiteyukiStudio/devops/internal/model"
	projectservice "github.com/LiteyukiStudio/devops/internal/project"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/LiteyukiStudio/devops/internal/variables"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	defaultClusterBuildConcurrency  = buildapi.DefaultClusterBuildConcurrency
	defaultProjectBuildConcurrency  = buildapi.DefaultProjectBuildConcurrency
	buildPushCredentialRequiredCode = buildapi.BuildPushCredentialRequiredCode
)

var (
	errBuildRunNotCancelable = buildapi.ErrBuildRunNotCancelable
	errBuildQueueUnavailable = buildapi.ErrBuildQueueUnavailable
)

type buildHost struct {
	handlers *Handlers
}

func (host buildHost) DBFor(ctx *gin.Context) *gorm.DB { return host.handlers.dbFor(ctx) }
func (host buildHost) DBWithContext(ctx context.Context) *gorm.DB {
	return host.handlers.dbWithContext(ctx)
}
func (host buildHost) CurrentUser(ctx *gin.Context) (model.User, bool) {
	return host.handlers.currentUser(ctx)
}
func (host buildHost) AuthorizeProject(ctx *gin.Context, action authz.Action) (model.User, model.Project, bool) {
	return host.handlers.authorizeProject(ctx, action)
}
func (host buildHost) ResolveListVisibility(ctx *gin.Context, user model.User) (projectservice.ListVisibility, bool) {
	return resolveListVisibility(ctx, user)
}
func (host buildHost) ApplyScopedResourceListVisibility(ctx *gin.Context, query *gorm.DB, resourceType string, user model.User, projectID string, visibility projectservice.ListVisibility) (*gorm.DB, bool) {
	return host.handlers.applyScopedResourceListVisibility(ctx, query, resourceType, user, projectID, visibility)
}
func (host buildHost) NormalizeScopedOwnerWithProjects(ctx *gin.Context, user model.User, scope, ownerRef string, projectIDs []string, globalError string) (string, string, []string, bool) {
	return host.handlers.normalizeScopedOwnerWithProjects(ctx, user, scope, ownerRef, projectIDs, globalError)
}
func (host buildHost) CanManageScopedResourceByID(ctx *gin.Context, user model.User, scope, ownerRef, resourceType, resourceID, errorMessage string) bool {
	return host.handlers.canManageScopedResourceByID(ctx, user, scope, ownerRef, resourceType, resourceID, errorMessage)
}
func (host buildHost) CanInspectScopedResourceConfigByID(user model.User, scope, ownerRef, resourceType, resourceID string, ctx context.Context) (bool, error) {
	return host.handlers.canInspectScopedResourceConfigByID(user, scope, ownerRef, resourceType, resourceID, ctx)
}
func (host buildHost) ReplaceScopedResourceProjectBindings(tx *gorm.DB, resourceType, resourceID string, projectIDs, defaultProjectIDs []string) error {
	return host.handlers.replaceScopedResourceProjectBindings(tx, resourceType, resourceID, projectIDs, defaultProjectIDs)
}
func (host buildHost) ScopedResourceProjectIDs(resourceType, resourceID string, ctx context.Context) []string {
	return host.handlers.scopedResourceProjectIDs(resourceType, resourceID, ctx)
}
func (host buildHost) ScopedResourceProjectIDMap(resourceType string, resourceIDs []string, ctx context.Context) map[string][]string {
	return host.handlers.scopedResourceProjectIDMap(resourceType, resourceIDs, ctx)
}
func (host buildHost) ProjectRoleActionAllowed(ctx context.Context, user model.User, projectID string, action authz.Action) (bool, error) {
	return host.handlers.projectRoleActionAllowed(ctx, user, projectID, action)
}
func (host buildHost) WriteProjectAuthorizationError(ctx *gin.Context, err error) {
	writeProjectAuthorizationError(ctx, err)
}
func (host buildHost) AuditWithContext(userID, action, resource string, success bool, message string, ctx context.Context) {
	host.handlers.auditWithContext(userID, action, resource, success, message, ctx)
}
func (host buildHost) EnsureBillingAllowsNewBuild(ctx *gin.Context, projectID string) bool {
	return host.handlers.ensureBillingAllowsNewBuild(ctx, projectID)
}
func (host buildHost) RequireContinuousAuthorizationBinding(ctx *gin.Context, user model.User) (projectapi.ContinuousAuthorizationBinding, bool) {
	return host.handlers.requireContinuousAuthorizationBinding(ctx, user)
}
func (host buildHost) MonitorContinuousAuthorization(ctx context.Context, binding projectapi.ContinuousAuthorizationBinding, authorizationAllowed func(context.Context, model.User) bool, revoke func()) (<-chan struct{}, bool) {
	return host.handlers.monitorContinuousAuthorization(ctx, binding, authorizationAllowed, revoke)
}
func (host buildHost) ProjectContinuousAuthorizationAllowed(ctx context.Context, user model.User, projectID string, action authz.Action) bool {
	return host.handlers.projectContinuousAuthorizationAllowed(ctx, user, projectID, action)
}
func (host buildHost) RegistryPushCredentialForProject(user model.User, registry model.ArtifactRegistry, projectID string, ctx context.Context) (model.RegistryCredential, bool) {
	return host.handlers.registryPushCredentialForProject(user, registry, projectID, ctx)
}
func (host buildHost) StoreSecret(ctx context.Context, value, userID, resource string) string {
	return host.handlers.secrets.StoreContext(ctx, value, userID, resource)
}
func (host buildHost) ResolveSecret(ctx context.Context, ref string) string {
	return host.handlers.secrets.ResolveContext(ctx, ref)
}
func (host buildHost) BuildQueueAvailable() bool { return host.handlers.taskClient != nil }
func (host buildHost) EnqueueBuildRun(ctx context.Context, payload tasks.BuildRunPayload) error {
	_, err := host.handlers.taskClient.EnqueueBuildRun(ctx, payload)
	return err
}
func (host buildHost) ApplicationCanMutate(app model.Application) bool {
	status := strings.TrimSpace(app.DeleteStatus)
	return status == "" || status == "active" || status == "delete_failed"
}
func (host buildHost) NormalizeDeploymentSourceType(value string) string {
	return normalizeDeploymentSourceType(value)
}
func (host buildHost) NormalizeBuildResourceQuantityValue(value, fallbackValue, label string) (string, error) {
	return normalizeBuildResourceQuantityValue(value, fallbackValue, label)
}
func (host buildHost) NormalizeBuildTimeoutSecondsValue(value int) int {
	return normalizeBuildTimeoutSecondsValue(value)
}

func (h *Handlers) buildAPI() *buildapi.Handler { return buildapi.New(buildHost{handlers: h}) }

func (h *Handlers) GetBuildEnvironmentConfig(ctx *gin.Context) {
	h.buildAPI().GetBuildEnvironmentConfig(ctx)
}
func (h *Handlers) UpdateBuildEnvironmentConfig(ctx *gin.Context) {
	h.buildAPI().UpdateBuildEnvironmentConfig(ctx)
}
func (h *Handlers) ListBuildJobs(ctx *gin.Context)      { h.buildAPI().ListBuildJobs(ctx) }
func (h *Handlers) GetBuildJob(ctx *gin.Context)        { h.buildAPI().GetBuildJob(ctx) }
func (h *Handlers) GetBuildJobLogs(ctx *gin.Context)    { h.buildAPI().GetBuildJobLogs(ctx) }
func (h *Handlers) StreamBuildJobLogs(ctx *gin.Context) { h.buildAPI().StreamBuildJobLogs(ctx) }
func (h *Handlers) ListBuildRuns(ctx *gin.Context)      { h.buildAPI().ListBuildRuns(ctx) }
func (h *Handlers) GetBuildRun(ctx *gin.Context)        { h.buildAPI().GetBuildRun(ctx) }
func (h *Handlers) TriggerBuildRun(ctx *gin.Context)    { h.buildAPI().TriggerBuildRun(ctx) }
func (h *Handlers) RetryBuildRun(ctx *gin.Context)      { h.buildAPI().RetryBuildRun(ctx) }
func (h *Handlers) CancelBuildRun(ctx *gin.Context)     { h.buildAPI().CancelBuildRun(ctx) }
func (h *Handlers) DeleteBuildRun(ctx *gin.Context)     { h.buildAPI().DeleteBuildRun(ctx) }
func (h *Handlers) ListBuildTemplates(ctx *gin.Context) { h.buildAPI().ListBuildTemplates(ctx) }
func (h *Handlers) PreviewBuildTemplate(ctx *gin.Context) {
	h.buildAPI().PreviewBuildTemplate(ctx)
}
func (h *Handlers) ListBuildVariableSets(ctx *gin.Context) {
	h.buildAPI().ListBuildVariableSets(ctx)
}
func (h *Handlers) CreateBuildVariableSet(ctx *gin.Context) {
	h.buildAPI().CreateBuildVariableSet(ctx)
}
func (h *Handlers) UpdateBuildVariableSet(ctx *gin.Context) {
	h.buildAPI().UpdateBuildVariableSet(ctx)
}
func (h *Handlers) DeleteBuildVariableSet(ctx *gin.Context) {
	h.buildAPI().DeleteBuildVariableSet(ctx)
}

type buildEnvironmentConfigInput = buildapi.BuildEnvironmentConfigInput
type buildEnvironmentConfigResponse = buildapi.BuildEnvironmentConfigResponse
type buildRunInput = buildapi.BuildRunInput
type buildTemplatePreviewInput = buildapi.BuildTemplatePreviewInput
type buildVariableSetInput = buildapi.BuildVariableSetInput
type buildVariableSetResponse = buildapi.BuildVariableSetResponse

func (h *Handlers) deploymentBuildEnvironmentFromInput(ctx *gin.Context, user model.User, projectID, targetID string, input deploymentTargetInput, existing *model.BuildEnvironmentConfig) (*model.BuildEnvironmentConfig, bool) {
	return h.buildAPI().DeploymentBuildEnvironmentFromInput(ctx, user, projectID, targetID, buildapi.DeploymentBuildEnvironmentInput{
		BuildVariables: input.BuildVariables,
		BuildSecrets:   input.BuildSecrets,
	}, existing)
}
func (h *Handlers) authorizeBuildEnvironmentConfig(ctx *gin.Context, user model.User) (string, string, string, bool) {
	return h.buildAPI().AuthorizeBuildEnvironmentConfig(ctx, user)
}
func (h *Handlers) canManageBuildEnvironmentProject(ctx *gin.Context, user model.User, projectID string) bool {
	return h.buildAPI().CanManageBuildEnvironmentProject(ctx, user, projectID)
}
func (h *Handlers) buildEnvironmentConfigFromInput(ctx *gin.Context, user model.User, scope, scopeRef string, input buildEnvironmentConfigInput, existingSecretRefs map[string]string) (model.BuildEnvironmentConfig, bool) {
	return h.buildAPI().BuildEnvironmentConfigFromInput(ctx, user, scope, scopeRef, input, existingSecretRefs)
}
func (h *Handlers) buildEnvironmentSecretRefsFromInput(ctx *gin.Context, user model.User, scope, scopeRef string, input, existing map[string]string) (map[string]string, bool) {
	return h.buildAPI().BuildEnvironmentSecretRefsFromInput(ctx, user, scope, scopeRef, input, existing)
}
func (h *Handlers) findBuildEnvironmentConfig(db *gorm.DB, scope, scopeRef string) (model.BuildEnvironmentConfig, error) {
	return h.buildAPI().FindBuildEnvironmentConfig(db, scope, scopeRef)
}
func buildEnvironmentConfigResponseFromModel(config model.BuildEnvironmentConfig) buildEnvironmentConfigResponse {
	return buildapi.BuildEnvironmentConfigResponseFromModel(config)
}

func (h *Handlers) createQueuedBuildRun(ctx *gin.Context, user model.User, run model.BuildRun, targetImageRef string, statusCode int) {
	h.buildAPI().CreateQueuedBuildRun(ctx, user, run, targetImageRef, statusCode)
}
func (h *Handlers) validateBuildRunRequest(ctx *gin.Context, user model.User, run *model.BuildRun) bool {
	return h.buildAPI().ValidateBuildRunRequest(ctx, user, run)
}
func (h *Handlers) queueBuildRun(ctx context.Context, user model.User, run model.BuildRun) (model.BuildRun, error) {
	return h.buildAPI().QueueBuildRun(ctx, user, run)
}
func (h *Handlers) markBuildRunDispatchFailed(run model.BuildRun, job model.BuildJob, message string, ctx context.Context) {
	h.buildAPI().MarkBuildRunDispatchFailed(run, job, message, ctx)
}
func (h *Handlers) findBuildRun(ctx *gin.Context) (model.BuildRun, bool) {
	return h.buildAPI().FindBuildRun(ctx)
}
func (h *Handlers) buildRunFromInput(projectID string, user model.User, input buildRunInput) model.BuildRun {
	return h.buildAPI().BuildRunFromInput(projectID, user, input)
}
func (h *Handlers) writeBuildLogStreamChunk(ctx *gin.Context, job model.BuildJob, offset int) (int, bool, error) {
	return h.buildAPI().WriteBuildLogStreamChunk(ctx, job, offset)
}
func (h *Handlers) prepareBuildRunRequest(user model.User, run *model.BuildRun, ctx context.Context) error {
	err := h.buildAPI().PrepareBuildRunRequest(user, run, ctx)
	var requestErr buildapi.BuildRunRequestError
	if errors.As(err, &requestErr) {
		return buildRunRequestError{
			status:           requestErr.Status(),
			code:             requestErr.Code(),
			message:          requestErr.Message(),
			publicMessageKey: requestErr.PublicMessageKey(),
		}
	}
	return err
}
func (h *Handlers) buildVariablesForRun(ctx *gin.Context, user model.User, projectID string, setIDs []string) (map[string]string, bool) {
	return h.buildAPI().BuildVariablesForRun(ctx, user, projectID, setIDs)
}
func (h *Handlers) buildVariablesForRunByIDs(db *gorm.DB, user model.User, projectID string, setIDs []string, ctx context.Context) (map[string]string, error) {
	return h.buildAPI().BuildVariablesForRunByIDs(db, user, projectID, setIDs, ctx)
}
func (h *Handlers) buildEnvironmentSnapshotForRun(db *gorm.DB, user model.User, run model.BuildRun, ctx context.Context) (buildenv.Snapshot, error) {
	return h.buildAPI().BuildEnvironmentSnapshotForRun(db, user, run, ctx)
}
func (h *Handlers) buildVariableSetResponseForUser(user model.User, set model.BuildVariableSet, ctx context.Context) (buildVariableSetResponse, error) {
	return h.buildAPI().BuildVariableSetResponseForUser(user, set, ctx)
}
func (h *Handlers) buildVariableSetResponsesForUser(user model.User, sets []model.BuildVariableSet, ctx context.Context) ([]buildVariableSetResponse, error) {
	return h.buildAPI().BuildVariableSetResponsesForUser(user, sets, ctx)
}
func (h *Handlers) buildVariableSetFromInput(ctx *gin.Context, user model.User, input buildVariableSetInput, setID string, existingSecretRefs map[string]string) (model.BuildVariableSet, bool) {
	return h.buildAPI().BuildVariableSetFromInput(ctx, user, input, setID, existingSecretRefs)
}
func (h *Handlers) saveBuildVariableSet(set model.BuildVariableSet, ctx context.Context) error {
	return h.buildAPI().SaveBuildVariableSet(set, ctx)
}
func (h *Handlers) attachBuildVariableSetProjects(sets []model.BuildVariableSet, ctx context.Context) {
	h.buildAPI().AttachBuildVariableSetProjects(sets, ctx)
}
func buildVariableSetModelIDs(sets []model.BuildVariableSet) []string {
	return buildapi.BuildVariableSetModelIDs(sets)
}
func (h *Handlers) buildVariableSecretRefsFromInput(ctx *gin.Context, user model.User, setID string, input, existing map[string]string) (map[string]string, bool) {
	return h.buildAPI().BuildVariableSecretRefsFromInput(ctx, user, setID, input, existing)
}
func (h *Handlers) buildVariableSetsForRun(db *gorm.DB, user model.User, projectID string, setIDs []string, ctx context.Context) ([]model.BuildVariableSet, error) {
	return h.buildAPI().BuildVariableSetsForRun(db, user, projectID, setIDs, ctx)
}
func (h *Handlers) buildVariableSetAccessible(user model.User, projectID string, set model.BuildVariableSet, ctx context.Context) bool {
	return h.buildAPI().BuildVariableSetAccessible(user, projectID, set, ctx)
}
func (h *Handlers) deploymentTargetForBuildRun(app model.Application, targetID string, ctx context.Context) (model.DeploymentTarget, error) {
	return h.buildAPI().DeploymentTargetForBuildRun(app, targetID, ctx)
}
func (h *Handlers) deploymentTargetForRun(ctx *gin.Context, app model.Application, targetID string) (model.DeploymentTarget, bool) {
	return h.buildAPI().DeploymentTargetForRun(ctx, app, targetID)
}

type buildRunRequestError struct {
	status           int
	code             string
	message          string
	publicMessageKey string
}

func (e buildRunRequestError) Error() string { return e.message }
func buildRunBadRequest(message string) error {
	return buildRunRequestError{status: http.StatusBadRequest, message: message}
}
func buildRunConflict(code, message string) error {
	return buildRunRequestError{status: http.StatusConflict, code: code, message: message}
}
func buildRunPublicConflict(code, message string) error {
	return buildRunRequestError{status: http.StatusConflict, code: code, message: message, publicMessageKey: code}
}
func firstPositiveInt(values ...int) int { return buildapi.FirstPositiveInt(values...) }
func writeBuildRunRequestError(ctx *gin.Context, err error) {
	var requestErr buildRunRequestError
	if !errors.As(err, &requestErr) {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if requestErr.publicMessageKey != "" {
		writeLocalizedErrorCode(ctx, requestErr.status, requestErr.code, requestErr.message, requestErr.publicMessageKey)
		return
	}
	if requestErr.code != "" {
		writeErrorCode(ctx, requestErr.status, requestErr.code, requestErr.message)
		return
	}
	writeError(ctx, requestErr.status, requestErr.message)
}

func normalizeBuildConcurrency(value, defaultValue int) int {
	return buildapi.NormalizeBuildConcurrency(value, defaultValue)
}
func registryAuthEndpointForBuilder(endpoint string) string {
	return buildapi.RegistryAuthEndpointForBuilder(endpoint)
}
func buildImageRef(registry model.ArtifactRegistry, run model.BuildRun) string {
	return buildapi.BuildImageRef(registry, run)
}
func buildTargetImageRepository(registry model.ArtifactRegistry, project model.Project, application model.Application) string {
	return buildapi.BuildTargetImageRepository(registry, project, application)
}
func buildTargetImageRepositoryForCredential(registry model.ArtifactRegistry, credential model.RegistryCredential, project model.Project, application model.Application, target model.DeploymentTarget) string {
	return buildapi.BuildTargetImageRepositoryForCredential(registry, credential, project, application, target)
}
func buildTargetImageTagTemplateForCredential(credential model.RegistryCredential) string {
	return buildapi.BuildTargetImageTagTemplateForCredential(credential)
}
func buildStaticTargetImageTagForCredential(registry model.ArtifactRegistry, credential model.RegistryCredential, project model.Project, application model.Application, target model.DeploymentTarget) string {
	return buildapi.BuildStaticTargetImageTagForCredential(registry, credential, project, application, target)
}
func repositoryWithoutRegistryHost(registry model.ArtifactRegistry, repository string) string {
	return buildapi.RepositoryWithoutRegistryHost(registry, repository)
}
func normalizeImageRepositoryTemplate(value string) string {
	return buildapi.NormalizeImageRepositoryTemplate(value)
}
func normalizeImageTagTemplate(value string) string { return buildapi.NormalizeImageTagTemplate(value) }
func isDefaultImageRepository(registry model.ArtifactRegistry, project model.Project, application model.Application, repository string) bool {
	return buildapi.IsDefaultImageRepository(registry, project, application, repository)
}
func buildImageNamePrefix(registry model.ArtifactRegistry, repository string) string {
	return buildapi.BuildImageNamePrefix(registry, repository)
}
func isDockerHubRegistry(registry model.ArtifactRegistry) bool {
	return buildapi.IsDockerHubRegistry(registry)
}
func hasRegistryHost(repository string) bool { return buildapi.HasRegistryHost(repository) }
func renderBuildTagTemplate(template string, ctx variables.Context) string {
	return buildapi.RenderBuildTagTemplate(template, ctx)
}
func sanitizeImageTag(value string) string { return buildapi.SanitizeImageTag(value) }
func dnsSafeSegment(value string) string   { return buildapi.DNSSafeSegment(value) }
func registryImageHost(endpoint string) string {
	return buildapi.RegistryImageHost(endpoint)
}

func buildRunPageQuery(query *gorm.DB, pagination paginationParams) *gorm.DB {
	return buildapi.BuildRunPageQuery(query, pagination)
}
func buildRunStatusAllowed(status string) bool { return buildapi.BuildRunStatusAllowed(status) }
func buildRunCancelable(status string) bool    { return buildapi.BuildRunCancelable(status) }
func buildRunTerminal(status string) bool      { return buildapi.BuildRunTerminal(status) }
func buildRunTriggerAllowed(value string) bool { return buildapi.BuildRunTriggerAllowed(value) }
func buildRunActorName(user model.User) string { return buildapi.BuildRunActorName(user) }
func splitTargetImageRef(value string) (string, string) {
	return buildapi.SplitTargetImageRef(value)
}
func buildLogStreamOffset(ctx *gin.Context) int { return buildapi.BuildLogStreamOffset(ctx) }
func buildJobTerminal(status string) bool       { return buildapi.BuildJobTerminal(status) }
func writeSSE(writer http.ResponseWriter, event, idValue string, data any) {
	buildapi.WriteSSE(writer, event, idValue, data)
}
func flushSSE(writer http.ResponseWriter) { buildapi.FlushSSE(writer) }

func normalizeBuildVariables(ctx *gin.Context, input map[string]string) (map[string]string, bool) {
	return buildapi.NormalizeBuildVariables(ctx, input)
}
func normalizeBuildArgsInput(ctx *gin.Context, raw string) (string, bool) {
	return buildapi.NormalizeBuildArgsInput(ctx, raw)
}
func normalizeBuildArgsInputValue(raw string) string {
	return buildapi.NormalizeBuildArgsInputValue(raw)
}
func parseBuildArgsInput(raw string) (map[string]string, error) {
	return buildapi.ParseBuildArgsInput(raw)
}
func validateBuildArgs(values map[string]string) (map[string]string, error) {
	return buildapi.ValidateBuildArgs(values)
}
func isBuildEnvKey(value string) bool { return buildapi.IsBuildEnvKey(value) }
func encodeBuildVariableSetIDs(ids []string) string {
	return buildapi.EncodeBuildVariableSetIDs(ids)
}
func buildVariableSetIDs(raw string) []string { return buildapi.BuildVariableSetIDs(raw) }
func removeBuildVariableSetID(raw, setID string) string {
	return buildapi.RemoveBuildVariableSetID(raw, setID)
}
func normalizeBuildSelectorList(values []string) []string {
	return buildapi.NormalizeBuildSelectorList(values)
}
func builderHasLabels(rawLabels string, requiredLabels []string) bool {
	return buildapi.BuilderHasLabels(rawLabels, requiredLabels)
}
func builderAllowsRun(rawScopes, projectID, userID string) bool {
	return buildapi.BuilderAllowsRun(rawScopes, projectID, userID)
}
func builderVisibleToUser(rawScopes, userID string, projectIDs []string) bool {
	return buildapi.BuilderVisibleToUser(rawScopes, userID, projectIDs)
}
func applyBuildVariableSetValues(output map[string]string, set model.BuildVariableSet, resolveSecret func(string) string) {
	buildapi.ApplyBuildVariableSetValues(output, set, resolveSecret)
}
func decodeSecretRefs(raw string) map[string]string { return buildapi.DecodeSecretRefs(raw) }
func buildVariableSetVariableCount(raw string) int {
	return buildapi.BuildVariableSetVariableCount(raw)
}
