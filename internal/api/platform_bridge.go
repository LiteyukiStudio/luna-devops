package api

import (
	"context"
	"time"

	"github.com/LiteyukiStudio/devops/internal/api/platformapi"
	"github.com/LiteyukiStudio/devops/internal/model"
	projectservice "github.com/LiteyukiStudio/devops/internal/project"
	"github.com/LiteyukiStudio/devops/internal/retention"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type platformHost struct {
	handlers *Handlers
}

func (host platformHost) Config() Config                  { return host.handlers.config }
func (host platformHost) DBFor(ctx *gin.Context) *gorm.DB { return host.handlers.dbFor(ctx) }
func (host platformHost) DBWithContext(ctx context.Context) *gorm.DB {
	return host.handlers.dbWithContext(ctx)
}
func (host platformHost) CurrentUser(ctx *gin.Context) (model.User, bool) {
	return host.handlers.currentUser(ctx)
}
func (host platformHost) ResolveListVisibility(ctx *gin.Context, user model.User) (projectservice.ListVisibility, bool) {
	return resolveListVisibility(ctx, user)
}
func (host platformHost) ProjectIDsForUser(ctx context.Context, userID string) []string {
	return host.handlers.projectIDsForUser(ctx, userID)
}
func (host platformHost) EnsurePlatformSystemProject(user model.User, ctx context.Context) (model.Project, error) {
	return host.handlers.ensurePlatformSystemProject(user, ctx)
}
func (host platformHost) ObserveRuntimeClusters(ctx context.Context, clusters []model.RuntimeCluster) {
	host.handlers.observeRuntimeClusters(ctx, clusters)
}
func (host platformHost) DashboardRegistryAvailable(ctx context.Context, user model.User, registry model.ArtifactRegistry) bool {
	return host.handlers.pingRegistry(ctx, user, registry).Success
}
func (host platformHost) RequirePlatformAdmin(ctx *gin.Context) bool {
	return host.handlers.requirePlatformAdmin(ctx)
}
func (host platformHost) AuditWithContext(userID, action, resource string, success bool, message string, ctx context.Context) {
	host.handlers.auditWithContext(userID, action, resource, success, message, ctx)
}
func (host platformHost) AllowRate(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	return host.handlers.rateLimiter.allow(ctx, key, limit, window)
}

func (h *Handlers) platformAPI() *platformapi.Handler {
	return platformapi.New(platformHost{handlers: h})
}

func (h *Handlers) GetAPIMeta(ctx *gin.Context) { h.platformAPI().GetAPIMeta(ctx) }
func (h *Handlers) ListDataRetentionCatalog(ctx *gin.Context) {
	h.platformAPI().ListDataRetentionCatalog(ctx)
}
func (h *Handlers) PreviewDataRetention(ctx *gin.Context) {
	h.platformAPI().PreviewDataRetention(ctx)
}
func (h *Handlers) CleanupDataRetention(ctx *gin.Context) {
	h.platformAPI().CleanupDataRetention(ctx)
}
func (h *Handlers) RelayBrowserTraces(ctx *gin.Context) {
	h.platformAPI().RelayBrowserTraces(ctx)
}
func (h *Handlers) ListPlatformEvents(ctx *gin.Context) {
	h.platformAPI().ListPlatformEvents(ctx)
}
func (h *Handlers) GetPlatformEvent(ctx *gin.Context) {
	h.platformAPI().GetPlatformEvent(ctx)
}
func (h *Handlers) ListPlatformEventCatalog(ctx *gin.Context) {
	h.platformAPI().ListPlatformEventCatalog(ctx)
}
func (h *Handlers) GetDashboard(ctx *gin.Context) { h.platformAPI().GetDashboard(ctx) }

const minimumCLIVersion = platformapi.MinimumCLIVersion

type apiMetaResponse = platformapi.APIMetaResponse
type dataRetentionRequest = platformapi.DataRetentionRequest
type dataRetentionRange = platformapi.DataRetentionRange
type platformEventResponse = platformapi.PlatformEventResponse

func openAPIDigest() string { return platformapi.OpenAPIDigest() }
func parseDataRetentionRange(input dataRetentionRequest) (dataRetentionRange, error) {
	return platformapi.ParseDataRetentionRange(input)
}
func dataRetentionFailureSummary(err error) string {
	return platformapi.DataRetentionFailureSummary(err)
}
func writeDataRetentionError(ctx *gin.Context, err error) {
	platformapi.WriteDataRetentionError(ctx, err)
}
func dataRetentionResultSummary(items []retention.Result) string {
	return platformapi.DataRetentionResultSummary(items)
}
func browserTraceMediaType(value string) (string, bool) {
	return platformapi.BrowserTraceMediaType(value)
}
func browserTraceEndpoint(cfg Config) (string, error) {
	return platformapi.BrowserTraceEndpoint(cfg)
}
func (h *Handlers) platformEventsVisibleTo(user model.User, visibility projectservice.ListVisibility, ctx context.Context) *gorm.DB {
	return h.platformAPI().PlatformEventsVisibleTo(user, visibility, ctx)
}
func (h *Handlers) canReadPlatformEvent(ctx context.Context, user model.User, event model.PlatformEvent) bool {
	return h.platformAPI().CanReadPlatformEvent(ctx, user, event)
}
func canReadPlatformEventForUser(user model.User, event model.PlatformEvent, projectIDs []string) bool {
	return platformapi.CanReadPlatformEventForUser(user, event, projectIDs)
}
func applyPlatformEventFilters(ctx *gin.Context, query *gorm.DB) *gorm.DB {
	return platformapi.ApplyPlatformEventFilters(ctx, query)
}
func platformEventFilterValues(ctx *gin.Context, singular, plural string) []string {
	return platformapi.PlatformEventFilterValues(ctx, singular, plural)
}
func parsePlatformEventTime(raw string, endOfDay bool) (time.Time, bool) {
	return platformapi.ParsePlatformEventTime(raw, endOfDay)
}
func platformEventResponseFor(event model.PlatformEvent, deliveryCount int64) platformEventResponse {
	return platformapi.PlatformEventResponseFor(event, deliveryCount)
}
func platformEventLinks(event model.PlatformEvent, links map[string]string) map[string]string {
	return platformapi.PlatformEventLinks(event, links)
}
