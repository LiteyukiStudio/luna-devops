package platformapi

import (
	"context"
	"time"

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
type PlatformEventResponse = platformEventResponse

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

func (h *Handler) CanReadPlatformEvent(ctx context.Context, user model.User, event model.PlatformEvent) bool {
	return h.canReadPlatformEvent(ctx, user, event)
}

func CanReadPlatformEventForUser(user model.User, event model.PlatformEvent, projectIDs []string) bool {
	return canReadPlatformEventForUser(user, event, projectIDs)
}

func ApplyPlatformEventFilters(ctx *gin.Context, query *gorm.DB) *gorm.DB {
	return applyPlatformEventFilters(ctx, query)
}

func PlatformEventFilterValues(ctx *gin.Context, singular, plural string) []string {
	return platformEventFilterValues(ctx, singular, plural)
}

func ParsePlatformEventTime(raw string, endOfDay bool) (time.Time, bool) {
	return parsePlatformEventTime(raw, endOfDay)
}

func PlatformEventResponseFor(event model.PlatformEvent, deliveryCount int64) PlatformEventResponse {
	return platformEventResponseFor(event, deliveryCount)
}

func PlatformEventLinks(event model.PlatformEvent, links map[string]string) map[string]string {
	return platformEventLinks(event, links)
}
