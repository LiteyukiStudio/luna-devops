package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/resourceidentifier"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/LiteyukiStudio/devops/internal/volume"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

func (h *Handlers) ListDeploymentTargets(ctx *gin.Context) {
	markLiveObservationResponse(ctx)
	_, project, ok := h.projectAndCurrentUserWithRoles(ctx, authz.ProjectRoleOwner, authz.ProjectRoleAdmin, authz.ProjectRoleDeveloper, authz.ProjectRoleViewer)
	if !ok {
		return
	}
	app, ok := h.findApplication(ctx)
	if !ok {
		return
	}
	var targets []model.DeploymentTarget
	query := h.dbFor(ctx).Model(&model.DeploymentTarget{}).Where("project_id = ? and application_id = ?", app.ProjectID, app.ID)
	query = applySearch(ctx, query, "name", "source_branch", "image_repository", "image_tag")
	pagination := paginationFromQueryWithSort(ctx, map[string]string{"name": "name", "createdAt": "created_at"}, "createdAt")
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if err := deploymentTargetPageQuery(query, pagination).Find(&targets).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.attachDeploymentTargetHookBindings(targets, ctx.Request.Context()); err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	mountsByTarget, err := h.deploymentTargetVolumeMountsByTarget(ctx.Request.Context(), targets)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.observeDeploymentTargets(ctx.Request.Context(), project, targets)
	ctx.JSON(http.StatusOK, paginatedResponse(deploymentTargetResponses(targets, mountsByTarget), total, pagination))
}

func deploymentTargetPageQuery(query *gorm.DB, pagination paginationParams) *gorm.DB {
	return query.Order(orderByClause(pagination, map[string]string{
		"name":      "name",
		"createdAt": "created_at",
	}, "created_at")).Limit(pagination.PageSize).Offset(pagination.Offset())
}

func (h *Handlers) CreateDeploymentTarget(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, authz.ProjectRoleOwner, authz.ProjectRoleAdmin, authz.ProjectRoleDeveloper)
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
	stage, validStage := normalizePublicStage(input.Stage)
	if !validStage {
		writeDeploymentStageInvalid(ctx, "stage", "deployment stage must be dev, test, staging, or prod")
		return
	}
	if err := resourceidentifier.Validate(stage, stageIdentifierMinLength, stageIdentifierMaxLength); err != nil {
		writeDeploymentStageInvalid(ctx, "stage", err.Error())
		return
	}
	if !h.ensureDeploymentStageAvailable(ctx, app.ID, stage, "") {
		return
	}
	targetID := id.New("dplt")
	kubernetesName := resourceidentifier.DeploymentTargetName(app.Identifier, stage)
	target, dataVolumes, ok := h.deploymentTargetFromInput(ctx, user, app, input, targetID, kubernetesName, nil, "")
	if !ok {
		return
	}
	buildEnvironment, ok := h.deploymentBuildEnvironmentFromInput(ctx, user, project.ID, target.ID, input, nil)
	if !ok {
		return
	}
	changes, err := h.createDeploymentTarget(target, dataVolumes, input.BuildHookBindings, buildEnvironment, ctx.Request.Context())
	if errors.Is(err, errDeploymentStageExists) {
		writeErrorCode(ctx, http.StatusConflict, "deployment.stage_exists", "deployment stage already exists in this application")
		return
	} else if err != nil {
		h.auditDeploymentVolumeMountFailure(ctx.Request.Context(), user.ID, changes, err)
		if volume.ErrorCode(err) != "" {
			writeVolumeError(ctx, err)
		} else {
			writeError(ctx, http.StatusBadRequest, err.Error())
		}
		return
	}
	h.auditDeploymentVolumeMountChanges(ctx.Request.Context(), user.ID, target, changes)
	target, _ = h.deploymentTargetWithHookBindings(target, ctx.Request.Context())
	target = h.observeDeploymentTarget(ctx.Request.Context(), project, target)
	mountsByTarget, err := h.deploymentTargetVolumeMountsByTarget(ctx.Request.Context(), []model.DeploymentTarget{target})
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusCreated, deploymentTargetResponseFromModel(target, mountsByTarget[target.ID]))
}

