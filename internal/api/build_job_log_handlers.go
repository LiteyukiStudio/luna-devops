package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (h *Handlers) ListBuildJobs(ctx *gin.Context) {
	if _, ok := h.findProjectForCurrentUser(ctx); !ok {
		return
	}
	pagination := paginationFromQueryWithSort(ctx, map[string]string{"createdAt": "created_at", "status": "status", "attempts": "attempts"}, "createdAt")
	query := h.dbFor(ctx).Where("project_id = ?", ctx.Param("projectId"))
	if runID := strings.TrimSpace(ctx.Query("buildRunId")); runID != "" {
		query = query.Where("build_run_id = ?", runID)
	}
	if applicationID := strings.TrimSpace(ctx.Query("applicationId")); applicationID != "" {
		buildRuns := h.dbFor(ctx).Model(&model.BuildRun{}).Select("id").
			Where("project_id = ? and application_id = ?", ctx.Param("projectId"), applicationID)
		query = query.Where("build_run_id in (?)", buildRuns)
	}
	var jobs []model.BuildJob
	var total int64
	if err := query.Model(&model.BuildJob{}).Count(&total).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	orderBy := orderByClause(pagination, map[string]string{
		"createdAt": "created_at",
		"status":    "status",
		"attempts":  "attempts",
	}, "created_at")
	if err := query.Order(orderBy).Limit(pagination.PageSize).Offset(pagination.Offset()).Find(&jobs).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, paginatedResponse(jobs, total, pagination))
}

func (h *Handlers) GetBuildJob(ctx *gin.Context) {
	if _, ok := h.findProjectForCurrentUser(ctx); !ok {
		return
	}
	var job model.BuildJob
	if err := h.dbFor(ctx).First(&job, "id = ? and project_id = ?", ctx.Param("jobId"), ctx.Param("projectId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "build job not found")
		return
	}
	ctx.JSON(http.StatusOK, job)
}

func (h *Handlers) GetBuildJobLogs(ctx *gin.Context) {
	if _, ok := h.findProjectForCurrentUser(ctx); !ok {
		return
	}
	var log model.BuildLog
	if err := h.dbFor(ctx).First(&log, "build_job_id = ? and project_id = ?", ctx.Param("jobId"), ctx.Param("projectId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "build log not found")
		return
	}
	ctx.JSON(http.StatusOK, log)
}

func (h *Handlers) StreamBuildJobLogs(ctx *gin.Context) {
	if _, ok := h.findProjectForCurrentUser(ctx); !ok {
		return
	}
	var job model.BuildJob
	if err := h.dbFor(ctx).First(&job, "id = ? and project_id = ?", ctx.Param("jobId"), ctx.Param("projectId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "build job not found")
		return
	}
	offset := buildLogStreamOffset(ctx)
	writer := ctx.Writer
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.Header().Set("Cache-Control", "no-cache")
	writer.Header().Set("Connection", "keep-alive")
	writer.Header().Set("X-Accel-Buffering", "no")
	writer.WriteHeader(http.StatusOK)
	flushSSE(writer)

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		nextOffset, sent, err := h.writeBuildLogStreamChunk(ctx, job, offset)
		if err != nil {
			writeSSE(writer, "error", strconv.Itoa(offset), map[string]string{"code": "build.logs.stream_error"})
			flushSSE(writer)
			return
		}
		offset = nextOffset
		if buildJobTerminal(job.Status) {
			writeSSE(writer, "done", strconv.Itoa(offset), map[string]string{"status": job.Status})
			flushSSE(writer)
			return
		}
		if sent {
			flushSSE(writer)
		}
		select {
		case <-ctx.Request.Context().Done():
			return
		case <-ticker.C:
			if err := h.dbFor(ctx).Select("status").First(&job, "id = ? and project_id = ?", job.ID, job.ProjectID).Error; err != nil {
				return
			}
		}
	}
}

func (h *Handlers) writeBuildLogStreamChunk(ctx *gin.Context, job model.BuildJob, offset int) (int, bool, error) {
	var log model.BuildLog
	if err := h.dbFor(ctx).First(&log, "build_job_id = ? and project_id = ?", job.ID, job.ProjectID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return offset, false, nil
		}
		return offset, false, err
	}
	content := log.Content
	if offset < 0 || offset > len(content) {
		offset = len(content)
	}
	if len(content) == offset {
		return offset, false, nil
	}
	nextOffset := len(content)
	writeSSE(ctx.Writer, "chunk", strconv.Itoa(nextOffset), map[string]any{
		"content": content[offset:],
		"offset":  nextOffset,
	})
	return nextOffset, true, nil
}

func buildLogStreamOffset(ctx *gin.Context) int {
	value := strings.TrimSpace(ctx.Query("after"))
	if value == "" {
		value = strings.TrimSpace(ctx.GetHeader("Last-Event-ID"))
	}
	offset, err := strconv.Atoi(value)
	if err != nil || offset < 0 {
		return 0
	}
	return offset
}

func buildJobTerminal(status string) bool {
	return status == "succeeded" || status == "failed" || status == "canceled" || status == "lost" || status == "timeout"
}

func writeSSE(writer http.ResponseWriter, event string, idValue string, data any) {
	payload, _ := json.Marshal(data)
	if idValue != "" {
		_, _ = fmt.Fprintf(writer, "id: %s\n", idValue)
	}
	if event != "" {
		_, _ = fmt.Fprintf(writer, "event: %s\n", event)
	}
	_, _ = fmt.Fprintf(writer, "data: %s\n\n", payload)
}

func flushSSE(writer http.ResponseWriter) {
	if flusher, ok := writer.(http.Flusher); ok {
		flusher.Flush()
	}
}
