package api

import (
	"context"
	"errors"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"net/http"
	"strings"
	"time"
)

var errBuildRunNotCancelable = errors.New("build run is not cancelable")
var errBuildQueueUnavailable = errors.New("构建任务队列未配置")

func (h *Handlers) ListBuildRuns(ctx *gin.Context) {
	if _, ok := h.findProjectForCurrentUser(ctx); !ok {
		return
	}
	pagination := paginationFromQuery(ctx)
	query := h.db.Where("project_id = ?", ctx.Param("projectId"))
	if applicationID := strings.TrimSpace(ctx.Query("applicationId")); applicationID != "" {
		query = query.Where("application_id = ?", applicationID)
	}
	if targetID := strings.TrimSpace(ctx.Query("deploymentTargetId")); targetID != "" {
		query = query.Where("deployment_target_id = ?", targetID)
	}
	if status := strings.TrimSpace(ctx.Query("status")); status != "" && buildRunStatusAllowed(status) {
		query = query.Where("status = ?", status)
	}
	if triggerType := strings.TrimSpace(ctx.Query("triggerType")); triggerType != "" && buildRunTriggerAllowed(triggerType) {
		query = query.Where("trigger_type = ?", triggerType)
	}
	if branch := strings.TrimSpace(ctx.Query("sourceBranch")); branch != "" {
		query = query.Where("source_branch = ?", branch)
	}
	if actor := strings.TrimSpace(ctx.Query("createdBy")); actor != "" {
		query = query.Where("created_by = ?", actor)
	}
	query = applySearch(ctx, query, "id", "status", "trigger_type", "source_branch", "source_tag", "source_commit", "target_repository", "image_ref", "created_by")
	var runs []model.BuildRun
	if ctx.Query("page") == "" && ctx.Query("pageSize") == "" {
		if err := query.Order("created_at desc").Find(&runs).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		}
		ctx.JSON(http.StatusOK, runs)
		return
	}
	var total int64
	if err := query.Model(&model.BuildRun{}).Count(&total).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	orderBy := orderByClause(pagination, map[string]string{
		"createdAt":    "created_at",
		"status":       "status",
		"sourceCommit": "source_commit",
		"sourceBranch": "source_branch",
		"triggerType":  "trigger_type",
		"createdBy":    "created_by",
	}, "created_at")
	if err := query.Order(orderBy).Limit(pagination.PageSize).Offset(pagination.Offset()).Find(&runs).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, paginatedResponse(runs, total, pagination))
}

func (h *Handlers) GetBuildRun(ctx *gin.Context) {
	if _, ok := h.findProjectForCurrentUser(ctx); !ok {
		return
	}
	run, ok := h.findBuildRun(ctx)
	if !ok {
		return
	}
	ctx.JSON(http.StatusOK, run)
}

func buildRunStatusAllowed(status string) bool {
	switch status {
	case "queued", "running", "succeeded", "failed", "canceled", "lost", "timeout":
		return true
	default:
		return false
	}
}

func buildRunCancelable(status string) bool {
	return status == "queued" || status == "running"
}

func buildRunTerminal(status string) bool {
	return status == "succeeded" || status == "failed" || status == "canceled" || status == "lost" || status == "timeout"
}

func buildRunTriggerAllowed(triggerType string) bool {
	switch triggerType {
	case "manual", "webhook", "push", "tag", "api", "retry":
		return true
	default:
		return false
	}
}

func (h *Handlers) TriggerBuildRun(ctx *gin.Context) {
	user, _, ok := h.projectAndCurrentUserWithRoles(ctx, "owner", "admin", "developer")
	if !ok {
		return
	}
	var input buildRunInput
	if !bindJSON(ctx, &input) {
		return
	}
	if !h.ensureBillingAllowsNewBuild(ctx, ctx.Param("projectId")) {
		return
	}
	run := h.buildRunFromInput(ctx.Param("projectId"), user, input)
	run.ID = id.New("bldr")
	run.Status = "queued"
	run.TriggerType = fallback(strings.TrimSpace(input.TriggerType), "manual")
	h.createQueuedBuildRun(ctx, user, run, strings.TrimSpace(input.TargetImageRef), http.StatusCreated)
}

