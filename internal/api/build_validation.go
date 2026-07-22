package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/buildenv"
	"github.com/LiteyukiStudio/devops/internal/buildtemplate"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type buildRunRequestError struct {
	status  int
	code    string
	message string
}

func (e buildRunRequestError) Error() string {
	return e.message
}

func buildRunBadRequest(message string) error {
	return buildRunRequestError{status: http.StatusBadRequest, message: message}
}

func buildRunConflict(code string, message string) error {
	return buildRunRequestError{status: http.StatusConflict, code: code, message: message}
}

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func (h *Handlers) validateBuildRunRequest(ctx *gin.Context, user model.User, run *model.BuildRun) bool {
	if err := h.prepareBuildRunRequest(user, run); err != nil {
		var requestErr buildRunRequestError
		if errors.As(err, &requestErr) {
			if requestErr.code != "" {
				writeErrorCode(ctx, requestErr.status, requestErr.code, requestErr.message)
				return false
			}
			writeError(ctx, requestErr.status, requestErr.message)
			return false
		}
		writeError(ctx, http.StatusBadRequest, err.Error())
		return false
	}
	return true
}

func (h *Handlers) prepareBuildRunRequest(user model.User, run *model.BuildRun) error {
	var project model.Project
	if err := h.db.First(&project, "id = ?", run.ProjectID).Error; err != nil {
		return buildRunBadRequest("项目空间不存在")
	}
	var app model.Application
	if err := h.db.First(&app, "id = ? and project_id = ?", run.ApplicationID, run.ProjectID).Error; err != nil {
		return buildRunBadRequest("应用不存在")
	}
	if !applicationCanMutate(app) {
		return buildRunConflict("application.delete_in_progress", "应用正在删除中，不能触发构建")
	}
	config, err := h.deploymentTargetForBuildRun(app, run.DeploymentTargetID)
	if err != nil {
		return buildRunBadRequest("部署配置不存在或不可用")
	}
	run.DeploymentTargetID = config.ID
	if normalizeDeploymentSourceType(config.SourceType) != "repository" {
		return buildRunBadRequest("镜像直部署配置不能触发构建")
	}
	if strings.TrimSpace(run.BuildVariableSetIDs) == "" {
		run.BuildVariableSetIDs = strings.TrimSpace(config.BuildVariableSetIDs)
	}
	run.BuildDefinitionMode = buildtemplate.DefinitionModeRepository
	run.BuildTemplateID = ""
	run.BuildTemplateVersion = ""
	run.BuildTemplateValues = "{}"
	run.BuildTemplateDockerfile = ""
	run.BuildTemplateChecksum = ""
	if strings.TrimSpace(config.BuildDefinitionMode) == buildtemplate.DefinitionModeTemplate {
		definition, ok := buildtemplate.Find(config.BuildTemplateID, config.BuildTemplateVersion)
		if !ok {
			return buildRunBadRequest("部署配置引用的构建模板不存在")
		}
		values, err := buildtemplate.NormalizeValues(definition, config.BuildTemplateValues)
		if err != nil {
			return buildRunBadRequest(err.Error())
		}
		preview, err := buildtemplate.Render(definition.ID, definition.Version, values)
		if err != nil {
			return buildRunBadRequest(err.Error())
		}
		run.BuildDefinitionMode = buildtemplate.DefinitionModeTemplate
		run.BuildTemplateID = preview.TemplateID
		run.BuildTemplateVersion = preview.Version
		run.BuildTemplateValues = buildtemplate.EncodeValues(preview.Values)
		run.BuildTemplateDockerfile = preview.Dockerfile
		run.BuildTemplateChecksum = preview.Checksum
	}
	run.DockerfilePath = fallback(strings.TrimSpace(config.DockerfilePath), "Dockerfile")
	run.BuildContext = fallback(strings.TrimSpace(config.BuildContext), ".")
	run.BuildDirectory = strings.TrimSpace(config.BuildDirectory)
	run.BuildArgs = strings.TrimSpace(config.BuildArgs)
	run.BuildEnvironmentID = strings.TrimSpace(config.BuildEnvironmentID)
	buildCPURequest, err := normalizeBuildResourceQuantityValue(firstNonEmpty(run.BuildCPURequest, config.BuildCPURequest), defaultBuildCPURequest, "构建 CPU")
	if err != nil {
		return buildRunBadRequest(err.Error())
	}
	buildMemoryRequest, err := normalizeBuildResourceQuantityValue(firstNonEmpty(run.BuildMemoryRequest, config.BuildMemoryRequest), defaultBuildMemoryRequest, "构建内存")
	if err != nil {
		return buildRunBadRequest(err.Error())
	}
	run.BuildCPURequest = buildCPURequest
	run.BuildMemoryRequest = buildMemoryRequest
	buildTimeoutSeconds := normalizeBuildTimeoutSecondsValue(firstPositiveInt(run.BuildTimeoutSeconds, config.BuildTimeoutSeconds))
	if buildTimeoutSeconds < minBuildTimeoutSeconds || buildTimeoutSeconds > maxBuildTimeoutSeconds {
		return buildRunBadRequest("构建超时时间必须在 1 分钟到 24 小时之间")
	}
	run.BuildTimeoutSeconds = buildTimeoutSeconds
	run.BuildLabels = strings.Join(normalizeBuildSelectorList(strings.Split(config.BuildLabels, ",")), ",")
	if strings.TrimSpace(config.RepositoryBindingID) != "" {
		var binding model.RepositoryBinding
		if err := h.db.First(&binding, "id = ? and project_id = ? and application_id = ?", config.RepositoryBindingID, run.ProjectID, run.ApplicationID).Error; err != nil {
			return buildRunBadRequest("部署配置绑定的代码仓库不存在")
		}
	} else {
		return buildRunBadRequest("部署配置未绑定代码仓库")
	}
	if strings.TrimSpace(run.TargetRegistryID) == "" {
		run.TargetRegistryID = strings.TrimSpace(config.TargetRegistryID)
	}
	if strings.TrimSpace(run.TargetRegistryID) == "" {
		return buildRunBadRequest("目标镜像站不能为空")
	}
	var registry model.ArtifactRegistry
	if err := h.db.First(&registry, "id = ?", run.TargetRegistryID).Error; err != nil {
		return buildRunBadRequest("目标镜像站不存在")
	}
	credential, hasCredential := h.registryPushCredentialForProject(user, registry, run.ProjectID)
	if strings.TrimSpace(run.TargetRepository) == "" {
		run.TargetRepository = strings.Trim(strings.TrimSpace(config.TargetRepository), "/")
		run.TargetTag = strings.TrimSpace(config.TargetTag)
	}
	if strings.TrimSpace(run.TargetRepository) == "" {
		repositoryRef := buildTargetImageRepository(registry, project, app)
		if hasCredential {
			repositoryRef = buildTargetImageRepositoryForCredential(registry, credential, project, app, config)
			run.TargetTag = buildTargetImageTagTemplateForCredential(credential)
		}
		repository, tag := splitTargetImageRef(repositoryRef)
		run.TargetRepository = repository
		if strings.TrimSpace(run.TargetTag) == "" {
			run.TargetTag = tag
		}
	}
	run.TargetRepository = strings.Trim(strings.TrimSpace(run.TargetRepository), "/")
	run.TargetTag = fallback(strings.TrimSpace(run.TargetTag), "latest")
	run.ImageRef = fallback(strings.TrimSpace(run.ImageRef), buildImageRef(registry, *run))
	if !h.usableRegistryCredentialExists(user.ID, run.ProjectID, registry) {
		return buildRunBadRequest("目标镜像站缺少可用推送凭据")
	}
	if _, err := h.buildVariablesForRunByIDs(h.db, user, run.ProjectID, buildVariableSetIDs(run.BuildVariableSetIDs)); err != nil {
		return buildRunBadRequest(err.Error())
	}
	if strings.TrimSpace(run.BuildVariablesSnapshot) == "" && strings.TrimSpace(run.BuildSecretRefsSnapshot) == "" {
		snapshot, err := h.buildEnvironmentSnapshotForRun(h.db, user, *run)
		if err != nil {
			return buildRunBadRequest("构建变量和密钥不可用")
		}
		run.BuildVariablesSnapshot = buildenv.Encode(snapshot.Variables)
		run.BuildSecretRefsSnapshot = buildenv.Encode(snapshot.SecretRefs)
	}
	return nil
}

