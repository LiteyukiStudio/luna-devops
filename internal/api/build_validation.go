package api

import (
	"errors"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"net/http"
	"strings"
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
	run.DockerfilePath = fallback(strings.TrimSpace(config.DockerfilePath), "Dockerfile")
	run.BuildContext = fallback(strings.TrimSpace(config.BuildContext), ".")
	run.BuildDirectory = strings.TrimSpace(config.BuildDirectory)
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
	credential, hasCredential := h.registryPushCredentialFor(user, registry)
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
	if !h.usableRegistryCredentialExists(user.ID, registry) {
		return buildRunBadRequest("目标镜像站缺少可用推送凭据")
	}
	if _, err := h.buildVariablesForRunByIDs(h.db, user, run.ProjectID, buildVariableSetIDs(run.BuildVariableSetIDs)); err != nil {
		return buildRunBadRequest(err.Error())
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

func (h *Handlers) usableRegistryCredentialExists(userID string, registry model.ArtifactRegistry) bool {
	if strings.TrimSpace(registry.CredentialRef) != "" {
		var count int64
		h.db.Model(&model.RegistryCredential{}).
			Where("registry_id = ? and scope in ?", registry.ID, []string{"push", "push-pull"}).
			Where("id = ? and (access_scope = ? or created_by = ?)", registry.CredentialRef, "registry", userID).
			Count(&count)
		if count > 0 {
			return true
		}
	}
	var count int64
	h.db.Model(&model.RegistryCredential{}).
		Where("registry_id = ? and scope in ?", registry.ID, []string{"push", "push-pull"}).
		Where("(access_scope = ? and created_by = ?) or access_scope = ?", "personal", userID, "registry").
		Count(&count)
	return count > 0
}