func (h *Handlers) RetryBuildRun(ctx *gin.Context) {
	user, _, ok := h.projectAndCurrentUserWithRoles(ctx, "owner", "admin", "developer")
	if !ok {
		return
	}
	previous, ok := h.findBuildRun(ctx)
	if !ok {
		return
	}
	if !h.ensureBillingAllowsNewBuild(ctx, previous.ProjectID) {
		return
	}
	run := model.BuildRun{
		ID:                      id.New("bldr"),
		ProjectID:               previous.ProjectID,
		ApplicationID:           previous.ApplicationID,
		DeploymentTargetID:      previous.DeploymentTargetID,
		BuildVariableSetIDs:     previous.BuildVariableSetIDs,
		BuildVariablesSnapshot:  previous.BuildVariablesSnapshot,
		BuildSecretRefsSnapshot: previous.BuildSecretRefsSnapshot,
		Status:                  "queued",
		TriggerType:             "retry",
		SourceBranch:            previous.SourceBranch,
		SourceTag:               previous.SourceTag,
		SourceCommit:            previous.SourceCommit,
		BuildDefinitionMode:     previous.BuildDefinitionMode,
		BuildTemplateID:         previous.BuildTemplateID,
		BuildTemplateVersion:    previous.BuildTemplateVersion,
		BuildTemplateValues:     previous.BuildTemplateValues,
		BuildTemplateDockerfile: previous.BuildTemplateDockerfile,
		BuildTemplateChecksum:   previous.BuildTemplateChecksum,
		DockerfilePath:          previous.DockerfilePath,
		BuildContext:            previous.BuildContext,
		BuildDirectory:          previous.BuildDirectory,
		BuildArgs:               previous.BuildArgs,
		BuildEnvironmentID:      previous.BuildEnvironmentID,
		BuildCPURequest:         previous.BuildCPURequest,
		BuildMemoryRequest:      previous.BuildMemoryRequest,
		BuildTimeoutSeconds:     previous.BuildTimeoutSeconds,
		TargetRegistryID:        previous.TargetRegistryID,
		TargetRepository:        previous.TargetRepository,
		TargetTag:               previous.TargetTag,
		CacheConfig:             previous.CacheConfig,
		CreatedBy:               user.ID,
		TriggeredByName:         buildRunActorName(user),
		TriggeredByEmail:        strings.TrimSpace(user.Email),
		SourceAuthorName:        previous.SourceAuthorName,
		SourceAuthorEmail:       previous.SourceAuthorEmail,
	}
	h.createQueuedBuildRun(ctx, user, run, "", http.StatusCreated)
}

func (h *Handlers) CancelBuildRun(ctx *gin.Context) {
	user, _, ok := h.projectAndCurrentUserWithRoles(ctx, "owner", "admin", "developer")
	if !ok {
		return
	}
	run, ok := h.findBuildRun(ctx)
	if !ok {
		return
	}
	if !buildRunCancelable(run.Status) {
		writeError(ctx, http.StatusConflict, "只有排队中或运行中的构建可以终止")
		return
	}

	finishedAt := time.Now()
	var jobs []model.BuildJob
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		var lockedRun model.BuildRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedRun, "id = ? and project_id = ?", run.ID, run.ProjectID).Error; err != nil {
			return err
		}
		if !buildRunCancelable(lockedRun.Status) {
			return errBuildRunNotCancelable
		}
		if err := tx.Where("build_run_id = ? and project_id = ? and status in ?", lockedRun.ID, lockedRun.ProjectID, []string{"queued", "running"}).Find(&jobs).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.BuildJob{}).
			Where("build_run_id = ? and project_id = ? and status in ?", lockedRun.ID, lockedRun.ProjectID, []string{"queued", "running"}).
			Updates(map[string]any{
				"status":      "canceled",
				"message":     "canceled by user",
				"lease_until": nil,
				"finished_at": &finishedAt,
			}).Error; err != nil {
			return err
		}
		return tx.Model(&model.BuildRun{}).
			Where("id = ?", lockedRun.ID).
			Updates(map[string]any{
				"status":      "canceled",
				"finished_at": &finishedAt,
			}).Error
	}); err != nil {
		if errors.Is(err, errBuildRunNotCancelable) {
			writeError(ctx, http.StatusConflict, "只有排队中或运行中的构建可以终止")
			return
		}
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	h.audit(user.ID, "build_run.cancel", run.ID, true, "")
	if err := h.db.First(&run, "id = ? and project_id = ?", run.ID, run.ProjectID).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, run)
}

