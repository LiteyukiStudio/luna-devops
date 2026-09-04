package buildapi

import (
	"context"
	"net/http"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/variables"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	DefaultClusterBuildConcurrency = defaultClusterBuildConcurrency

	BuildPushCredentialRequiredCode = buildPushCredentialRequiredCode
)

type BuildVariableSetResponse = buildVariableSetResponse
type BuildRunRequestError = buildRunRequestError

func (e buildRunRequestError) Status() int     { return e.status }
func (e buildRunRequestError) Code() string    { return e.code }
func (e buildRunRequestError) Message() string { return e.message }

func NormalizeBuildConcurrency(value, defaultValue int) int {
	return normalizeBuildConcurrency(value, defaultValue)
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

func IsDefaultImageRepository(registry model.ArtifactRegistry, project model.Project, application model.Application, repository string) bool {
	return isDefaultImageRepository(registry, project, application, repository)
}

func BuildImageNamePrefix(registry model.ArtifactRegistry, repository string) string {
	return buildImageNamePrefix(registry, repository)
}

func IsDockerHubRegistry(registry model.ArtifactRegistry) bool { return isDockerHubRegistry(registry) }

func RenderBuildTagTemplate(template string, ctx variables.Context) string {
	return renderBuildTagTemplate(template, ctx)
}

func RegistryImageHost(endpoint string) string {
	return registryImageHost(endpoint)
}

func BuildRunPageQuery(query *gorm.DB, pagination paginationParams) *gorm.DB {
	return buildRunPageQuery(query, pagination)
}

func BuildRunActorName(user model.User) string          { return buildRunActorName(user) }
func SplitTargetImageRef(value string) (string, string) { return splitTargetImageRef(value) }

func WriteSSE(writer http.ResponseWriter, event, idValue string, data any) {
	writeSSE(writer, event, idValue, data)
}
func FlushSSE(writer http.ResponseWriter) { flushSSE(writer) }

func BuildRunBadRequest(message string) error { return buildRunBadRequest(message) }

func BuildRunPublicConflict(code, message string) error {
	return buildRunPublicConflict(code, message)
}

func WriteBuildRunRequestError(ctx *gin.Context, err error) {
	writeBuildRunRequestError(ctx, err)
}

func NormalizeBuildVariables(ctx *gin.Context, input map[string]string) (map[string]string, bool) {
	return normalizeBuildVariables(ctx, input)
}

func NormalizeBuildArgsInput(ctx *gin.Context, raw string) (string, bool) {
	return normalizeBuildArgsInput(ctx, raw)
}

func IsBuildEnvKey(value string) bool { return isBuildEnvKey(value) }
func EncodeBuildVariableSetIDs(ids []string) string {
	return encodeBuildVariableSetIDs(ids)
}
func BuildVariableSetIDs(raw string) []string { return buildVariableSetIDs(raw) }

func DecodeSecretRefs(raw string) map[string]string { return decodeSecretRefs(raw) }

func (h *Handler) DeploymentBuildEnvironmentFromInput(ctx *gin.Context, user model.User, projectID, targetID string, input DeploymentBuildEnvironmentInput, existing *model.BuildEnvironmentConfig) (*model.BuildEnvironmentConfig, bool) {
	return h.deploymentBuildEnvironmentFromInput(ctx, user, projectID, targetID, input, existing)
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

func (h *Handler) BuildVariableSetResponseForUser(user model.User, set model.BuildVariableSet, ctx context.Context) (BuildVariableSetResponse, error) {
	return h.buildVariableSetResponseForUser(user, set, ctx)
}
