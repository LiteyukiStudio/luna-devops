package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/resourceidentifier"
	"github.com/LiteyukiStudio/devops/internal/tasks"
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
	h.observeDeploymentTargets(ctx.Request.Context(), project, targets)
	ctx.JSON(http.StatusOK, paginatedResponse(deploymentTargetResponses(targets), total, pagination))
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
	stage := normalizeStage(input.Stage)
	if err := resourceidentifier.Validate(stage, stageIdentifierMinLength, stageIdentifierMaxLength); err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "deployment.stage_invalid", err.Error())
		return
	}
	if !h.ensureDeploymentStageAvailable(ctx, app.ID, stage, "") {
		return
	}
	targetID := id.New("dplt")
	kubernetesName := resourceidentifier.DeploymentTargetName(app.Identifier, stage)
	target, ok := h.deploymentTargetFromInput(ctx, user, app, input, targetID, kubernetesName, nil, "")
	if !ok {
		return
	}
	target = model.ApplyPlatformDeploymentTargetDefaults(project, app, target)
	buildEnvironment, ok := h.deploymentBuildEnvironmentFromInput(ctx, user, project.ID, target.ID, input, nil)
	if !ok {
		return
	}
	if err := h.createDeploymentTarget(target, input.BuildHookBindings, buildEnvironment, ctx.Request.Context()); errors.Is(err, errDeploymentStageExists) {
		writeErrorCode(ctx, http.StatusConflict, "deployment.stage_exists", "deployment stage already exists in this application")
		return
	} else if err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if !h.syncDeploymentTargetDataVolume(ctx, target) {
		return
	}
	target, _ = h.deploymentTargetWithHookBindings(target, ctx.Request.Context())
	target = h.observeDeploymentTarget(ctx.Request.Context(), project, target)
	ctx.JSON(http.StatusCreated, deploymentTargetResponseFromModel(target))
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
	target, ok := h.deploymentTargetFromInput(ctx, user, app, input, existing.ID, existing.KubernetesName, decodeSecretRefs(existing.SecretFiles), existing.RuntimeConfigRefs)
	if !ok {
		return
	}
	target.CreatedBy = existing.CreatedBy
	target.CreatedAt = existing.CreatedAt
	// Stage is immutable and may use an internal sys-* value for platform-managed
	// components. Keep the persisted value authoritative instead of passing it
	// through the public-stage normalizer used by create requests.
	target.Stage = existing.Stage
	if strings.TrimSpace(input.SecretRefs) == "" {
		target.SecretRefs = existing.SecretRefs
	}
	target = model.ApplyPlatformDeploymentTargetDefaults(project, app, target)
	existingBuildEnvironment, err := h.findBuildEnvironmentConfig(h.dbFor(ctx), model.BuildEnvironmentScopeDeployment, target.ID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	buildEnvironment, ok := h.deploymentBuildEnvironmentFromInput(ctx, user, project.ID, target.ID, input, &existingBuildEnvironment)
	if !ok {
		return
	}
	if err := h.saveDeploymentTarget(target, input.BuildHookBindings, buildEnvironment, ctx.Request.Context()); err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if !h.syncDeploymentTargetDataVolume(ctx, target) {
		return
	}
	target, _ = h.deploymentTargetWithHookBindings(target, ctx.Request.Context())
	target = h.observeDeploymentTarget(ctx.Request.Context(), project, target)
	ctx.JSON(http.StatusOK, deploymentTargetResponseFromModel(target))
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

type deploymentTargetDataExportAuthorization struct {
	user    model.User
	project model.Project
	app     model.Application
	target  model.DeploymentTarget
	binding dataExportAuthorizationBinding
}

func (h *Handlers) authorizeDeploymentTargetDataExport(ctx *gin.Context) (deploymentTargetDataExportAuthorization, bool) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, authz.ProjectRoleOwner, authz.ProjectRoleAdmin)
	if !ok {
		return deploymentTargetDataExportAuthorization{}, false
	}
	if !h.ensureProjectCanMutate(ctx, project) {
		return deploymentTargetDataExportAuthorization{}, false
	}
	binding, ok := h.requireDataExportAuthorizationBinding(ctx, user)
	if !ok {
		return deploymentTargetDataExportAuthorization{}, false
	}
	app, ok := h.findApplication(ctx)
	if !ok {
		return deploymentTargetDataExportAuthorization{}, false
	}
	if !applicationCanMutate(app) {
		writeErrorCode(ctx, http.StatusConflict, "application.delete_in_progress", "应用正在删除中，不能导出运行数据")
		return deploymentTargetDataExportAuthorization{}, false
	}
	var target model.DeploymentTarget
	if err := h.dbFor(ctx).First(&target, "id = ? and project_id = ? and application_id = ?", ctx.Param("targetId"), app.ProjectID, app.ID).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "deployment target not found")
		return deploymentTargetDataExportAuthorization{}, false
	}
	if !h.ensureDeploymentTargetCanMutate(ctx, target) {
		return deploymentTargetDataExportAuthorization{}, false
	}
	if !target.DataRetentionEnabled {
		writeError(ctx, http.StatusBadRequest, "该部署配置未启用运行数据保留")
		return deploymentTargetDataExportAuthorization{}, false
	}
	return deploymentTargetDataExportAuthorization{
		user: user, project: project, app: app, target: target, binding: binding,
	}, true
}

