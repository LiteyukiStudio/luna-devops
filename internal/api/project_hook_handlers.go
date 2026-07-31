package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	hookPhasePrePull        = "prePull"
	hookPhasePostPull       = "postPull"
	hookPhasePreBuild       = "preBuild"
	hookPhasePostBuild      = "postBuild"
	hookPhasePrePush        = "prePush"
	hookPhasePostPush       = "postPush"
	hookPhasePreDeployment  = "preDeployment"
	hookPhasePostDeployment = "postDeployment"
)

var buildHookPhases = []string{
	hookPhasePrePull,
	hookPhasePostPull,
	hookPhasePreBuild,
	hookPhasePostBuild,
	hookPhasePrePush,
	hookPhasePostPush,
}

func (h *Handlers) ListProjectHookConfigs(ctx *gin.Context) {
	if _, _, ok := h.projectAndCurrentUser(ctx); !ok {
		return
	}
	var hooks []model.ProjectHookConfig
	if err := h.dbFor(ctx).Where("project_id = ?", ctx.Param("projectId")).Order("created_at asc").Find(&hooks).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, hooks)
}

func (h *Handlers) CreateProjectHookConfig(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, authz.ProjectRoleOwner, authz.ProjectRoleAdmin)
	if !ok {
		return
	}
	var input projectHookConfigInput
	if !bindJSON(ctx, &input) {
		return
	}
	hook, ok := h.projectHookConfigFromInput(ctx, user, project.ID, input, id.New("hook"))
	if !ok {
		return
	}
	if err := h.dbFor(ctx).Create(&hook).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	h.auditWithContext(user.ID, "project_hook.create", hook.ID, true, hook.Name, ctx.Request.Context())
	ctx.JSON(http.StatusCreated, hook)
}

func (h *Handlers) UpdateProjectHookConfig(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, authz.ProjectRoleOwner, authz.ProjectRoleAdmin)
	if !ok {
		return
	}
	var existing model.ProjectHookConfig
	if err := h.dbFor(ctx).First(&existing, "id = ? and project_id = ?", ctx.Param("hookId"), project.ID).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "project hook not found")
		return
	}
	var input projectHookConfigInput
	if !bindJSON(ctx, &input) {
		return
	}
	hook, ok := h.projectHookConfigFromInput(ctx, user, project.ID, input, existing.ID)
	if !ok {
		return
	}
	hook.CreatedBy = existing.CreatedBy
	hook.CreatedAt = existing.CreatedAt
	if err := h.dbFor(ctx).Save(&hook).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	h.auditWithContext(user.ID, "project_hook.update", hook.ID, true, hook.Name, ctx.Request.Context())
	ctx.JSON(http.StatusOK, hook)
}

func (h *Handlers) DeleteProjectHookConfig(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, authz.ProjectRoleOwner, authz.ProjectRoleAdmin)
	if !ok {
		return
	}
	var hook model.ProjectHookConfig
	if err := h.dbFor(ctx).First(&hook, "id = ? and project_id = ?", ctx.Param("hookId"), project.ID).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "project hook not found")
		return
	}
	if err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("hook_config_id = ?", hook.ID).Delete(&model.DeploymentTargetHookBinding{}).Error; err != nil {
			return err
		}
		return tx.Delete(&hook).Error
	}); err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.auditWithContext(user.ID, "project_hook.delete", hook.ID, true, hook.Name, ctx.Request.Context())
	ctx.Status(http.StatusNoContent)
}

func (h *Handlers) ListProjectHookRuns(ctx *gin.Context) {
	if _, _, ok := h.projectAndCurrentUser(ctx); !ok {
		return
	}
	query := h.dbFor(ctx).Where("project_id = ?", ctx.Param("projectId"))
	if phase := normalizeHookPhase(ctx.Query("phase")); phase != "" {
		query = query.Where("phase = ?", phase)
	}
	if buildRunID := strings.TrimSpace(ctx.Query("buildRunId")); buildRunID != "" {
		query = query.Where("build_run_id = ?", buildRunID)
	}
	if releaseID := strings.TrimSpace(ctx.Query("releaseId")); releaseID != "" {
		query = query.Where("release_id = ?", releaseID)
	}
	var runs []model.HookRun
	if err := query.Order("created_at desc").Limit(100).Find(&runs).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, runs)
}