func (h *Handlers) deploymentTargetForBuildRun(app model.Application, targetID string) (model.DeploymentTarget, error) {
	var config model.DeploymentTarget
	query := h.db.Where("project_id = ? and application_id = ? and enabled = ?", app.ProjectID, app.ID, true)
	if strings.TrimSpace(targetID) != "" {
		query = query.Where("id = ?", strings.TrimSpace(targetID))
	} else {
		query = query.Order("created_at asc")
	}
	return config, query.First(&config).Error
}

func (h *Handlers) deploymentTargetForRun(ctx *gin.Context, app model.Application, targetID string) (model.DeploymentTarget, bool) {
	config, err := h.deploymentTargetForBuildRun(app, targetID)
	if err != nil {
		writeError(ctx, http.StatusBadRequest, "部署配置不存在或不可用")
		return config, false
	}
	return config, true
}

func (h *Handlers) usableRegistryCredentialExists(userID, projectID string, registry model.ArtifactRegistry) bool {
	visible := func(query *gorm.DB) *gorm.DB {
		return query.Where("scope = ? and owner_ref = ? or scope = ? or (scope = ? and exists (select 1 from scoped_resource_project_bindings srpb where srpb.resource_type = ? and srpb.resource_id = registry_credentials.id and srpb.project_id = ?))",
			"user", userID, "global", "project", scopedResourceRegistryCredential, projectID)
	}
	if strings.TrimSpace(registry.CredentialRef) != "" {
		var count int64
		visible(h.db.Model(&model.RegistryCredential{})).
			Where("registry_id = ? and usage in ?", registry.ID, []string{"push", "push-pull"}).
			Where("id = ?", registry.CredentialRef).
			Count(&count)
		if count > 0 {
			return true
		}
	}
	var count int64
	visible(h.db.Model(&model.RegistryCredential{})).
		Where("registry_id = ? and usage in ?", registry.ID, []string{"push", "push-pull"}).
		Count(&count)
	return count > 0
}
