package deploymentapi

import (
	"net/http"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *Handlers) findReleaseByIdempotency(ctx *gin.Context, userID, projectID, key, requestHash string) (model.Release, bool, bool) {
	var release model.Release
	err := h.dbFor(ctx).First(&release, "created_by = ? and idempotency_key = ?", userID, key).Error
	if err == gorm.ErrRecordNotFound {
		return model.Release{}, false, true
	}
	if err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "deployment.idempotency_unavailable", "release idempotency state is unavailable")
		return model.Release{}, false, false
	}
	if release.ProjectID != projectID || release.IdempotencyRequestHash != requestHash {
		writeErrorCode(ctx, http.StatusConflict, "request.idempotency_conflict", "idempotency key belongs to another request")
		return model.Release{}, false, false
	}
	if release.Status == "pending" && !h.enqueueDeployRun(ctx.Request.Context(), release) {
		writeError(ctx, http.StatusServiceUnavailable, "部署队列暂不可用")
		return model.Release{}, false, false
	}
	return release, true, true
}