func (h *Handlers) UpdateDeploymentTarget(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, authz.ProjectRoleOwner, authz.ProjectRoleAdmin, authz.ProjectRoleDeveloper)
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
	if err := h.dbFor(ctx).First(&existing, "id = ? and project_id = ? and application_id = ?", ctx.Param("targetId"), app.ProjectID, app.ID).Error; err != nil {
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
	if strings.TrimSpace(input.Stage) != strings.TrimSpace(existing.Stage) {
		writeErrorCode(ctx, http.StatusConflict, "deployment.stage_immutable", "deployment stage cannot be changed")
		return
	}
	target, dataVolumes, ok := h.deploymentTargetFromInput(ctx, user, app, input, existing.ID, existing.KubernetesName, decodeSecretRefs(existing.SecretFiles), existing.RuntimeConfigRefs)
	if !ok {
		return
	}
	target.CreatedBy = existing.CreatedBy
	target.CreatedAt = existing.CreatedAt
	// Stage is immutable and may use an internal sys-* value for platform-managed
	// components. Keep the persisted value authoritative instead of passing it
	// through the public-stage normalizer used by create requests.
	target.Stage = existing.Stage
	target.SecretRefs = existing.SecretRefs
	existingBuildEnvironment, err := h.findBuildEnvironmentConfig(h.dbFor(ctx), model.BuildEnvironmentScopeDeployment, target.ID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	buildEnvironment, ok := h.deploymentBuildEnvironmentFromInput(ctx, user, project.ID, target.ID, input, &existingBuildEnvironment)
	if !ok {
		return
	}
	changes, err := h.saveDeploymentTarget(target, dataVolumes, input.BuildHookBindings, buildEnvironment, ctx.Request.Context())
	if err != nil {
		h.auditDeploymentVolumeMountFailure(ctx.Request.Context(), user.ID, changes, err)
		if volume.ErrorCode(err) != "" {
			writeVolumeError(ctx, err)
		} else {
			writeError(ctx, http.StatusBadRequest, err.Error())
		}
		return
	}
	h.auditDeploymentVolumeMountChanges(ctx.Request.Context(), user.ID, target, changes)
	target, _ = h.deploymentTargetWithHookBindings(target, ctx.Request.Context())
	target = h.observeDeploymentTarget(ctx.Request.Context(), project, target)
	mountsByTarget, err := h.deploymentTargetVolumeMountsByTarget(ctx.Request.Context(), []model.DeploymentTarget{target})
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, deploymentTargetResponseFromModel(target, mountsByTarget[target.ID]))
}

func (h *Handlers) ensureDeploymentStageAvailable(ctx *gin.Context, applicationID, stage, excludeTargetID string) bool {
	query := h.dbFor(ctx).Select("id", "delete_status").
		Where("application_id = ? and stage = ?", applicationID, stage)
	if strings.TrimSpace(excludeTargetID) != "" {
		query = query.Where("id <> ?", excludeTargetID)
	}
	var existing model.DeploymentTarget
	if err := query.First(&existing).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return true
	} else if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return false
	}
	writeDeploymentStageConflict(ctx, existing.DeleteStatus)
	return false
}

func writeDeploymentStageConflict(ctx *gin.Context, deleteStatus string) {
	switch strings.TrimSpace(deleteStatus) {
	case "deleting":
		writeErrorCode(ctx, http.StatusConflict, "deployment.stage_delete_in_progress", "同阶段部署配置正在删除，资源清理完成后才能复用")
	case "delete_failed":
		writeErrorCode(ctx, http.StatusConflict, "deployment.stage_delete_failed", "同阶段部署配置上次删除失败，请先完成资源清理")
	default:
		writeErrorCode(ctx, http.StatusConflict, "deployment.stage_exists", "deployment stage already exists in this application")
	}
}

