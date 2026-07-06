package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

func (h *Handlers) ListDeploymentTargets(ctx *gin.Context) {
	if _, _, ok := h.projectAndCurrentUserWithRoles(ctx, "owner", "admin", "developer", "viewer"); !ok {
		return
	}
	app, ok := h.findApplication(ctx)
	if !ok {
		return
	}
	var targets []model.DeploymentTarget
	query := h.db.Model(&model.DeploymentTarget{}).Where("project_id = ? and application_id = ?", app.ProjectID, app.ID)
	query = applySearch(ctx, query, "name", "source_branch", "image_repository", "image_tag")
	if paginationRequested(ctx) {
		pagination := paginationFromQuery(ctx)
		var total int64
		if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		}
		if err := query.Order(orderByClause(pagination, map[string]string{
			"name":      "name",
			"createdAt": "created_at",
		}, "created_at")).Limit(pagination.PageSize).Offset(pagination.Offset()).Find(&targets).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		}
		if err := h.attachDeploymentTargetHookBindings(targets); err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		}
		ctx.JSON(http.StatusOK, paginatedResponse(deploymentTargetResponses(targets), total, pagination))
		return
	}
	if err := query.Order("created_at asc").Find(&targets).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.attachDeploymentTargetHookBindings(targets); err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, deploymentTargetResponses(targets))
}

func (h *Handlers) CreateDeploymentTarget(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, "owner", "admin", "developer")
	if !ok {
		return
	}
	if !h.ensureProjectCanMutate(ctx, project) {
		return
	}
	if !h.requireStepUp(ctx, user, stepUpPurposeDataExport) {
		return
	}
	app, ok := h.findApplication(ctx)
	if !ok {
		return
	}
	if !applicationCanMutate(app) {
		writeErrorCode(ctx, http.StatusConflict, "application.delete_in_progress", "应用正在删除中，不能新增部署配置")
		return
	}
	var input deploymentTargetInput
	if !bindJSON(ctx, &input) {
		return
	}
	if !h.ensureBillingAllowsDeployChange(ctx, project.ID) {
		return
	}
	input.Enabled = true
	target, ok := h.deploymentTargetFromInput(ctx, user, app, input, id.New("dplt"), nil, "")
	if !ok {
		return
	}
	target = model.ApplyPlatformDeploymentTargetDefaults(project, app, target)
	if err := h.saveDeploymentTarget(target, input.BuildHookBindings); err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if !h.syncDeploymentTargetDataVolume(ctx, target) {
		return
	}
	target, _ = h.deploymentTargetWithHookBindings(target)
	ctx.JSON(http.StatusCreated, deploymentTargetResponseFromModel(target))
}

func (h *Handlers) UpdateDeploymentTarget(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, "owner", "admin", "developer")
	if !ok {
		return
	}
	if !h.ensureProjectCanMutate(ctx, project) {
		return
	}
	app, ok := h.findApplication(ctx)
	if !ok {
		return
	}
	if !applicationCanMutate(app) {
		writeErrorCode(ctx, http.StatusConflict, "application.delete_in_progress", "应用正在删除中，不能修改部署配置")
		return
	}
	var existing model.DeploymentTarget
	if err := h.db.First(&existing, "id = ? and project_id = ? and application_id = ?", ctx.Param("targetId"), app.ProjectID, app.ID).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "deployment target not found")
		return
	}
	if !h.ensureDeploymentTargetCanMutate(ctx, existing) {
		return
	}
	var input deploymentTargetInput
	if !bindJSON(ctx, &input) {
		return
	}
	if !h.ensureBillingAllowsDeployChange(ctx, project.ID) {
		return
	}
	target, ok := h.deploymentTargetFromInput(ctx, user, app, input, existing.ID, decodeSecretRefs(existing.SecretFiles), existing.RuntimeConfigRefs)
	if !ok {
		return
	}
	target.CreatedBy = existing.CreatedBy
	target.CreatedAt = existing.CreatedAt
	if strings.TrimSpace(input.SecretRefs) == "" {
		target.SecretRefs = existing.SecretRefs
	}
	target = model.ApplyPlatformDeploymentTargetDefaults(project, app, target)
	if err := h.saveDeploymentTarget(target, input.BuildHookBindings); err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if !h.syncDeploymentTargetDataVolume(ctx, target) {
		return
	}
	target, _ = h.deploymentTargetWithHookBindings(target)
	ctx.JSON(http.StatusOK, deploymentTargetResponseFromModel(target))
}

