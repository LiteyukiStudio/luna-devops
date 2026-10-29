package buildapi

import (
	"net/http"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *Handlers) findBuildRunByIdempotency(ctx *gin.Context, userID, projectID, key, requestHash string) (model.BuildRun, bool, bool) {
	var run model.BuildRun
	err := h.dbFor(ctx).First(&run, "created_by = ? and idempotency_key = ?", userID, key).Error
	if err == gorm.ErrRecordNotFound {
		return model.BuildRun{}, false, true
	}
	if err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "build.idempotency_unavailable", "build idempotency state is unavailable")
		return model.BuildRun{}, false, false
	}
	if run.ProjectID != projectID || run.IdempotencyRequestHash != requestHash {
		writeErrorCode(ctx, http.StatusConflict, "request.idempotency_conflict", "idempotency key belongs to another request")
		return model.BuildRun{}, false, false
	}
	if run.Status == "queued" {
		var job model.BuildJob
		if err := h.dbFor(ctx).First(&job, "build_run_id = ? and project_id = ?", run.ID, run.ProjectID).Error; err != nil {
			writeErrorCode(ctx, http.StatusInternalServerError, "build.idempotency_unavailable", "build job state is unavailable")
			return model.BuildRun{}, false, false
		}
		if job.Status == "queued" {
			if !h.host.BuildQueueAvailable() {
				writeError(ctx, http.StatusServiceUnavailable, "构建任务队列未配置")
				return model.BuildRun{}, false, false
			}
			if err := h.host.EnqueueBuildRun(ctx.Request.Context(), tasks.BuildRunPayload{
				BuildRunID: run.ID,
				BuildJobID: job.ID,
				ProjectID:  run.ProjectID,
				ActorID:    run.CreatedBy,
			}); err != nil {
				writeError(ctx, http.StatusServiceUnavailable, "构建任务投递失败")
				return model.BuildRun{}, false, false
			}
		}
	}
	return run, true, true
}