func (h *Handlers) GetProjectHookRunLog(ctx *gin.Context) {
	if _, project, ok := h.projectAndCurrentUser(ctx); !ok {
		return
	} else {
		var run model.HookRun
		if err := h.dbFor(ctx).First(&run, "id = ? and project_id = ?", ctx.Param("runId"), project.ID).Error; err != nil {
			writeError(ctx, http.StatusNotFound, "hook run not found")
			return
		}
		var log model.HookRunLog
		err := h.dbFor(ctx).First(&log, "hook_run_id = ? and project_id = ?", run.ID, project.ID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			ctx.JSON(http.StatusOK, model.HookRunLog{HookRunID: run.ID, ProjectID: project.ID, Content: ""})
			return
		}
		if err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		}
		ctx.JSON(http.StatusOK, log)
	}
}

func (h *Handlers) projectHookConfigFromInput(ctx *gin.Context, user model.User, projectID string, input projectHookConfigInput, hookID string) (model.ProjectHookConfig, bool) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		writeError(ctx, http.StatusBadRequest, "hook name is required")
		return model.ProjectHookConfig{}, false
	}
	script := strings.TrimSpace(input.Script)
	if script == "" {
		writeError(ctx, http.StatusBadRequest, "hook script is required")
		return model.ProjectHookConfig{}, false
	}
	if len(script) > 64*1024 {
		writeError(ctx, http.StatusBadRequest, "hook script is too large")
		return model.ProjectHookConfig{}, false
	}
	return model.ProjectHookConfig{
		ID:             hookID,
		ProjectID:      projectID,
		Name:           name,
		Script:         script,
		Shell:          normalizeHookShell(input.Shell),
		TimeoutSeconds: normalizeHookTimeout(input.TimeoutSeconds),
		FailurePolicy:  normalizeHookFailurePolicy(input.FailurePolicy),
		CreatedBy:      user.ID,
	}, true
}

func (h *Handlers) appendHookRunLog(run model.HookRun, content string, contexts ...context.Context) error {
	content = h.redactHookRunLogContent(run, content, firstContext(contexts))
	content = trimHookRunLogContent(content)
	if content == "" {
		return nil
	}
	var existing model.HookRunLog
	err := h.dbWithContext(firstContext(contexts)).First(&existing, "hook_run_id = ? and project_id = ?", run.ID, run.ProjectID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return h.dbWithContext(firstContext(contexts)).Create(&model.HookRunLog{
			ID:        id.New("hlog"),
			HookRunID: run.ID,
			ProjectID: run.ProjectID,
			Content:   content,
		}).Error
	}
	if err != nil {
		return err
	}
	existing.Content = trimHookRunLogContent(existing.Content + "\n" + content)
	return h.dbWithContext(firstContext(contexts)).Save(&existing).Error
}

func trimHookRunLogContent(content string) string {
	content = strings.TrimSpace(content)
	const maxLogBytes = 1024 * 1024
	if len(content) <= maxLogBytes {
		return content
	}
	return content[len(content)-maxLogBytes:]
}

func normalizeHookPhase(value string) string {
	switch strings.TrimSpace(value) {
	case hookPhasePrePull:
		return hookPhasePrePull
	case hookPhasePostPull:
		return hookPhasePostPull
	case hookPhasePreBuild:
		return hookPhasePreBuild
	case hookPhasePostBuild:
		return hookPhasePostBuild
	case hookPhasePrePush:
		return hookPhasePrePush
	case hookPhasePostPush:
		return hookPhasePostPush
	case hookPhasePreDeployment:
		return hookPhasePreDeployment
	case hookPhasePostDeployment:
		return hookPhasePostDeployment
	default:
		return ""
	}
}

func isBuildHookPhase(phase string) bool {
	for _, item := range buildHookPhases {
		if phase == item {
			return true
		}
	}
	return false
}

func normalizeHookShell(value string) string {
	switch strings.TrimSpace(value) {
	case "bash":
		return "bash"
	default:
		return "sh"
	}
}

func normalizeHookFailurePolicy(value string) string {
	if strings.TrimSpace(value) == "ignore" {
		return "ignore"
	}
	return "fail"
}

func normalizeHookTimeout(value int) int {
	if value <= 0 {
		return 300
	}
	if value > 3600 {
		return 3600
	}
	return value
}

type projectHookConfigInput struct {
	Name           string `json:"name" binding:"required"`
	Script         string `json:"script" binding:"required"`
	Shell          string `json:"shell"`
	TimeoutSeconds int    `json:"timeoutSeconds"`
	FailurePolicy  string `json:"failurePolicy"`
}