func (h *Handlers) AuthorizeDeploymentTargetDataExport(ctx *gin.Context) {
	authorization, ok := h.authorizeDeploymentTargetDataExport(ctx)
	if !ok {
		return
	}
	ticket, expiresAt, err := h.issueDataExportTicket(ctx.Request.Context(), authorization)
	if err != nil {
		h.auditWithContext(authorization.user.ID, "deployment_target.data_export_authorize", authorization.target.ID, false, err.Error(), ctx.Request.Context())
		writeErrorCode(ctx, http.StatusServiceUnavailable, "data_export.ticket_unavailable", "data export authorization is temporarily unavailable")
		return
	}
	ctx.JSON(http.StatusOK, dataExportTicketResponse{Ticket: ticket, ExpiresAt: expiresAt})
}

func (h *Handlers) ExportDeploymentTargetData(ctx *gin.Context) {
	ticket := strings.TrimSpace(ctx.Query("ticket"))
	if ticket == "" {
		writeErrorCode(ctx, http.StatusBadRequest, "data_export.ticket_required", "data export ticket is required")
		return
	}
	ticketValue, valid, err := h.consumeDataExportTicket(ctx.Request.Context(), ticket)
	if err != nil {
		writeErrorCode(ctx, http.StatusServiceUnavailable, "data_export.ticket_unavailable", "data export authorization is temporarily unavailable")
		return
	}
	if !valid || !ticketValue.matchesResource(ctx.Param("projectId"), ctx.Param("applicationId"), ctx.Param("targetId")) {
		writeErrorCode(ctx, http.StatusForbidden, "data_export.ticket_invalid", "data export ticket is invalid, expired, consumed, or bound to another request")
		return
	}
	authorization, ok := h.dataExportAuthorizationFromTicket(ctx.Request.Context(), ticketValue)
	if !ok {
		writeErrorCode(ctx, http.StatusForbidden, "data_export.ticket_invalid", "data export ticket authorization is no longer valid")
		return
	}
	user, project, app, target := authorization.user, authorization.project, authorization.app, authorization.target
	client, namespace, ok := h.kubernetesClientForDeploymentTarget(ctx, project, target, "运行集群不可用，无法导出运行数据")
	if !ok {
		return
	}
	filename := fmt.Sprintf("%s-%s-data.tar.gz", app.Identifier, target.ID)
	requestCtx, cancel := context.WithTimeout(ctx.Request.Context(), 5*time.Minute)
	defer cancel()
	archiveReader, archiveWriter := io.Pipe()
	streamResult := make(chan error, 1)
	go func() {
		err := client.StreamDataArchive(requestCtx, kubeprovider.DataExportSpec{
			Name:      "lyd-export-" + shortResourceID(target.ID),
			Namespace: namespace,
			MountPath: deploymentTargetDataMountPath(target),
			Volumes:   deploymentTargetDataExportVolumes(target),
		}, archiveWriter)
		_ = archiveWriter.CloseWithError(err)
		streamResult <- err
	}()
	defer archiveReader.Close()

	firstChunk := make([]byte, 32*1024)
	readCount, readErr := archiveReader.Read(firstChunk)
	if readCount == 0 && readErr != nil {
		streamErr := <-streamResult
		if streamErr == nil {
			streamErr = readErr
		}
		h.auditWithContext(user.ID, "deployment_target.data_export", target.ID, false, streamErr.Error(), ctx.Request.Context())
		writeErrorCode(ctx, http.StatusBadGateway, "data_export.stream_failed", "runtime data export could not be started")
		return
	}

	ctx.Header("Content-Type", "application/gzip")
	ctx.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	ctx.Header("X-Content-Type-Options", "nosniff")
	ctx.Header("Cache-Control", "no-store")
	ctx.Header("Referrer-Policy", "no-referrer")
	if _, err := ctx.Writer.Write(firstChunk[:readCount]); err != nil {
		_ = archiveReader.CloseWithError(err)
		streamErr := <-streamResult
		if streamErr == nil {
			streamErr = err
		}
		h.auditWithContext(user.ID, "deployment_target.data_export", target.ID, false, streamErr.Error(), ctx.Request.Context())
		return
	}
	_, copyErr := io.Copy(ctx.Writer, archiveReader)
	if copyErr != nil {
		_ = archiveReader.CloseWithError(copyErr)
	}
	streamErr := <-streamResult
	if streamErr == nil {
		streamErr = copyErr
	}
	if streamErr != nil {
		h.auditWithContext(user.ID, "deployment_target.data_export", target.ID, false, streamErr.Error(), ctx.Request.Context())
		return
	}
	h.auditWithContext(user.ID, "deployment_target.data_export", target.ID, true, filename, ctx.Request.Context())
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
	if err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
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
		_ = markResourceDeleteFailed(h.dbFor(ctx), &model.DeploymentTarget{}, target.ID, "资源清理任务投递失败，请稍后重试")
		_ = markDeploymentTargetGatewayRoutesDeleteFailed(h.dbFor(ctx), target, "资源清理任务投递失败，请稍后重试")
		writeError(ctx, http.StatusServiceUnavailable, "资源清理任务投递失败，请稍后重试")
		return
	}
	ctx.Status(http.StatusNoContent)
}
