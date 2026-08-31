package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
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
	"gorm.io/gorm/clause"
)

var errApplicationIdentifierExists = errors.New("application identifier already exists")

type applicationDeletionTargetPreview struct {
	TargetID   string                             `json:"deploymentTargetId"`
	TargetName string                             `json:"deploymentTargetName"`
	Volumes    []applicationDeletionVolumePreview `json:"volumes"`
}

type applicationDeletionVolumePreview struct {
	BindingID       string `json:"bindingId"`
	ProjectVolumeID string `json:"projectVolumeId"`
	DisplayName     string `json:"displayName"`
	LogicalName     string `json:"logicalName"`
	MountPath       string `json:"mountPath,omitempty"`
	DevicePath      string `json:"devicePath,omitempty"`
	ActivationState string `json:"activationState"`
}

type applicationDeletionPreview struct {
	HasPersistentData bool                               `json:"hasPersistentData"`
	Targets           []applicationDeletionTargetPreview `json:"targets"`
}

func (h *Handlers) ListApplications(ctx *gin.Context) {
	_, project, ok := h.authorizeProject(ctx, authz.ActionApplicationRead)
	if !ok {
		return
	}

	var applications []model.Application
	query := h.dbFor(ctx).Model(&model.Application{}).Where("project_id = ?", ctx.Param("projectId"))
	query = applySearch(ctx, query, "name", "identifier")
	pagination := paginationFromQueryWithSort(ctx, map[string]string{"name": "name", "identifier": "identifier", "createdAt": "created_at"}, "createdAt")
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if err := query.Order(orderByClause(pagination, map[string]string{
		"name":       "name",
		"identifier": "identifier",
		"createdAt":  "created_at",
	}, "created_at")).Limit(pagination.PageSize).Offset(pagination.Offset()).Find(&applications).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if ctx.Query("includeRuntime") == "true" {
		items, err := h.applicationListItemsWithRuntime(ctx.Request.Context(), project, applications)
		if err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		}
		markLiveObservationResponse(ctx)
		ctx.JSON(http.StatusOK, paginatedResponse(items, total, pagination))
		return
	}
	ctx.JSON(http.StatusOK, paginatedResponse(applications, total, pagination))
}

func (h *Handlers) CreateApplication(ctx *gin.Context) {
	_, _, ok := h.authorizeProject(ctx, authz.ActionApplicationCreate)
	if !ok {
		return
	}

	var input applicationInput
	if !bindJSON(ctx, &input) {
		return
	}
	input.Identifier = strings.TrimSpace(input.Identifier)
	if err := resourceidentifier.Validate(input.Identifier, applicationIdentifierMinLength, applicationIdentifierMaxLength); err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "application.identifier_invalid", err.Error())
		return
	}
	if !h.ensureApplicationIdentifierAvailable(ctx, ctx.Param("projectId"), input.Identifier, "") {
		return
	}
	app := model.Application{
		ID:           id.New("app"),
		ProjectID:    ctx.Param("projectId"),
		Identifier:   input.Identifier,
		Name:         input.Name,
		Icon:         normalizeApplicationIcon(input.Icon),
		DeleteStatus: "active",
	}

	if err := createApplicationRecord(h.dbFor(ctx), &app); errors.Is(err, errApplicationIdentifierExists) {
		writeApplicationIdentifierConflict(ctx, "active")
		return
	} else if err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	ctx.JSON(http.StatusCreated, app)
}

func (h *Handlers) GetApplication(ctx *gin.Context) {
	if _, _, ok := h.authorizeProject(ctx, authz.ActionApplicationRead); !ok {
		return
	}

	app, ok := h.findApplication(ctx)
	if !ok {
		return
	}
	ctx.JSON(http.StatusOK, app)
}

func (h *Handlers) UpdateApplication(ctx *gin.Context) {
	_, _, ok := h.authorizeProject(ctx, authz.ActionApplicationUpdate)
	if !ok {
		return
	}

	app, ok := h.findApplication(ctx)
	if !ok {
		return
	}
	if !applicationCanMutate(app) {
		writeErrorCode(ctx, http.StatusConflict, "application.delete_in_progress", "应用正在删除中，不能编辑")
		return
	}

	var input applicationInput
	if !bindJSON(ctx, &input) {
		return
	}
	input.Identifier = strings.TrimSpace(input.Identifier)
	if input.Identifier != app.Identifier {
		writeErrorCode(ctx, http.StatusConflict, "application.identifier_immutable", "application identifier cannot be changed")
		return
	}
	app.Name = input.Name
	app.Icon = normalizeApplicationIcon(input.Icon)

	if err := h.dbFor(ctx).Save(&app).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, app)
}

