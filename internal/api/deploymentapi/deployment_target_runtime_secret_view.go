package deploymentapi

import (
	"net/http"
	"sort"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
)

type deploymentTargetRuntimeSecretsSummary struct {
	EnvironmentVariables []runtimeEnvironmentVariableResponse `json:"environmentVariables"`
}

func (h *Handlers) GetDeploymentTargetRuntimeSecretsSummary(ctx *gin.Context) {
	setRuntimeSecretNoStoreHeaders(ctx)
	_, project, ok := h.authorizeProject(ctx, authz.ActionSecretReadSummary)
	if !ok || !h.ensureRuntimeSecretProjectReadable(ctx, project) {
		return
	}
	app, ok := h.findApplication(ctx)
	if !ok || !h.ensureRuntimeSecretApplicationReadable(ctx, app) {
		return
	}
	target, ok := h.findRuntimeSecretTarget(ctx, project, app)
	if !ok || !h.ensureRuntimeSecretTargetReadable(ctx, target) {
		return
	}

	keys := runtimeSecretKeys(target.SecretRefs)
	ctx.JSON(http.StatusOK, deploymentTargetRuntimeSecretsSummary{EnvironmentVariables: secretEnvironmentVariables(keys)})
}

func setRuntimeSecretNoStoreHeaders(ctx *gin.Context) {
	ctx.Header("Cache-Control", "no-store, private")
	ctx.Header("Pragma", "no-cache")
	ctx.Header("X-Content-Type-Options", "nosniff")
}

func runtimeSecretKeys(raw string) []string {
	refs := decodeSecretRefs(raw)
	keys := make([]string, 0, len(refs))
	for key, ref := range refs {
		if isBuildEnvKey(key) && strings.TrimSpace(ref) != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func (h *Handlers) findRuntimeSecretTarget(ctx *gin.Context, project model.Project, app model.Application) (model.DeploymentTarget, bool) {
	var target model.DeploymentTarget
	if err := h.dbFor(ctx).First(&target, "id = ? and project_id = ? and application_id = ?", ctx.Param("targetId"), project.ID, app.ID).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "deployment target not found")
		return target, false
	}
	return target, true
}

func (h *Handlers) ensureRuntimeSecretProjectReadable(ctx *gin.Context, project model.Project) bool {
	if h.host.ResourceCanMutateDuringDelete(project.DeleteStatus) {
		return true
	}
	writeErrorCode(ctx, http.StatusConflict, "project.delete_in_progress", "项目空间正在删除中，请等待资源清理完成")
	return false
}

func (h *Handlers) ensureRuntimeSecretApplicationReadable(ctx *gin.Context, app model.Application) bool {
	if h.host.ResourceCanMutateDuringDelete(app.DeleteStatus) {
		return true
	}
	writeErrorCode(ctx, http.StatusConflict, "application.delete_in_progress", "应用正在删除中，请等待资源清理完成")
	return false
}

func (h *Handlers) ensureRuntimeSecretTargetReadable(ctx *gin.Context, target model.DeploymentTarget) bool {
	if h.host.ResourceCanMutateDuringDelete(target.DeleteStatus) {
		return true
	}
	writeErrorCode(ctx, http.StatusConflict, "deployment_target.delete_in_progress", "部署配置正在删除中，请等待资源清理完成")
	return false
}

func runtimeSecretAuditResource(project model.Project, app model.Application, target model.DeploymentTarget, key string) string {
	return strings.Join([]string{project.ID, app.ID, target.ID, strings.TrimSpace(key)}, "/")
}
