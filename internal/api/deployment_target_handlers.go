package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

const (
	dataExportTicketTTL       = time.Minute
	dataExportTicketKeyPrefix = "data_export:ticket:"
)

var dataExportMemoryTickets sync.Map

type dataExportTicketValue struct {
	UserID        string    `json:"userId"`
	SessionID     string    `json:"sessionId"`
	ProjectID     string    `json:"projectId"`
	ApplicationID string    `json:"applicationId"`
	TargetID      string    `json:"targetId"`
	ExpiresAt     time.Time `json:"expiresAt"`
}

type dataExportTicketResponse struct {
	Ticket    string    `json:"ticket"`
	ExpiresAt time.Time `json:"expiresAt"`
}

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
	buildEnvironment, ok := h.deploymentBuildEnvironmentFromInput(ctx, user, project.ID, target.ID, input, nil)
	if !ok {
		return
	}
	if err := h.saveDeploymentTarget(target, input.BuildHookBindings, buildEnvironment); err != nil {
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
	existingBuildEnvironment, err := h.findBuildEnvironmentConfig(h.db, model.BuildEnvironmentScopeDeployment, target.ID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	buildEnvironment, ok := h.deploymentBuildEnvironmentFromInput(ctx, user, project.ID, target.ID, input, &existingBuildEnvironment)
	if !ok {
		return
	}
	if err := h.saveDeploymentTarget(target, input.BuildHookBindings, buildEnvironment); err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if !h.syncDeploymentTargetDataVolume(ctx, target) {
		return
	}
	target, _ = h.deploymentTargetWithHookBindings(target)
	ctx.JSON(http.StatusOK, deploymentTargetResponseFromModel(target))
}

type deploymentTargetDataExportAuthorization struct {
	user    model.User
	project model.Project
	app     model.Application
	target  model.DeploymentTarget
}

func (h *Handlers) authorizeDeploymentTargetDataExport(ctx *gin.Context) (deploymentTargetDataExportAuthorization, bool) {
	if !requireInteractiveSession(ctx) {
		return deploymentTargetDataExportAuthorization{}, false
	}
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, "owner", "admin")
	if !ok {
		return deploymentTargetDataExportAuthorization{}, false
	}
	if !h.ensureProjectCanMutate(ctx, project) {
		return deploymentTargetDataExportAuthorization{}, false
	}
	if !h.requireStepUp(ctx, user, stepUpPurposeDataExport) {
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
	if err := h.db.First(&target, "id = ? and project_id = ? and application_id = ?", ctx.Param("targetId"), app.ProjectID, app.ID).Error; err != nil {
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
	return deploymentTargetDataExportAuthorization{user: user, project: project, app: app, target: target}, true
}

func (h *Handlers) AuthorizeDeploymentTargetDataExport(ctx *gin.Context) {
	authorization, ok := h.authorizeDeploymentTargetDataExport(ctx)
	if !ok {
		return
	}
	session, ok := h.currentSessionFromCookie(ctx)
	if !ok || session.UserID != authorization.user.ID {
		writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.session.expired")
		return
	}
	ticket, expiresAt, err := h.issueDataExportTicket(ctx.Request.Context(), authorization, session)
	if err != nil {
		h.audit(authorization.user.ID, "deployment_target.data_export_authorize", authorization.target.ID, false, err.Error())
		writeErrorCode(ctx, http.StatusServiceUnavailable, "data_export.ticket_unavailable", "data export authorization is temporarily unavailable")
		return
	}
	ctx.JSON(http.StatusOK, dataExportTicketResponse{Ticket: ticket, ExpiresAt: expiresAt})
}

func (h *Handlers) ExportDeploymentTargetData(ctx *gin.Context) {
	authorization, ok := h.authorizeDeploymentTargetDataExport(ctx)
	if !ok {
		return
	}
	session, ok := h.currentSessionFromCookie(ctx)
	if !ok || session.UserID != authorization.user.ID {
		writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.session.expired")
		return
	}
	ticket := strings.TrimSpace(ctx.Query("ticket"))
	if ticket == "" {
		writeErrorCode(ctx, http.StatusBadRequest, "data_export.ticket_required", "data export ticket is required")
		return
	}
	valid, err := h.consumeDataExportTicket(ctx.Request.Context(), ticket, authorization, session)
	if err != nil {
		h.audit(authorization.user.ID, "deployment_target.data_export", authorization.target.ID, false, err.Error())
		writeErrorCode(ctx, http.StatusServiceUnavailable, "data_export.ticket_unavailable", "data export authorization is temporarily unavailable")
		return
	}
	if !valid {
		writeErrorCode(ctx, http.StatusForbidden, "data_export.ticket_invalid", "data export ticket is invalid, expired, consumed, or bound to another request")
		return
	}
	user, project, app, target := authorization.user, authorization.project, authorization.app, authorization.target
	client, namespace, ok := h.kubernetesClientForDeploymentTarget(ctx, project, target, "运行集群不可用，无法导出运行数据")
	if !ok {
		return
	}
	filename := fmt.Sprintf("%s-%s-data.tar.gz", app.Slug, target.ID)
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
		h.audit(user.ID, "deployment_target.data_export", target.ID, false, streamErr.Error())
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
		h.audit(user.ID, "deployment_target.data_export", target.ID, false, streamErr.Error())
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
		h.audit(user.ID, "deployment_target.data_export", target.ID, false, streamErr.Error())
		return
	}
	h.audit(user.ID, "deployment_target.data_export", target.ID, true, filename)
}

func (h *Handlers) issueDataExportTicket(ctx context.Context, authorization deploymentTargetDataExportAuthorization, session model.UserSession) (string, time.Time, error) {
	expiresAt := time.Now().Add(dataExportTicketTTL)
	value := dataExportTicketValue{
		UserID:        authorization.user.ID,
		SessionID:     session.ID,
		ProjectID:     authorization.project.ID,
		ApplicationID: authorization.app.ID,
		TargetID:      authorization.target.ID,
		ExpiresAt:     expiresAt,
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return "", time.Time{}, err
	}
	if h.rateLimiter != nil && h.rateLimiter.redis != nil {
		ticket := "r_" + randomHex(32)
		redisCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		err = h.rateLimiter.redis.Set(redisCtx, dataExportTicketKeyPrefix+hashToken(ticket), payload, dataExportTicketTTL).Err()
		cancel()
		if err == nil {
			return ticket, expiresAt, nil
		}
		if h.mode == "production" {
			return "", time.Time{}, err
		}
	}
	if h.mode == "production" {
		return "", time.Time{}, errors.New("Redis is required for production data export tickets")
	}
	ticket := "m_" + randomHex(32)
	dataExportMemoryTickets.Store(hashToken(ticket), value)
	return ticket, expiresAt, nil
}

func (h *Handlers) consumeDataExportTicket(ctx context.Context, ticket string, authorization deploymentTargetDataExportAuthorization, session model.UserSession) (bool, error) {
	var value dataExportTicketValue
	switch {
	case strings.HasPrefix(ticket, "r_"):
		if h.rateLimiter == nil || h.rateLimiter.redis == nil {
			return false, errors.New("Redis data export ticket store is unavailable")
		}
		redisCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		raw, err := h.rateLimiter.redis.GetDel(redisCtx, dataExportTicketKeyPrefix+hashToken(ticket)).Bytes()
		cancel()
		if errors.Is(err, redis.Nil) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if err := json.Unmarshal(raw, &value); err != nil {
			return false, err
		}
	case strings.HasPrefix(ticket, "m_") && h.mode != "production":
		raw, found := dataExportMemoryTickets.LoadAndDelete(hashToken(ticket))
		if !found {
			return false, nil
		}
		var ok bool
		value, ok = raw.(dataExportTicketValue)
		if !ok {
			return false, errors.New("invalid in-memory data export ticket")
		}
	default:
		return false, nil
	}
	if !value.ExpiresAt.After(time.Now()) {
		return false, nil
	}
	return value.UserID == authorization.user.ID &&
		value.SessionID == session.ID &&
		value.ProjectID == authorization.project.ID &&
		value.ApplicationID == authorization.app.ID &&
		value.TargetID == authorization.target.ID, nil
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
	if !h.ensureNoIncomingServiceBindings(ctx, target.ProjectID, target.ApplicationID, target.ID) {
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