func (h *Handlers) DeleteApplication(ctx *gin.Context) {
	user, _, ok := h.authorizeProject(ctx, authz.ActionApplicationDelete)
	if !ok {
		return
	}

	app, ok := h.findApplication(ctx)
	if !ok {
		return
	}
	if !applicationCanMutate(app) {
		writeErrorCode(ctx, http.StatusConflict, "application.delete_in_progress", "应用正在删除中，不能重复操作")
		return
	}
	if !h.ensureNoIncomingServiceBindings(ctx, app.ProjectID, app.ID, "") {
		return
	}
	var targets []model.DeploymentTarget
	if err := h.dbFor(ctx).Where("project_id = ? and application_id = ?", app.ProjectID, app.ID).Find(&targets).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	startedAt := time.Now()
	volumeChanges := make(map[string]deploymentVolumeMountChanges, len(targets))
	err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.Application{}).
			Where("id = ? and project_id = ? and delete_status in ?", app.ID, app.ProjectID, []string{"active", "delete_failed", ""}).
			Updates(map[string]any{
				"delete_status":      "deleting",
				"delete_message":     "",
				"delete_started_at":  &startedAt,
				"delete_finished_at": nil,
			}).Error; err != nil {
			return err
		}
		app.DeleteStatus = "deleting"
		app.DeleteMessage = ""
		app.DeleteStartedAt = &startedAt
		app.DeleteFinishedAt = nil
		for _, target := range targets {
			changes, syncErr := syncDeploymentTargetVolumeMounts(ctx.Request.Context(), tx, target, nil)
			volumeChanges[target.ID] = changes
			if syncErr != nil {
				return syncErr
			}
		}
		return nil
	})
	if err != nil {
		for _, target := range targets {
			h.auditDeploymentVolumeMountFailure(ctx.Request.Context(), user.ID, volumeChanges[target.ID], err)
		}
		if volume.ErrorCode(err) != "" {
			writeVolumeError(ctx, err)
		} else {
			writeError(ctx, http.StatusInternalServerError, err.Error())
		}
		return
	}
	for _, target := range targets {
		h.auditDeploymentVolumeMountChanges(ctx.Request.Context(), user.ID, target, volumeChanges[target.ID])
	}
	if !h.enqueueApplicationDelete(ctx.Request.Context(), app, user.ID, false) {
		finishedAt := time.Now()
		_ = h.dbFor(ctx).Model(&model.Application{}).Where("id = ?", app.ID).Updates(map[string]any{
			"delete_status":      "delete_failed",
			"delete_message":     "应用删除任务投递失败，请确认 Worker 队列可用后重试",
			"delete_finished_at": &finishedAt,
		}).Error
		writeError(ctx, http.StatusServiceUnavailable, "应用删除任务投递失败，请确认 Worker 队列可用后重试")
		return
	}
	h.auditWithContext(user.ID, "application.delete.request", app.ID, true, "volume_mounts_unbound", ctx.Request.Context())
	ctx.JSON(http.StatusAccepted, app)
}

func (h *Handlers) PreviewApplicationDeletion(ctx *gin.Context) {
	_, project, ok := h.authorizeProject(ctx, authz.ActionApplicationDelete)
	if !ok {
		return
	}
	app, ok := h.findApplication(ctx)
	if !ok {
		return
	}
	result, err := h.buildApplicationDeletionPreview(ctx.Request.Context(), project, app)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, result)
}

func (h *Handlers) buildApplicationDeletionPreview(requestCtx context.Context, project model.Project, app model.Application) (applicationDeletionPreview, error) {
	var targets []model.DeploymentTarget
	if err := h.dbWithContext(requestCtx).Where("project_id = ? and application_id = ?", project.ID, app.ID).Find(&targets).Error; err != nil {
		return applicationDeletionPreview{}, err
	}
	mountsByTarget, err := h.deploymentTargetVolumeMountsByTarget(requestCtx, targets)
	if err != nil {
		return applicationDeletionPreview{}, err
	}
	volumeIDs := make([]string, 0)
	for _, mounts := range mountsByTarget {
		for _, mount := range mounts {
			if mount.ProjectVolumeID != nil {
				volumeIDs = append(volumeIDs, *mount.ProjectVolumeID)
			}
		}
	}
	volumesByID := make(map[string]model.ProjectVolume, len(volumeIDs))
	if len(volumeIDs) > 0 {
		var volumes []model.ProjectVolume
		if err := h.dbWithContext(requestCtx).Where("project_id = ? and id in ?", project.ID, volumeIDs).Find(&volumes).Error; err != nil {
			return applicationDeletionPreview{}, err
		}
		for _, projectVolume := range volumes {
			volumesByID[projectVolume.ID] = projectVolume
		}
	}
	result := applicationDeletionPreview{Targets: make([]applicationDeletionTargetPreview, 0, len(targets))}
	for _, target := range targets {
		preview := applicationDeletionTargetPreview{TargetID: target.ID, TargetName: target.Name, Volumes: []applicationDeletionVolumePreview{}}
		for _, mount := range mountsByTarget[target.ID] {
			if mount.ProjectVolumeID == nil {
				continue
			}
			projectVolume, exists := volumesByID[*mount.ProjectVolumeID]
			if !exists {
				continue
			}
			preview.Volumes = append(preview.Volumes, applicationDeletionVolumePreview{
				BindingID: mount.ID, ProjectVolumeID: projectVolume.ID, DisplayName: projectVolume.DisplayName,
				LogicalName: mount.LogicalName, MountPath: optionalStringValue(mount.MountPath),
				DevicePath: optionalStringValue(mount.DevicePath), ActivationState: mount.ActivationState,
			})
		}
		result.HasPersistentData = result.HasPersistentData || len(preview.Volumes) > 0
		result.Targets = append(result.Targets, preview)
	}
	return result, nil
}

