package deploymentapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/repository"
	"github.com/gin-gonic/gin"
)

func (h *Handlers) runtimeConfigRefsFromInput(ctx *gin.Context, projectID string, input deploymentTargetInput, existingRaw string) ([]model.DeploymentRuntimeConfigRef, bool) {
	refs := runtimeConfigRefInputs(input)
	if len(refs) == 0 {
		return nil, true
	}
	existingSnapshots := map[string]*model.DeploymentRuntimeConfigSnapshot{}
	for _, ref := range model.DecodeDeploymentRuntimeConfigRefs(existingRaw) {
		if ref.Mode == model.RuntimeConfigRefModeSnapshot && ref.Snapshot != nil {
			snapshot := *ref.Snapshot
			existingSnapshots[ref.SetID] = &snapshot
		}
	}
	setIDs := make([]string, 0, len(refs))
	for _, ref := range refs {
		mode := model.RuntimeConfigRefMode(ref.Mode)
		if mode == model.RuntimeConfigRefModeSnapshot && existingSnapshots[ref.SetID] != nil {
			continue
		}
		setIDs = append(setIDs, ref.SetID)
	}
	var sets []model.ProjectRuntimeConfigSet
	if len(setIDs) > 0 {
		if err := h.dbFor(ctx).Where("project_id = ? and id in ?", projectID, setIDs).Find(&sets).Error; err != nil {
			writeErrorCode(ctx, http.StatusInternalServerError, "deployment.runtime_config_unavailable", "runtime configuration is unavailable")
			return nil, false
		}
	}
	setsByID := make(map[string]model.ProjectRuntimeConfigSet, len(sets))
	for _, set := range sets {
		setsByID[set.ID] = set
	}
	if len(setsByID) != len(setIDs) {
		writeError(ctx, http.StatusBadRequest, "运行配置集不存在或不属于当前项目空间")
		return nil, false
	}
	now := time.Now()
	output := make([]model.DeploymentRuntimeConfigRef, 0, len(refs))
	for _, ref := range refs {
		mode := model.RuntimeConfigRefMode(ref.Mode)
		next := model.DeploymentRuntimeConfigRef{SetID: ref.SetID, Mode: mode}
		if mode == model.RuntimeConfigRefModeSnapshot {
			if snapshot := existingSnapshots[ref.SetID]; snapshot != nil {
				next.Snapshot = snapshot
			} else {
				snapshot := model.ProjectRuntimeConfigSetSnapshot(setsByID[ref.SetID], now)
				next.Snapshot = &snapshot
			}
		}
		output = append(output, next)
	}
	return output, true
}

func runtimeConfigRefInputs(input deploymentTargetInput) []deploymentRuntimeConfigRefInput {
	refs := make([]deploymentRuntimeConfigRefInput, 0)
	if len(input.RuntimeConfigRefs) > 0 {
		seen := map[string]bool{}
		for _, ref := range input.RuntimeConfigRefs {
			setID := strings.TrimSpace(ref.SetID)
			if setID == "" || seen[setID] {
				continue
			}
			seen[setID] = true
			refs = append(refs, deploymentRuntimeConfigRefInput{
				SetID: setID,
				Mode:  model.RuntimeConfigRefMode(ref.Mode),
			})
		}
	}
	return refs
}

func (h *Handlers) applyRegistryCredentialImageTemplate(ctx *gin.Context, user model.User, app model.Application, sourceType string, registryID string, repository string, tag string, target model.DeploymentTarget) (string, string, bool) {
	if sourceType != "repository" || strings.TrimSpace(registryID) == "" {
		return repository, tag, true
	}
	var project model.Project
	if err := h.dbFor(ctx).First(&project, "id = ?", app.ProjectID).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, "项目空间不存在")
		return repository, tag, false
	}
	var registry model.ArtifactRegistry
	if err := h.dbFor(ctx).First(&registry, "id = ?", registryID).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, "目标镜像站不存在")
		return repository, tag, false
	}
	credential, ok := h.registryPushCredentialForProject(user, registry, app.ProjectID, ctx.Request.Context())
	if !ok {
		return repository, tag, true
	}
	if strings.TrimSpace(repository) == "" || isDefaultImageRepository(registry, project, app, repository) {
		repository, _ = splitTargetImageRef(buildTargetImageRepositoryForCredential(registry, credential, project, app, target))
		repository = repositoryWithoutRegistryHost(registry, repository)
	}
	if strings.TrimSpace(tag) == "" || (strings.TrimSpace(tag) == "latest" && strings.TrimSpace(credential.TagTemplate) != "") {
		tag = buildStaticTargetImageTagForCredential(registry, credential, project, app, target)
	}
	return strings.Trim(strings.TrimSpace(repository), "/"), strings.TrimSpace(tag), true
}

// ensureTargetImageRepositoryAvailable 拒绝把构建目标指向其它项目空间已使用的镜像仓库。
func (h *Handlers) ensureTargetImageRepositoryAvailable(ctx *gin.Context, projectID, registryID, targetRepository string) bool {
	if strings.TrimSpace(registryID) == "" || strings.TrimSpace(targetRepository) == "" {
		return true
	}
	var registry model.ArtifactRegistry
	if err := h.dbFor(ctx).First(&registry, "id = ?", registryID).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, "目标镜像站不存在")
		return false
	}
	claimed, err := repository.ImageRepositoryClaimedByOtherProject(ctx.Request.Context(), h.dbFor(ctx), projectID, registry, targetRepository)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, "目标镜像仓库占用检查失败")
		return false
	}
	if claimed {
		writeErrorCode(ctx, http.StatusConflict, "deployment_target.image_repository_claimed", "目标镜像仓库已被其它项目空间使用")
		return false
	}
	return true
}

// ensureTargetHasNoGatewayRoutes 拒绝把仍有访问入口的部署配置设为内部服务：内部服务不接受网关流量，
// 已有访问入口会全部失效。需要先删除访问入口。
func (h *Handlers) ensureTargetHasNoGatewayRoutes(ctx *gin.Context, targetID string) bool {
	var count int64
	if err := h.dbFor(ctx).Model(&model.GatewayRoute{}).Where("deployment_target_id = ? and delete_status = ?", targetID, "active").Count(&count).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, "gateway route lookup failed")
		return false
	}
	if count > 0 {
		writeErrorCode(ctx, http.StatusConflict, "deployment_target.internal_has_routes", "remove the gateway routes of this deployment target before making it internal")
		return false
	}
	return true
}