func (h *Handlers) ExportDeploymentTargetData(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, "owner", "admin")
	if !ok {
		return
	}
	if !h.ensureProjectCanMutate(ctx, project) {
		return
	}
	app, ok := h.findApplication(ctx)
	if !ok {
		return
	}
	if !applicationCanMutate(app) {
		writeErrorCode(ctx, http.StatusConflict, "application.delete_in_progress", "应用正在删除中，不能删除部署配置")
		return
	}
	var target model.DeploymentTarget
	if err := h.db.First(&target, "id = ? and project_id = ? and application_id = ?", ctx.Param("targetId"), app.ProjectID, app.ID).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "deployment target not found")
		return
	}
	if !h.ensureDeploymentTargetCanMutate(ctx, target) {
		return
	}
	if !target.DataRetentionEnabled {
		writeError(ctx, http.StatusBadRequest, "该部署配置未启用运行数据保留")
		return
	}
	client, namespace, ok := h.kubernetesClientForDeploymentTarget(ctx, project, target, "运行集群不可用，无法导出运行数据")
	if !ok {
		return
	}
	filename := fmt.Sprintf("%s-%s-data.tar.gz", app.Slug, target.ID)
	ctx.Header("Content-Type", "application/gzip")
	ctx.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	ctx.Header("X-Content-Type-Options", "nosniff")
	requestCtx, cancel := context.WithTimeout(ctx.Request.Context(), 5*time.Minute)
	defer cancel()
	if err := client.StreamDataArchive(requestCtx, kubeprovider.DataExportSpec{
		Name:      "lyd-export-" + shortResourceID(target.ID),
		Namespace: namespace,
		MountPath: deploymentTargetDataMountPath(target),
		Volumes:   deploymentTargetDataExportVolumes(target),
	}, ctx.Writer); err != nil {
		h.audit(user.ID, "deployment_target.data_export", target.ID, false, err.Error())
		return
	}
	h.audit(user.ID, "deployment_target.data_export", target.ID, true, filename)
}

func (h *Handlers) RestartDeploymentTarget(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, "owner", "admin", "developer")
	if !ok {
		return
	}
	if !h.ensureProjectCanMutate(ctx, project) {
		return
	}
	app, ok := h.findApplication(ctx)
	if !ok {
		return
	}
	if !applicationCanMutate(app) {
		writeErrorCode(ctx, http.StatusConflict, "application.delete_in_progress", "应用正在删除中，不能重启部署")
		return
	}
	var target model.DeploymentTarget
	if err := h.db.First(&target, "id = ? and project_id = ? and application_id = ?", ctx.Param("targetId"), app.ProjectID, app.ID).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "deployment target not found")
		return
	}
	if !h.ensureDeploymentTargetCanMutate(ctx, target) {
		return
	}
	client, namespace, ok := h.kubernetesClientForDeploymentTarget(ctx, project, target, "运行集群不可用，无法重启部署")
	if !ok {
		return
	}
	requestCtx, cancel := context.WithTimeout(ctx.Request.Context(), 10*time.Second)
	defer cancel()
	resourceName := deploymentTargetResourceName(target)
	if err := client.RestartDeployment(requestCtx, namespace, resourceName); err != nil {
		h.audit(user.ID, "deployment_target.restart", target.ID, false, err.Error())
		if apierrors.IsNotFound(err) {
			writeError(ctx, http.StatusNotFound, "运行 Deployment 不存在，请先完成一次部署")
			return
		}
		writeError(ctx, http.StatusBadGateway, "部署重启失败，请检查运行集群状态")
		return
	}
	h.audit(user.ID, "deployment_target.restart", target.ID, true, resourceName)
	ctx.Status(http.StatusNoContent)
}

func (h *Handlers) DeleteDeploymentTarget(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, "owner", "admin", "developer")
	if !ok {
		return
	}
	if !h.ensureProjectCanMutate(ctx, project) {
		return
	}
	app, ok := h.findApplication(ctx)
	if !ok {
		return
	}
	var target model.DeploymentTarget
	if err := h.db.First(&target, "id = ? and project_id = ? and application_id = ?", ctx.Param("targetId"), app.ProjectID, app.ID).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "deployment target not found")
		return
	}
	if !deleteStatusCanStart(target.DeleteStatus) {
		writeError(ctx, http.StatusConflict, "部署配置正在删除中，请等待资源清理完成")
		return
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := markResourceDeleting(tx, &model.DeploymentTarget{}, target.ID); err != nil {
			return err
		}
		return markDeploymentTargetGatewayRoutesDeleting(tx, target)
	}); err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if !h.enqueueResourceCleanup(ctx.Request.Context(), tasks.ResourceCleanupPayload{
		ResourceType: "deployment_target",
		ResourceID:   target.ID,
		ProjectID:    target.ProjectID,
		ActorID:      user.ID,
		DeleteData:   !target.DataRetentionEnabled,
	}) {
		_ = markResourceDeleteFailed(h.db, &model.DeploymentTarget{}, target.ID, "资源清理任务投递失败，请稍后重试")
		_ = markDeploymentTargetGatewayRoutesDeleteFailed(h.db, target, "资源清理任务投递失败，请稍后重试")
		writeError(ctx, http.StatusServiceUnavailable, "资源清理任务投递失败，请稍后重试")
		return
	}
	ctx.Status(http.StatusNoContent)
}
