package api

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/resourceidentifier"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *Handlers) ListApplications(ctx *gin.Context) {
	if _, ok := h.findProjectForCurrentUser(ctx); !ok {
		return
	}

	var applications []model.Application
	query := h.db.Model(&model.Application{}).Where("project_id = ?", ctx.Param("projectId"))
	query = applySearch(ctx, query, "name", "identifier")
	if paginationRequested(ctx) {
		pagination := paginationFromQuery(ctx)
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
		ctx.JSON(http.StatusOK, paginatedResponse(applications, total, pagination))
		return
	}
	if err := query.Order("created_at desc").Find(&applications).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, applications)
}

func (h *Handlers) CreateApplication(ctx *gin.Context) {
	_, project, ok := h.projectAndCurrentUserWithRoles(ctx, authz.ProjectRoleOwner, authz.ProjectRoleAdmin, authz.ProjectRoleDeveloper)
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
		ID:                resourceidentifier.ApplicationID(project.Identifier, input.Identifier),
		ProjectID:         ctx.Param("projectId"),
		Identifier:        input.Identifier,
		Name:              input.Name,
		Icon:              normalizeApplicationIcon(input.Icon),
		DeleteStatus:      "active",
		DataRetentionMode: "retain",
	}

	if err := h.db.Create(&app).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	ctx.JSON(http.StatusCreated, app)
}

func (h *Handlers) GetApplication(ctx *gin.Context) {
	if _, ok := h.findProjectForCurrentUser(ctx); !ok {
		return
	}

	app, ok := h.findApplication(ctx)
	if !ok {
		return
	}
	ctx.JSON(http.StatusOK, app)
}

func (h *Handlers) UpdateApplication(ctx *gin.Context) {
	_, _, ok := h.projectAndCurrentUserWithRoles(ctx, authz.ProjectRoleOwner, authz.ProjectRoleAdmin, authz.ProjectRoleDeveloper)
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

	if err := h.db.Save(&app).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, app)
}

func (h *Handlers) DeleteApplication(ctx *gin.Context) {
	user, _, ok := h.projectAndCurrentUserWithRoles(ctx, authz.ProjectRoleOwner, authz.ProjectRoleAdmin)
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
	startedAt := time.Now()
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.Application{}).
			Where("id = ? and project_id = ? and delete_status in ?", app.ID, app.ProjectID, []string{"active", "delete_failed", ""}).
			Updates(map[string]any{
				"delete_status":       "deleting",
				"delete_message":      "",
				"delete_started_at":   &startedAt,
				"delete_finished_at":  nil,
				"data_retention_mode": "retain",
			}).Error; err != nil {
			return err
		}
		app.DeleteStatus = "deleting"
		app.DeleteMessage = ""
		app.DeleteStartedAt = &startedAt
		app.DeleteFinishedAt = nil
		app.DataRetentionMode = "retain"
		return nil
	})
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if !h.enqueueApplicationDelete(ctx.Request.Context(), app, user.ID, false) {
		finishedAt := time.Now()
		_ = h.db.Model(&model.Application{}).Where("id = ?", app.ID).Updates(map[string]any{
			"delete_status":      "delete_failed",
			"delete_message":     "应用删除任务投递失败，请确认 Worker 队列可用后重试",
			"delete_finished_at": &finishedAt,
		}).Error
		writeError(ctx, http.StatusServiceUnavailable, "应用删除任务投递失败，请确认 Worker 队列可用后重试")
		return
	}
	h.audit(user.ID, "application.delete.request", app.ID, true, app.Name)
	ctx.JSON(http.StatusAccepted, app)
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
	err := h.db.First(
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
		writeError(ctx, http.StatusBadRequest, "应用标识不能为空")
		return false
	}
	query := h.db.Unscoped().Model(&model.Application{}).Where("project_id = ? and identifier = ?", projectID, identifier)
	if strings.TrimSpace(excludeApplicationID) != "" {
		query = query.Where("id <> ?", excludeApplicationID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return false
	}
	if count > 0 {
		writeError(ctx, http.StatusBadRequest, "该项目空间内应用标识已存在")
		return false
	}
	return true
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