func requireInteractiveSession(ctx *gin.Context) bool {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(ctx.GetHeader("Authorization"))), "bearer ") {
		writeErrorCode(ctx, http.StatusForbidden, "auth.interactive_session_required", "该操作需要使用交互式登录会话")
		return false
	}
	if _, err := ctx.Cookie(sessionCookieName); err != nil {
		writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.session.missing")
		return false
	}
	return true
}

func (h *Handlers) RestartDeploymentTarget(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, authz.ProjectRoleOwner, authz.ProjectRoleAdmin, authz.ProjectRoleDeveloper)
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
	if err := h.dbFor(ctx).First(&target, "id = ? and project_id = ? and application_id = ?", ctx.Param("targetId"), app.ProjectID, app.ID).Error; err != nil {
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
		h.auditWithContext(user.ID, "deployment_target.restart", target.ID, false, err.Error(), ctx.Request.Context())
		if apierrors.IsNotFound(err) {
			writeError(ctx, http.StatusNotFound, "运行 Deployment 不存在，请先完成一次部署")
			return
		}
		writeError(ctx, http.StatusBadGateway, "部署重启失败，请检查运行集群状态")
		return
	}
	h.auditWithContext(user.ID, "deployment_target.restart", target.ID, true, resourceName, ctx.Request.Context())
	ctx.Status(http.StatusNoContent)
}

func (h *Handlers) DeleteDeploymentTarget(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, authz.ProjectRoleOwner, authz.ProjectRoleAdmin, authz.ProjectRoleDeveloper)
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
	if err := h.dbFor(ctx).First(&target, "id = ? and project_id = ? and application_id = ?", ctx.Param("targetId"), app.ProjectID, app.ID).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "deployment target not found")
		return
	}
	if !deleteStatusCanStart(target.DeleteStatus) {
		writeError(ctx, http.StatusConflict, "部署配置正在删除中，请等待资源清理完成")
		return
	}
	if !h.ensureNoIncomingServiceBindings(ctx, target.ProjectID, target.ApplicationID, target.ID) {
		return
	}
	volumeChanges := deploymentVolumeMountChanges{}
	if err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		if err := markResourceDeleting(tx, &model.DeploymentTarget{}, target.ID); err != nil {
			return err
		}
		if err := markDeploymentTargetGatewayRoutesDeleting(tx, target); err != nil {
			return err
		}
		var syncErr error
		volumeChanges, syncErr = syncDeploymentTargetVolumeMounts(ctx.Request.Context(), tx, target, nil)
		return syncErr
	}); err != nil {
		h.auditDeploymentVolumeMountFailure(ctx.Request.Context(), user.ID, volumeChanges, err)
		if volume.ErrorCode(err) != "" {
			writeVolumeError(ctx, err)
		} else {
			writeError(ctx, http.StatusInternalServerError, err.Error())
		}
		return
	}
	h.auditDeploymentVolumeMountChanges(ctx.Request.Context(), user.ID, target, volumeChanges)
	if !h.enqueueResourceCleanup(ctx.Request.Context(), tasks.ResourceCleanupPayload{
		ResourceType: "deployment_target",
		ResourceID:   target.ID,
		ProjectID:    target.ProjectID,
	}) {
		_ = markResourceDeleteFailed(h.dbFor(ctx), &model.DeploymentTarget{}, target.ID, "资源清理任务投递失败，请稍后重试")
		_ = markDeploymentTargetGatewayRoutesDeleteFailed(h.dbFor(ctx), target, "资源清理任务投递失败，请稍后重试")
		writeError(ctx, http.StatusServiceUnavailable, "资源清理任务投递失败，请稍后重试")
		return
	}
	ctx.Status(http.StatusNoContent)
}
