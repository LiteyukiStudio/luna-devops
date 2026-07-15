package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/retention"
	"github.com/gin-gonic/gin"
)

const dataRetentionCleanupAction = "data_retention.cleanup"

type dataRetentionRequest struct {
	Datasets []string `json:"datasets"`
	StartAt  string   `json:"startAt"`
	EndAt    string   `json:"endAt"`
}

type dataRetentionRange struct {
	Datasets []string
	StartAt  time.Time
	EndAt    time.Time
}

func (h *Handlers) ListDataRetentionCatalog(ctx *gin.Context) {
	if !h.requirePlatformAdmin(ctx) {
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"items": retention.Catalog()})
}

func (h *Handlers) PreviewDataRetention(ctx *gin.Context) {
	if !h.requirePlatformAdmin(ctx) {
		return
	}
	input, ok := bindDataRetentionRange(ctx)
	if !ok {
		return
	}

	items, err := retention.NewService(h.db).Preview(ctx.Request.Context(), input.Datasets, input.StartAt, input.EndAt, time.Now())
	if err != nil {
		writeDataRetentionError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *Handlers) CleanupDataRetention(ctx *gin.Context) {
	if !h.requirePlatformAdmin(ctx) {
		return
	}
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if !h.requireStepUp(ctx, user, stepUpPurposeDataRetentionCleanup) {
		return
	}
	input, ok := bindDataRetentionRange(ctx)
	if !ok {
		h.audit(user.ID, dataRetentionCleanupAction, "data_retention", false, "invalid cleanup request")
		return
	}

	items, err := retention.NewService(h.db).Cleanup(ctx.Request.Context(), input.Datasets, input.StartAt, input.EndAt, time.Now())
	if err != nil {
		h.audit(user.ID, dataRetentionCleanupAction, "data_retention", false, dataRetentionFailureSummary(err))
		writeDataRetentionError(ctx, err)
		return
	}
	h.audit(user.ID, dataRetentionCleanupAction, "data_retention", true, dataRetentionResultSummary(items))
	ctx.JSON(http.StatusOK, gin.H{"items": items})
}

func bindDataRetentionRange(ctx *gin.Context) (dataRetentionRange, bool) {
	var input dataRetentionRequest
	if !bindJSON(ctx, &input) {
		return dataRetentionRange{}, false
	}
	parsed, err := parseDataRetentionRange(input)
	if err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "retention.invalid_range", err.Error())
		return dataRetentionRange{}, false
	}
	return parsed, true
}

func parseDataRetentionRange(input dataRetentionRequest) (dataRetentionRange, error) {
	startAt, err := time.Parse(time.RFC3339, strings.TrimSpace(input.StartAt))
	if err != nil {
		return dataRetentionRange{}, fmt.Errorf("startAt must be RFC3339")
	}
	endAt, err := time.Parse(time.RFC3339, strings.TrimSpace(input.EndAt))
	if err != nil {
		return dataRetentionRange{}, fmt.Errorf("endAt must be RFC3339")
	}
	if !startAt.Before(endAt) {
		return dataRetentionRange{}, fmt.Errorf("startAt must be before endAt")
	}
	return dataRetentionRange{Datasets: input.Datasets, StartAt: startAt, EndAt: endAt}, nil
}

func writeDataRetentionError(ctx *gin.Context, err error) {
	switch {
	case errors.Is(err, retention.ErrInvalidRange):
		writeErrorCode(ctx, http.StatusBadRequest, "retention.invalid_range", err.Error())
	case errors.Is(err, retention.ErrUnknownDataset), errors.Is(err, retention.ErrNoDatasets):
		writeErrorCode(ctx, http.StatusBadRequest, "retention.invalid_dataset", err.Error())
	default:
		writeErrorCode(ctx, http.StatusInternalServerError, "retention.cleanup_failed", "data retention operation failed")
	}
}

func dataRetentionFailureSummary(err error) string {
	switch {
	case errors.Is(err, retention.ErrInvalidRange):
		return "cleanup rejected: invalid time range"
	case errors.Is(err, retention.ErrUnknownDataset), errors.Is(err, retention.ErrNoDatasets):
		return "cleanup rejected: invalid dataset"
	default:
		return "cleanup failed"
	}
}

func dataRetentionResultSummary(items []retention.Result) string {
	var matched, deleted int64
	for _, item := range items {
		matched += item.Matched
		deleted += item.Deleted
	}
	return fmt.Sprintf("datasets=%d matched=%d deleted=%d", len(items), matched, deleted)
}