func (h *Handlers) enqueueApplicationDelete(ctx context.Context, app model.Application, actorID string, deleteData bool) bool {
	if h.taskClient == nil {
		return false
	}
	_, err := h.taskClient.EnqueueApplicationDelete(ctx, tasks.ApplicationDeletePayload{
		ApplicationID: app.ID,
		ProjectID:     app.ProjectID,
		ActorID:       actorID,
		DeleteData:    deleteData,
	})
	if err != nil {
		return false
	}
	return true
}

func (h *Handlers) findApplication(ctx *gin.Context) (model.Application, bool) {
	var app model.Application
	err := h.dbFor(ctx).First(
		&app,
		"id = ? and project_id = ?",
		ctx.Param("applicationId"),
		ctx.Param("projectId"),
	).Error
	if err != nil {
		writeError(ctx, http.StatusNotFound, "application not found")
		return app, false
	}
	return app, true
}

func applicationCanMutate(app model.Application) bool {
	status := strings.TrimSpace(app.DeleteStatus)
	return status == "" || status == "active" || status == "delete_failed"
}

func (h *Handlers) ensureApplicationIdentifierAvailable(ctx *gin.Context, projectID string, identifier string, excludeApplicationID string) bool {
	if identifier == "" {
		writeErrorCode(ctx, http.StatusBadRequest, "application.identifier_invalid", "应用标识不能为空")
		return false
	}
	query := h.dbFor(ctx).Select("id", "delete_status").Where("project_id = ? and identifier = ?", projectID, identifier)
	if strings.TrimSpace(excludeApplicationID) != "" {
		query = query.Where("id <> ?", excludeApplicationID)
	}
	var existing model.Application
	if err := query.First(&existing).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return true
	} else if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return false
	}
	writeApplicationIdentifierConflict(ctx, existing.DeleteStatus)
	return false
}

func createApplicationRecord(db *gorm.DB, application *model.Application) error {
	result := db.Clauses(clause.OnConflict{
		Columns:     []clause.Column{{Name: "project_id"}, {Name: "identifier"}},
		TargetWhere: clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "deleted_at IS NULL"}}},
		DoNothing:   true,
	}).Create(application)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errApplicationIdentifierExists
	}
	return nil
}

func writeApplicationIdentifierConflict(ctx *gin.Context, deleteStatus string) {
	switch strings.TrimSpace(deleteStatus) {
	case "deleting":
		writeErrorCode(ctx, http.StatusConflict, "application.identifier_delete_in_progress", "同标识应用正在删除，资源清理完成后才能复用")
	case "delete_failed":
		writeErrorCode(ctx, http.StatusConflict, "application.identifier_delete_failed", "同标识应用上次删除失败，请先完成资源清理")
	default:
		writeErrorCode(ctx, http.StatusConflict, "application.identifier_exists", "该项目空间内应用标识已存在")
	}
}

type applicationInput struct {
	Identifier string `json:"identifier" binding:"required"`
	Name       string `json:"name" binding:"required"`
	Icon       string `json:"icon"`
}

func normalizeBuildConcurrencyPolicy(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "parallel":
		return "parallel"
	default:
		return "queue"
	}
}

func normalizeApplicationIcon(value string) string {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "box"
	}
	if isApplicationIconReference(normalized) {
		return normalized
	}
	return "box"
}

func isApplicationIconReference(value string) bool {
	if value == "" || len(value) > 512 || strings.ContainsAny(value, "\r\n\t") {
		return false
	}
	for _, icon := range applicationIconNames {
		if value == icon {
			return true
		}
	}
	if strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//") {
		return true
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		return false
	}
	return parsed.Scheme == "https" || parsed.Scheme == "http"
}

var applicationIconNames = []string{
	"box",
	"app-window",
	"layout-dashboard",
	"server",
	"database",
	"cpu",
	"cloud",
	"globe",
	"network",
	"shield",
	"lock-keyhole",
	"key-round",
	"shopping-cart",
	"credit-card",
	"chart-line",
	"bar-chart-3",
	"message-square",
	"mail",
	"bell",
	"calendar",
	"file-text",
	"folder-kanban",
	"git-branch",
	"terminal",
	"workflow",
	"package",
	"container",
	"rocket",
	"zap",
	"bot",
	"users",
	"settings",
}