func (h *Handlers) DeleteBuildRun(ctx *gin.Context) {
	user, _, ok := h.projectAndCurrentUserWithRoles(ctx, "owner", "admin", "developer")
	if !ok {
		return
	}
	run, ok := h.findBuildRun(ctx)
	if !ok {
		return
	}
	if !buildRunTerminal(run.Status) {
		writeError(ctx, http.StatusConflict, "只有已结束的构建记录可以删除")
		return
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		var hookRuns []model.HookRun
		if err := tx.Where("build_run_id = ? and project_id = ?", run.ID, run.ProjectID).Find(&hookRuns).Error; err != nil {
			return err
		}
		hookRunIDs := make([]string, 0, len(hookRuns))
		for _, hookRun := range hookRuns {
			hookRunIDs = append(hookRunIDs, hookRun.ID)
		}
		if len(hookRunIDs) > 0 {
			if err := tx.Where("hook_run_id in ? and project_id = ?", hookRunIDs, run.ProjectID).Delete(&model.HookRunLog{}).Error; err != nil {
				return err
			}
			if err := tx.Where("id in ? and project_id = ?", hookRunIDs, run.ProjectID).Delete(&model.HookRun{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("build_run_id = ? and project_id = ?", run.ID, run.ProjectID).Delete(&model.BuildLog{}).Error; err != nil {
			return err
		}
		if err := tx.Where("build_run_id = ? and project_id = ?", run.ID, run.ProjectID).Delete(&model.BuildJob{}).Error; err != nil {
			return err
		}
		return tx.Delete(&run).Error
	}); err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(user.ID, "build_run.delete", run.ID, true, "")
	ctx.Status(http.StatusNoContent)
}

func (h *Handlers) createQueuedBuildRun(ctx *gin.Context, user model.User, run model.BuildRun, targetImageRef string, statusCode int) {
	if !h.validateBuildRunRequest(ctx, user, &run) {
		return
	}
	_ = targetImageRef
	queuedRun, err := h.queueBuildRun(ctx.Request.Context(), user, run)
	if err != nil {
		if errors.Is(err, errBuildQueueUnavailable) || strings.Contains(err.Error(), "投递失败") {
			writeError(ctx, http.StatusServiceUnavailable, err.Error())
			return
		}
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	ctx.JSON(statusCode, queuedRun)
}

func (h *Handlers) queueBuildRun(ctx context.Context, user model.User, run model.BuildRun) (model.BuildRun, error) {
	job := model.BuildJob{
		ID:         id.New("bldj"),
		BuildRunID: run.ID,
		ProjectID:  run.ProjectID,
		Type:       "build",
		Status:     "queued",
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&run).Error; err != nil {
			return err
		}
		return tx.Create(&job).Error
	}); err != nil {
		return run, err
	}
	if h.taskClient == nil {
		h.markBuildRunDispatchFailed(run, job, "构建任务队列未配置")
		return run, errBuildQueueUnavailable
	}
	if _, err := h.taskClient.EnqueueBuildRun(ctx, tasks.BuildRunPayload{
		BuildRunID: run.ID,
		BuildJobID: job.ID,
		ProjectID:  run.ProjectID,
		ActorID:    user.ID,
	}); err != nil {
		h.markBuildRunDispatchFailed(run, job, "构建任务投递失败: "+err.Error())
		return run, errors.New("构建任务投递失败")
	}
	return run, nil
}

func (h *Handlers) markBuildRunDispatchFailed(run model.BuildRun, job model.BuildJob, message string) {
	finishedAt := time.Now()
	_ = h.db.Model(&model.BuildJob{}).Where("id = ? and project_id = ?", job.ID, job.ProjectID).Updates(map[string]any{
		"status":      "failed",
		"message":     message,
		"finished_at": &finishedAt,
	}).Error
	_ = h.db.Model(&model.BuildRun{}).Where("id = ? and project_id = ?", run.ID, run.ProjectID).Updates(map[string]any{
		"status":      "failed",
		"finished_at": &finishedAt,
	}).Error
}

func (h *Handlers) findBuildRun(ctx *gin.Context) (model.BuildRun, bool) {
	var run model.BuildRun
	if err := h.db.First(&run, "id = ? and project_id = ?", ctx.Param("runId"), ctx.Param("projectId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "build run not found")
		return run, false
	}
	return run, true
}

func (h *Handlers) buildRunFromInput(projectID string, user model.User, input buildRunInput) model.BuildRun {
	targetRepository, targetTag := splitTargetImageRef(input.TargetImageRef)
	if targetRepository == "" {
		targetRepository = strings.Trim(strings.TrimSpace(input.TargetRepository), "/")
		targetTag = strings.TrimSpace(input.TargetTag)
	}
	return model.BuildRun{
		ProjectID:           projectID,
		ApplicationID:       strings.TrimSpace(input.ApplicationID),
		DeploymentTargetID:  strings.TrimSpace(input.DeploymentTargetID),
		BuildVariableSetIDs: encodeBuildVariableSetIDs(input.BuildVariableSetIDs),
		SourceBranch:        strings.TrimSpace(input.SourceBranch),
		SourceTag:           strings.TrimSpace(input.SourceTag),
		SourceCommit:        strings.TrimSpace(input.SourceCommit),
		DockerfilePath:      fallback(strings.TrimSpace(input.DockerfilePath), "Dockerfile"),
		BuildContext:        fallback(strings.TrimSpace(input.BuildContext), "."),
		BuildDirectory:      strings.TrimSpace(input.BuildDirectory),
		BuildArgs:           normalizeBuildArgsInputValue(input.BuildArgs),
		BuildEnvironmentID:  strings.TrimSpace(input.BuildEnvironmentID),
		BuildCPURequest:     strings.TrimSpace(input.BuildCPURequest),
		BuildMemoryRequest:  strings.TrimSpace(input.BuildMemoryRequest),
		BuildTimeoutSeconds: input.BuildTimeoutSeconds,
		TargetRegistryID:    strings.TrimSpace(input.TargetRegistryID),
		TargetRepository:    targetRepository,
		TargetTag:           fallback(targetTag, "latest"),
		ImageRef:            "",
		CacheConfig:         strings.TrimSpace(input.CacheConfig),
		CreatedBy:           user.ID,
		TriggeredByName:     buildRunActorName(user),
		TriggeredByEmail:    strings.TrimSpace(user.Email),
	}
}

func buildRunActorName(user model.User) string {
	return fallback(strings.TrimSpace(user.Name), strings.TrimSpace(user.Email))
}

func splitTargetImageRef(value string) (string, string) {
	normalized := strings.Trim(strings.TrimSpace(value), "/")
	if normalized == "" {
		return "", ""
	}
	lastSlash := strings.LastIndex(normalized, "/")
	lastColon := strings.LastIndex(normalized, ":")
	if lastColon > lastSlash {
		repository := strings.Trim(strings.TrimSpace(normalized[:lastColon]), "/")
		tag := strings.TrimSpace(normalized[lastColon+1:])
		return repository, tag
	}
	return normalized, "latest"
}

type buildRunInput struct {
	ApplicationID       string   `json:"applicationId"`
	DeploymentTargetID  string   `json:"deploymentTargetId"`
	BuildVariableSetIDs []string `json:"buildVariableSetIds"`
	TriggerType         string   `json:"triggerType"`
	SourceBranch        string   `json:"sourceBranch"`
	SourceTag           string   `json:"sourceTag"`
	SourceCommit        string   `json:"sourceCommit"`
	DockerfilePath      string   `json:"dockerfilePath"`
	BuildContext        string   `json:"buildContext"`
	BuildDirectory      string   `json:"buildDirectory"`
	BuildArgs           string   `json:"buildArgs"`
	BuildEnvironmentID  string   `json:"buildEnvironmentId"`
	BuildCPURequest     string   `json:"buildCpuRequest"`
	BuildMemoryRequest  string   `json:"buildMemoryRequest"`
	BuildTimeoutSeconds int      `json:"buildTimeoutSeconds"`
	TargetRegistryID    string   `json:"targetRegistryId"`
	TargetImageRef      string   `json:"targetImageRef"`
	TargetRepository    string   `json:"targetRepository"`
	TargetTag           string   `json:"targetTag"`
	ImageRef            string   `json:"imageRef"`
	CacheConfig         string   `json:"cacheConfig"`
}
