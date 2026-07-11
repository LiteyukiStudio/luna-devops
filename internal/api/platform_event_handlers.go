package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/platformevent"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type platformEventResponse struct {
	model.PlatformEvent
	Detail        any               `json:"detail"`
	Links         map[string]string `json:"links"`
	DeliveryCount int64             `json:"deliveryCount"`
}

func (h *Handlers) ListPlatformEvents(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	pagination := paginationFromQuery(ctx)
	query := h.platformEventsVisibleTo(user, strings.TrimSpace(ctx.Query("scope")))
	query = applySearch(ctx, query, "platform_events.type", "platform_events.message", "platform_events.resource_id")
	query = applyPlatformEventFilters(ctx, query)

	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	var events []model.PlatformEvent
	if err := query.Session(&gorm.Session{}).
		Order(orderByClause(pagination, map[string]string{
			"occurredAt": "occurred_at",
			"createdAt":  "created_at",
			"severity":   "severity",
			"type":       "type",
			"category":   "category",
		}, "occurred_at desc")).
		Limit(pagination.PageSize).
		Offset(pagination.Offset()).
		Find(&events).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	responses := make([]platformEventResponse, 0, len(events))
	for _, event := range events {
		responses = append(responses, platformEventResponseFor(event, 0))
	}
	ctx.JSON(http.StatusOK, paginatedResponse(responses, total, pagination))
}

func (h *Handlers) GetPlatformEvent(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var event model.PlatformEvent
	if err := h.db.First(&event, "id = ?", ctx.Param("eventId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "event not found")
		return
	}
	if !h.canReadPlatformEvent(user, event) {
		writeError(ctx, http.StatusForbidden, "you cannot access this event")
		return
	}
	var deliveryCount int64
	if authz.IsPlatformAdmin(user.Role) {
		if err := h.db.Model(&model.NotificationDelivery{}).Where("event_id = ?", event.ID).Count(&deliveryCount).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		}
	}
	ctx.JSON(http.StatusOK, platformEventResponseFor(event, deliveryCount))
}

func (h *Handlers) ListPlatformEventCatalog(ctx *gin.Context) {
	if _, ok := h.currentUser(ctx); !ok {
		return
	}
	ctx.JSON(http.StatusOK, platformevent.Catalog())
}

func (h *Handlers) platformEventsVisibleTo(user model.User, scope string) *gorm.DB {
	query := h.db.Model(&model.PlatformEvent{})
	if authz.IsPlatformAdmin(user.Role) && scope == "all" {
		return query
	}
	projectIDs := h.projectIDsForUser(user.ID)
	if len(projectIDs) == 0 {
		return query.Where("actor_id = ?", user.ID)
	}
	return query.Where("project_id in ? or actor_id = ?", projectIDs, user.ID)
}

func (h *Handlers) canReadPlatformEvent(user model.User, event model.PlatformEvent) bool {
	return canReadPlatformEventForUser(user, event, h.projectIDsForUser(user.ID))
}

func canReadPlatformEventForUser(user model.User, event model.PlatformEvent, projectIDs []string) bool {
	if authz.IsPlatformAdmin(user.Role) {
		return true
	}
	if event.ActorID != "" && event.ActorID == user.ID {
		return true
	}
	for _, projectID := range projectIDs {
		if event.ProjectID != "" && event.ProjectID == projectID {
			return true
		}
	}
	return false
}

func applyPlatformEventFilters(ctx *gin.Context, query *gorm.DB) *gorm.DB {
	filters := []struct {
		singular string
		plural   string
		column   string
	}{
		{singular: "projectId", plural: "projectIds", column: "project_id"},
		{singular: "applicationId", plural: "applicationIds", column: "application_id"},
		{singular: "deploymentTargetId", plural: "deploymentTargetIds", column: "deployment_target_id"},
		{singular: "category", plural: "categories", column: "category"},
		{singular: "type", plural: "types", column: "type"},
		{singular: "severity", plural: "severities", column: "severity"},
		{singular: "status", plural: "statuses", column: "status"},
	}
	for _, filter := range filters {
		values := platformEventFilterValues(ctx, filter.singular, filter.plural)
		if len(values) == 1 {
			query = query.Where(filter.column+" = ?", values[0])
		} else if len(values) > 1 {
			query = query.Where(filter.column+" in ?", values)
		}
	}
	if value, ok := parsePlatformEventTime(ctx.Query("dateFrom"), false); ok {
		query = query.Where("occurred_at >= ?", value)
	}
	if value, ok := parsePlatformEventTime(ctx.Query("dateTo"), true); ok {
		query = query.Where("occurred_at <= ?", value)
	}
	return query
}

func platformEventFilterValues(ctx *gin.Context, singular, plural string) []string {
	rawValues := append([]string{}, ctx.QueryArray(plural)...)
	rawValues = append(rawValues, ctx.QueryArray(singular)...)
	values := make([]string, 0, len(rawValues))
	seen := make(map[string]struct{}, len(rawValues))
	for _, rawValue := range rawValues {
		for value := range strings.SplitSeq(rawValue, ",") {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			if _, exists := seen[value]; exists {
				continue
			}
			seen[value] = struct{}{}
			values = append(values, value)
		}
	}
	return values
}

func parsePlatformEventTime(raw string, endOfDay bool) (time.Time, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Time{}, false
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed, true
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, false
	}
	if endOfDay {
		parsed = parsed.Add(24*time.Hour - time.Nanosecond)
	}
	return parsed, true
}

func platformEventResponseFor(event model.PlatformEvent, deliveryCount int64) platformEventResponse {
	var detail any = map[string]any{}
	_ = json.Unmarshal([]byte(event.DetailJSON), &detail)
	links := map[string]string{}
	_ = json.Unmarshal([]byte(event.LinksJSON), &links)
	return platformEventResponse{
		PlatformEvent: event,
		Detail:        detail,
		Links:         links,
		DeliveryCount: deliveryCount,
	}
}
