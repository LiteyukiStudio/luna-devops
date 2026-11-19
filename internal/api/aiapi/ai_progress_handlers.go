package aiapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

const aiProgressPollInterval = time.Second

type aiProgressStep struct {
	ID        string `json:"id"`
	LabelCode string `json:"labelCode"`
	Status    string `json:"status"`
}

type aiProgressValue struct {
	Mode  string   `json:"mode"`
	Value *float64 `json:"value,omitempty"`
}

type aiProgressError struct {
	Code      string `json:"code"`
	RequestID string `json:"requestId,omitempty"`
	TraceID   string `json:"traceId,omitempty"`
}

type aiProgressSnapshot struct {
	OperationID   string           `json:"operationId"`
	OperationType string           `json:"operationType"`
	Revision      string           `json:"revision"`
	State         string           `json:"state"`
	StageCode     string           `json:"stageCode"`
	Progress      aiProgressValue  `json:"progress"`
	Steps         []aiProgressStep `json:"steps"`
	StartedAt     *time.Time       `json:"startedAt,omitempty"`
	UpdatedAt     time.Time        `json:"updatedAt"`
	FinishedAt    *time.Time       `json:"finishedAt,omitempty"`
	Error         *aiProgressError `json:"error,omitempty"`
}

type aiTaskStatusInternal struct {
	TaskType           string     `json:"taskType"`
	TaskID             string     `json:"taskId"`
	ProjectID          string     `json:"projectId"`
	ApplicationID      string     `json:"applicationId,omitempty"`
	DeploymentTargetID string     `json:"deploymentTargetId,omitempty"`
	RawStatus          string     `json:"rawStatus"`
	TaskStatus         string     `json:"taskStatus"`
	Terminal           bool       `json:"terminal"`
	FailureCode        string     `json:"failureCode,omitempty"`
	ImageRef           string     `json:"imageRef,omitempty"`
	ImageDigest        string     `json:"imageDigest,omitempty"`
	SourceBranch       string     `json:"sourceBranch,omitempty"`
	SourceTag          string     `json:"sourceTag,omitempty"`
	SourceCommit       string     `json:"sourceCommit,omitempty"`
	BuildRunID         string     `json:"buildRunId,omitempty"`
	Revision           int        `json:"revision,omitempty"`
	ReleaseType        string     `json:"releaseType,omitempty"`
	StartedAt          *time.Time `json:"startedAt,omitempty"`
	FinishedAt         *time.Time `json:"finishedAt,omitempty"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}

type aiRunTaskBinding struct {
	OwnerUserID           string `gorm:"column:owner_user_id"`
	ActorSessionID        string `gorm:"column:actor_session_id"`
	RunStatus             string `gorm:"column:run_status"`
	ConversationOwnerID   string `gorm:"column:conversation_owner_id"`
	ConversationProjectID string `gorm:"column:conversation_project_id"`
}

// GetAITaskStatusInternal 为 Agent 的 wait_for_task 快检与后台到期唤醒提供无审计噪音的权威任务状态读取。
// 每次调用按 Run 绑定的真实用户与会话重新校验项目空间只读权限，不写 AuditLog。
func (h *Handler) GetAITaskStatusInternal(ctx *gin.Context) {
	if !h.requireAIAgentService(ctx) {
		return
	}
	runID := strings.TrimSpace(ctx.Param("runId"))
	projectID := strings.TrimSpace(ctx.Param("projectId"))
	taskType := strings.TrimSpace(ctx.Param("taskType"))
	taskID := strings.TrimSpace(ctx.Param("taskId"))
	if runID == "" || projectID == "" || taskID == "" || len(runID) > 128 || len(projectID) > 128 || len(taskID) > 128 {
		writeErrorCode(ctx, http.StatusBadRequest, "request.invalid", "AI task status parameters are incomplete")
		return
	}
	var requiredAction authz.Action
	switch taskType {
	case "build_run":
		requiredAction = authz.ActionBuildRead
	case "release":
		requiredAction = authz.ActionDeploymentRead
	default:
		writeErrorCode(ctx, http.StatusBadRequest, "ai.progress.type_invalid", "task type must be build_run or release")
		return
	}

	var binding aiRunTaskBinding
	result := h.dbFor(ctx).Raw(`
		SELECT r.owner_user_id,
		       COALESCE(r.actor_session_id, '') AS actor_session_id,
		       r.status AS run_status,
		       c.owner_user_id AS conversation_owner_id,
		       COALESCE(c.project_id, '') AS conversation_project_id
		FROM ai.runs AS r
		JOIN ai.conversations AS c ON c.id = r.conversation_id
		WHERE r.id = ?
	`, runID).Scan(&binding)
	if result.Error != nil {
		writeErrorCode(ctx, http.StatusServiceUnavailable, "ai.progress.unavailable", "AI run task binding is unavailable")
		return
	}
	if result.RowsAffected != 1 {
		writeErrorCode(ctx, http.StatusNotFound, "ai.run_not_found", "AI Run was not found")
		return
	}
	if binding.RunStatus != "running" && binding.RunStatus != "waiting_external" {
		writeErrorCode(ctx, http.StatusConflict, "ai.run_not_resumable", "AI Run is not in an active task-observation state")
		return
	}
	if binding.OwnerUserID == "" || binding.OwnerUserID != binding.ConversationOwnerID || binding.ActorSessionID == "" {
		writeErrorCode(ctx, http.StatusForbidden, "ai.authorization_changed", "AI conversation ownership is no longer valid")
		return
	}
	requestCtx := h.host.WithAIPlatformActor(ctx.Request.Context(), PlatformActor{
		UserID:    binding.OwnerUserID,
		SessionID: binding.ActorSessionID,
		ProjectID: binding.ConversationProjectID,
	})
	restore := replaceRequestContext(ctx, requestCtx)
	defer restore()

	var resolvedProjectID string
	if binding.ConversationProjectID != "" {
		var convProject model.Project
		if err := h.dbFor(ctx).First(&convProject, "id = ? AND deleted_at IS NULL", binding.ConversationProjectID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				writeErrorCode(ctx, http.StatusForbidden, "ai.project_scope_mismatch", "AI conversation project scope is no longer available")
				return
			}
			writeErrorCode(ctx, http.StatusServiceUnavailable, "ai.progress.unavailable", "AI conversation project lookup failed")
			return
		}
		if projectID != convProject.ID && projectID != convProject.Identifier {
			writeErrorCode(ctx, http.StatusForbidden, "ai.project_scope_mismatch", "AI conversation project scope does not match the requested project")
			return
		}
		if _, _, ok := h.host.AuthorizeProjectByID(ctx, convProject.ID, requiredAction); !ok {
			return
		}
		resolvedProjectID = convProject.ID
	} else {
		_, project, ok := h.host.AuthorizeProjectRef(ctx, projectID, requiredAction)
		if !ok {
			return
		}
		resolvedProjectID = project.ID
	}

	snapshot, err := h.resolveAITaskStatusInternal(ctx, resolvedProjectID, taskType, taskID)
	if err != nil {
		if errors.Is(err, errAIProgressNotFound) {
			writeErrorCode(ctx, http.StatusNotFound, "resource.not_found", "target task was not found")
			return
		}
		writeErrorCode(ctx, http.StatusServiceUnavailable, "ai.progress.unavailable", "task status observation is unavailable")
		return
	}
	ctx.Header("Cache-Control", "no-store")
	ctx.JSON(http.StatusOK, snapshot)
}

func (h *Handler) resolveAITaskStatusInternal(ctx *gin.Context, projectID, taskType, taskID string) (aiTaskStatusInternal, error) {
	switch taskType {
	case "build_run":
		var run model.BuildRun
		if err := h.dbFor(ctx).First(&run, "id = ? AND project_id = ?", taskID, projectID).Error; err != nil {
			return aiTaskStatusInternal{}, normalizeAIProgressDBError(err)
		}
		return buildRunTaskStatusInternal(run), nil
	case "release":
		var release model.Release
		if err := h.dbFor(ctx).First(&release, "id = ? AND project_id = ?", taskID, projectID).Error; err != nil {
			return aiTaskStatusInternal{}, normalizeAIProgressDBError(err)
		}
		return releaseTaskStatusInternal(release), nil
	default:
		return aiTaskStatusInternal{}, errAIProgressTypeInvalid
	}
}

func buildRunTaskStatusInternal(run model.BuildRun) aiTaskStatusInternal {
	taskStatus, terminal := normalizeAITaskStatus(run.Status)
	return aiTaskStatusInternal{
		TaskType:           "build_run",
		TaskID:             run.ID,
		ProjectID:          run.ProjectID,
		ApplicationID:      strings.TrimSpace(run.ApplicationID),
		DeploymentTargetID: strings.TrimSpace(run.DeploymentTargetID),
		RawStatus:          strings.TrimSpace(run.Status),
		TaskStatus:         taskStatus,
		Terminal:           terminal,
		FailureCode:        strings.TrimSpace(run.FailureCode),
		ImageRef:           strings.TrimSpace(run.ImageRef),
		ImageDigest:        strings.TrimSpace(run.ImageDigest),
		SourceBranch:       strings.TrimSpace(run.SourceBranch),
		SourceTag:          strings.TrimSpace(run.SourceTag),
		SourceCommit:       strings.TrimSpace(run.SourceCommit),
		StartedAt:          run.StartedAt,
		FinishedAt:         run.FinishedAt,
		UpdatedAt:          run.UpdatedAt,
	}
}

func releaseTaskStatusInternal(release model.Release) aiTaskStatusInternal {
	taskStatus, terminal := normalizeAITaskStatus(release.Status)
	return aiTaskStatusInternal{
		TaskType:           "release",
		TaskID:             release.ID,
		ProjectID:          release.ProjectID,
		ApplicationID:      strings.TrimSpace(release.ApplicationID),
		DeploymentTargetID: strings.TrimSpace(release.DeploymentTargetID),
		RawStatus:          strings.TrimSpace(release.Status),
		TaskStatus:         taskStatus,
		Terminal:           terminal,
		FailureCode:        strings.TrimSpace(release.FailureCode),
		ImageRef:           strings.TrimSpace(release.ImageRef),
		BuildRunID:         strings.TrimSpace(release.BuildRunID),
		Revision:           release.Revision,
		ReleaseType:        strings.TrimSpace(release.Type),
		StartedAt:          release.StartedAt,
		FinishedAt:         release.FinishedAt,
		UpdatedAt:          release.UpdatedAt,
	}
}

func normalizeAITaskStatus(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "queued", "pending":
		return "pending", false
	case "running", "progressing", "deploying", "building":
		return "running", false
	case "succeeded", "success", "completed", "ready":
		return "succeeded", true
	case "canceled", "cancelled":
		return "canceled", true
	default:
		return "failed", true
	}
}

func (h *Handler) GetAIProgress(ctx *gin.Context) {
	_, project, ok := h.host.AuthorizeProjectRef(ctx, strings.TrimSpace(ctx.Param("projectId")), authz.ActionProjectRead)
	if !ok {
		return
	}
	snapshot, err := h.resolveAIProgress(ctx, project.ID)
	if err != nil {
		h.writeAIProgressError(ctx, err)
		return
	}
	ctx.Header("Cache-Control", "no-store")
	ctx.JSON(http.StatusOK, snapshot)
}

func (h *Handler) StreamAIProgress(ctx *gin.Context) {
	user, project, ok := h.host.AuthorizeProjectRef(ctx, strings.TrimSpace(ctx.Param("projectId")), authz.ActionProjectRead)
	if !ok {
		return
	}
	initial, err := h.resolveAIProgress(ctx, project.ID)
	if err != nil {
		h.writeAIProgressError(ctx, err)
		return
	}
	streamCtx, cancelStream := context.WithCancel(ctx.Request.Context())
	defer cancelStream()
	restoreRequestContext := replaceRequestContext(ctx, streamCtx)
	defer restoreRequestContext()
	authorizationRevoked, authorizationActive := h.host.MonitorProjectAuthorization(
		ctx, streamCtx, user, project.ID, authz.ActionProjectRead, cancelStream,
	)
	if !authorizationActive {
		h.host.WriteContinuousAuthorizationRevoked(ctx)
		return
	}

	writer := ctx.Writer
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.Header().Set("Cache-Control", "no-store, no-transform")
	writer.Header().Set("Pragma", "no-cache")
	writer.Header().Set("Connection", "keep-alive")
	writer.Header().Set("X-Accel-Buffering", "no")

	lastRevision := initial.Revision
	ticker := time.NewTicker(aiProgressPollInterval)
	defer ticker.Stop()
	writeSSE(writer, "progress.snapshot", initial.Revision, initial)
	flushSSE(writer)
	if aiProgressTerminal(initial.State) {
		return
	}

	for {
		select {
		case <-authorizationRevoked:
			return
		case <-streamCtx.Done():
			return
		case <-ticker.C:
		}

		snapshot, err := h.resolveAIProgress(ctx, project.ID)
		if err != nil {
			if streamCtx.Err() != nil {
				return
			}
			span := trace.SpanFromContext(ctx.Request.Context())
			span.SetStatus(codes.Error, "ai progress observation failed")
			span.SetAttributes(attribute.String("error.code", aiProgressErrorCode(err)))
			writeSSE(writer, "progress.error", strconv.FormatInt(time.Now().UnixMicro(), 10), gin.H{"code": aiProgressErrorCode(err)})
			flushSSE(writer)
			return
		}

		revision := snapshot.Revision
		if revision == lastRevision {
			continue
		}
		writeSSE(writer, "progress.snapshot", snapshot.Revision, snapshot)
		flushSSE(writer)
		lastRevision = revision
		if aiProgressTerminal(snapshot.State) {
			return
		}
	}
}

func (h *Handler) resolveAIProgress(ctx *gin.Context, projectID string) (aiProgressSnapshot, error) {
	operationType := strings.TrimSpace(ctx.Param("operationType"))
	operationID := strings.TrimSpace(ctx.Param("operationId"))
	if projectID == "" {
		projectID = strings.TrimSpace(ctx.Param("projectId"))
	}
	if operationID == "" {
		return aiProgressSnapshot{}, errAIProgressNotFound
	}

	switch operationType {
	case "build_run":
		var run model.BuildRun
		if err := h.dbFor(ctx).First(&run, "id = ? and project_id = ?", operationID, projectID).Error; err != nil {
			return aiProgressSnapshot{}, normalizeAIProgressDBError(err)
		}
		return buildRunProgress(run), nil
	case "release":
		var release model.Release
		if err := h.dbFor(ctx).First(&release, "id = ? and project_id = ?", operationID, projectID).Error; err != nil {
			return aiProgressSnapshot{}, normalizeAIProgressDBError(err)
		}
		return releaseProgress(release), nil
	case "hook_run":
		var run model.HookRun
		if err := h.dbFor(ctx).First(&run, "id = ? and project_id = ?", operationID, projectID).Error; err != nil {
			return aiProgressSnapshot{}, normalizeAIProgressDBError(err)
		}
		return hookRunProgress(run), nil
	case "app_template_installation":
		var installation model.AppTemplateInstallation
		if err := h.dbFor(ctx).First(&installation, "id = ? and project_id = ?", operationID, projectID).Error; err != nil {
			return aiProgressSnapshot{}, normalizeAIProgressDBError(err)
		}
		var components []model.AppTemplateInstallationComponent
		if err := h.dbFor(ctx).Where("installation_id = ? and project_id = ?", installation.ID, projectID).Find(&components).Error; err != nil {
			return aiProgressSnapshot{}, err
		}
		releaseIDs := appTemplateInstallationReleaseIDs(installation, components)
		if len(releaseIDs) > 0 {
			var releases []model.Release
			if err := h.dbFor(ctx).Where("id in ? and project_id = ?", releaseIDs, projectID).Find(&releases).Error; err != nil {
				return aiProgressSnapshot{}, err
			}
			if len(releases) != len(releaseIDs) {
				return aiProgressSnapshot{}, errAIProgressNotFound
			}
			return appTemplateReleaseProgress(installation, releases), nil
		}
		return appTemplateInstallationProgress(installation), nil
	default:
		return aiProgressSnapshot{}, errAIProgressTypeInvalid
	}
}

var (
	errAIProgressNotFound    = errors.New("ai progress operation not found")
	errAIProgressTypeInvalid = errors.New("ai progress operation type invalid")
)

func normalizeAIProgressDBError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errAIProgressNotFound
	}
	return err
}

func (h *Handler) writeAIProgressError(ctx *gin.Context, err error) {
	switch {
	case errors.Is(err, errAIProgressNotFound):
		writeErrorCode(ctx, http.StatusNotFound, "ai.progress.not_found", "progress operation not found")
	case errors.Is(err, errAIProgressTypeInvalid):
		writeErrorCode(ctx, http.StatusBadRequest, "ai.progress.type_invalid", "progress operation type is invalid")
	default:
		writeErrorCode(ctx, http.StatusInternalServerError, "ai.progress.unavailable", "progress observation is unavailable")
	}
}

func aiProgressErrorCode(err error) string {
	switch {
	case errors.Is(err, errAIProgressNotFound):
		return "ai.progress.not_found"
	case errors.Is(err, errAIProgressTypeInvalid):
		return "ai.progress.type_invalid"
	default:
		return "ai.progress.unavailable"
	}
}

func buildRunProgress(run model.BuildRun) aiProgressSnapshot {
	state := normalizeAIProgressState(run.Status)
	steps := threeStageProgress("build", state)
	return progressSnapshot(run.ID, "build_run", state, "ai.progress.build."+state, run.FailureCode, run.StartedAt, run.FinishedAt, run.UpdatedAt, steps)
}

func releaseProgress(release model.Release) aiProgressSnapshot {
	state := normalizeAIProgressState(release.Status)
	steps := threeStageProgress("release", state)
	return progressSnapshot(release.ID, "release", state, "ai.progress.release."+state, release.FailureCode, release.StartedAt, release.FinishedAt, release.UpdatedAt, steps)
}

func hookRunProgress(run model.HookRun) aiProgressSnapshot {
	state := normalizeAIProgressState(run.Status)
	steps := threeStageProgress("hook", state)
	return progressSnapshot(run.ID, "hook_run", state, "ai.progress.hook."+state, "", run.StartedAt, run.FinishedAt, run.UpdatedAt, steps)
}

// appTemplateInstallationProgress 描述未创建 Release 的安装（installNow=false）：资源已在安装事务内落库即完成。
func appTemplateInstallationProgress(installation model.AppTemplateInstallation) aiProgressSnapshot {
	const state = "succeeded"
	steps := threeStageProgress("installation", state)
	return progressSnapshot(installation.ID, "app_template_installation", state, "ai.progress.installation."+state, "", nil, terminalTime(state, installation.UpdatedAt), installation.UpdatedAt, steps)
}

// appTemplateInstallationReleaseIDs 返回一次模板安装创建的全部 Release；多组件安装按组件记录汇总。
func appTemplateInstallationReleaseIDs(installation model.AppTemplateInstallation, components []model.AppTemplateInstallationComponent) []string {
	if len(components) == 0 {
		if installation.ReleaseID == "" {
			return nil
		}
		return []string{installation.ReleaseID}
	}
	ids := make([]string, 0, len(components))
	for _, component := range components {
		if component.ReleaseID != "" {
			ids = append(ids, component.ReleaseID)
		}
	}
	return ids
}

// appTemplateReleaseProgress 汇总安装创建的所有 Release：任一失败即失败，全部成功才成功。
func appTemplateReleaseProgress(installation model.AppTemplateInstallation, releases []model.Release) aiProgressSnapshot {
	statePriority := map[string]int{"failed": 6, "cancelled": 5, "waiting_approval": 4, "waiting_input": 3, "running": 2, "queued": 1, "succeeded": 0}
	state := "succeeded"
	failureCode := ""
	var startedAt, finishedAt *time.Time
	updatedAt := installation.UpdatedAt
	for _, release := range releases {
		releaseState := normalizeAIProgressState(release.Status)
		if statePriority[releaseState] > statePriority[state] {
			state = releaseState
		}
		if releaseState == "failed" && failureCode == "" && strings.TrimSpace(release.FailureCode) != "" {
			failureCode = strings.TrimSpace(release.FailureCode)
		}
		if release.StartedAt != nil && (startedAt == nil || release.StartedAt.Before(*startedAt)) {
			startedAt = release.StartedAt
		}
		if release.FinishedAt != nil && (finishedAt == nil || release.FinishedAt.After(*finishedAt)) {
			finishedAt = release.FinishedAt
		}
		if release.UpdatedAt.After(updatedAt) {
			updatedAt = release.UpdatedAt
		}
	}
	if !aiProgressTerminal(state) {
		finishedAt = nil
	}
	steps := threeStageProgress("installation", state)
	return progressSnapshot(installation.ID, "app_template_installation", state, "ai.progress.installation."+state, failureCode, startedAt, finishedAt, updatedAt, steps)
}

func progressSnapshot(operationID, operationType, state, stageCode, failureCode string, startedAt, finishedAt *time.Time, updatedAt time.Time, steps []aiProgressStep) aiProgressSnapshot {
	progress := aiProgressValue{Mode: "indeterminate"}
	if state == "succeeded" {
		value := float64(100)
		progress = aiProgressValue{Mode: "determinate", Value: &value}
	}
	result := aiProgressSnapshot{
		OperationID: operationID, OperationType: operationType, Revision: aiProgressRevision(updatedAt),
		State: state, StageCode: stageCode, Progress: progress, Steps: steps,
		StartedAt: startedAt, UpdatedAt: updatedAt, FinishedAt: finishedAt,
	}
	if state == "failed" {
		code := strings.TrimSpace(failureCode)
		if code == "" {
			code = "ai.progress.operation_failed"
		}
		result.Error = &aiProgressError{Code: code}
	}
	return result
}

func aiProgressRevision(value time.Time) string {
	return value.UTC().Format("20060102T150405.000000000Z")
}

func normalizeAIProgressState(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "queued", "pending":
		return "queued"
	case "running", "progressing", "deploying", "building":
		return "running"
	case "succeeded", "success", "completed", "installed", "ready":
		return "succeeded"
	case "canceled", "cancelled":
		return "cancelled"
	case "waiting_input":
		return "waiting_input"
	case "waiting_approval":
		return "waiting_approval"
	default:
		return "failed"
	}
}

func threeStageProgress(prefix, state string) []aiProgressStep {
	steps := []aiProgressStep{
		{ID: "queued", LabelCode: "ai.progress." + prefix + ".queued", Status: "pending"},
		{ID: "running", LabelCode: "ai.progress." + prefix + ".running", Status: "pending"},
		{ID: "completed", LabelCode: "ai.progress." + prefix + ".succeeded", Status: "pending"},
	}
	switch state {
	case "queued":
		steps[0].Status = "running"
	case "running", "waiting_input", "waiting_approval":
		steps[0].Status = "success"
		steps[1].Status = "running"
	case "succeeded":
		for index := range steps {
			steps[index].Status = "success"
		}
	case "failed":
		steps[0].Status = "success"
		steps[1].Status = "error"
		steps[2].Status = "skipped"
	case "cancelled":
		steps[0].Status = "success"
		steps[1].Status = "warning"
		steps[2].Status = "skipped"
	}
	return steps
}

func aiProgressTerminal(state string) bool {
	return state == "succeeded" || state == "failed" || state == "cancelled"
}

func terminalTime(state string, value time.Time) *time.Time {
	if !aiProgressTerminal(state) {
		return nil
	}
	return &value
}
