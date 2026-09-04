package platformapi

import (
	"context"

	sharedconfig "github.com/LiteyukiStudio/devops/internal/config"
	"github.com/LiteyukiStudio/devops/internal/model"
	projectservice "github.com/LiteyukiStudio/devops/internal/project"
	"github.com/LiteyukiStudio/devops/internal/retention"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const MinimumCLIVersion = minimumCLIVersion

type APIMetaResponse = apiMetaResponse
type DataRetentionRequest = dataRetentionRequest
type DataRetentionRange = dataRetentionRange

func OpenAPIDigest() string { return openAPIDigest() }

func ParseDataRetentionRange(input DataRetentionRequest) (DataRetentionRange, error) {
	return parseDataRetentionRange(input)
}

func DataRetentionFailureSummary(err error) string { return dataRetentionFailureSummary(err) }

func WriteDataRetentionError(ctx *gin.Context, err error) { writeDataRetentionError(ctx, err) }

func DataRetentionResultSummary(items []retention.Result) string {
	return dataRetentionResultSummary(items)
}

func BrowserTraceMediaType(value string) (string, bool) { return browserTraceMediaType(value) }

func BrowserTraceEndpoint(cfg sharedconfig.APIConfig) (string, error) {
	return browserTraceEndpoint(cfg)
}

func (h *Handler) PlatformEventsVisibleTo(user model.User, visibility projectservice.ListVisibility, ctx context.Context) *gorm.DB {
	return h.platformEventsVisibleTo(user, visibility, ctx)
}
