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

func (h *Handler) GetAIProgress(ctx *gin.Context) {
	if _, _, ok := h.host.AuthorizeProject(ctx, authz.ActionProjectRead); !ok {
		return
	}
	snapshot, err := h.resolveAIProgress(ctx)
	if err != nil {
		h.writeAIProgressError(ctx, err)
		return
	}
	ctx.Header("Cache-Control", "no-store")
	ctx.JSON(http.StatusOK, snapshot)
}

func (h *Handler) StreamAIProgress(ctx *gin.Context) {
	user, project, ok := h.host.AuthorizeProject(ctx, authz.ActionProjectRead)
	if !ok {
		return
	}
	initial, err := h.resolveAIProgress(ctx)
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

		snapshot, err := h.resolveAIProgress(ctx)
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

func (h *Handler) resolveAIProgress(ctx *gin.Context) (aiProgressSnapshot, error) {
	operationType := strings.TrimSpace(ctx.Param("operationType"))
	operationID := strings.TrimSpace(ctx.Param("operationId"))
	projectID := strings.TrimSpace(ctx.Param("projectId"))
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
		if installation.ReleaseID != "" {
			var release model.Release
			if err := h.dbFor(ctx).First(&release, "id = ? and project_id = ?", installation.ReleaseID, projectID).Error; err != nil {
				return aiProgressSnapshot{}, normalizeAIProgressDBError(err)
			}
			return appTemplateReleaseProgress(installation, release), nil
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
	return progressSnapshot(run.ID, "build_run", state, "ai.progress.build."+state, run.StartedAt, run.FinishedAt, run.UpdatedAt, steps)
}

func releaseProgress(release model.Release) aiProgressSnapshot {
	state := normalizeAIProgressState(release.Status)
	steps := threeStageProgress("release", state)
	return progressSnapshot(release.ID, "release", state, "ai.progress.release."+state, release.StartedAt, release.FinishedAt, release.UpdatedAt, steps)
}

func hookRunProgress(run model.HookRun) aiProgressSnapshot {
	state := normalizeAIProgressState(run.Status)
	steps := threeStageProgress("hook", state)
	return progressSnapshot(run.ID, "hook_run", state, "ai.progress.hook."+state, run.StartedAt, run.FinishedAt, run.UpdatedAt, steps)
}

func appTemplateInstallationProgress(installation model.AppTemplateInstallation) aiProgressSnapshot {
	state := "running"
	switch installation.Status {
	case "installed", "succeeded":
		state = "succeeded"
	case "deploy_failed", "failed":
		state = "failed"
	case "canceled", "cancelled":
		state = "cancelled"
	case "pending", "queued":
		state = "queued"
	}
	steps := threeStageProgress("installation", state)
	return progressSnapshot(installation.ID, "app_template_installation", state, "ai.progress.installation."+state, nil, terminalTime(state, installation.UpdatedAt), installation.UpdatedAt, steps)
}

func appTemplateReleaseProgress(installation model.AppTemplateInstallation, release model.Release) aiProgressSnapshot {
	state := normalizeAIProgressState(release.Status)
	steps := threeStageProgress("installation", state)
	return progressSnapshot(installation.ID, "app_template_installation", state, "ai.progress.installation."+state, release.StartedAt, release.FinishedAt, release.UpdatedAt, steps)
}

func progressSnapshot(operationID, operationType, state, stageCode string, startedAt, finishedAt *time.Time, updatedAt time.Time, steps []aiProgressStep) aiProgressSnapshot {
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
		result.Error = &aiProgressError{Code: "ai.progress.operation_failed"}
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
