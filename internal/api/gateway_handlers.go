package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *Handlers) ListGatewayRoutes(ctx *gin.Context) {
	markLiveObservationResponse(ctx)
	if _, _, ok := h.authorizeProject(ctx, authz.ActionGatewayRead); !ok {
		return
	}
	query := h.dbFor(ctx).Model(&model.GatewayRoute{}).Where("project_id = ?", ctx.Param("projectId"))
	if applicationID := strings.TrimSpace(ctx.Query("applicationId")); applicationID != "" {
		query = query.Where("application_id = ?", applicationID)
	}
	query = applySearch(ctx, query, "host", "path")
	var routes []model.GatewayRoute
	pagination := paginationFromQueryWithSort(ctx, map[string]string{"host": "host", "enabled": "enabled", "createdAt": "created_at"}, "createdAt")
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if err := query.Order(orderByClause(pagination, map[string]string{
		"host":      "host",
		"enabled":   "enabled",
		"createdAt": "created_at",
	}, "created_at")).Limit(pagination.PageSize).Offset(pagination.Offset()).Find(&routes).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	routes = h.observeGatewayRoutes(ctx.Request.Context(), routes)
	ctx.JSON(http.StatusOK, paginatedResponse(h.gatewayRoutesWithAccessURL(routes, ctx.Request.Context()), total, pagination))
}

func (h *Handlers) GetGatewayRoute(ctx *gin.Context) {
	if _, _, ok := h.authorizeProject(ctx, authz.ActionGatewayRead); !ok {
		return
	}
	route, ok := h.findGatewayRoute(ctx)
	if !ok {
		return
	}
	markLiveObservationResponse(ctx)
	routes := h.observeGatewayRoutes(ctx.Request.Context(), []model.GatewayRoute{route})
	if len(routes) == 1 {
		route = routes[0]
	}
	ctx.JSON(http.StatusOK, h.gatewayRouteWithAccessURL(route, ctx.Request.Context()))
}

func (h *Handlers) CreateGatewayRoute(ctx *gin.Context) {
	user, project, ok := h.authorizeProject(ctx, authz.ActionGatewayManage)
	if !ok {
		return
	}
	if !h.ensureProjectCanMutate(ctx, project) {
		return
	}
	var input gatewayRouteInput
	if !bindJSON(ctx, &input) {
		return
	}
	route, ok := h.gatewayRouteFromInput(ctx, project, user, user.ID, input, "")
	if !ok {
		return
	}
	route.ID = id.New("gwr")
	if !h.ensureGatewayRouteBackendAvailable(ctx, route) {
		return
	}
	if err := h.dbFor(ctx).Create(&route).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if !h.enqueueGatewayApply(ctx.Request.Context(), route, user.ID) {
		writeError(ctx, http.StatusServiceUnavailable, "网关任务投递失败，请稍后重试")
		return
	}
	route.Status = "progressing"
	route.ObservationCode = "gateway_route.apply_queued"
	ctx.JSON(http.StatusCreated, h.gatewayRouteWithAccessURL(route, ctx.Request.Context()))
}

func (h *Handlers) UpdateGatewayRoute(ctx *gin.Context) {
	user, project, ok := h.authorizeProject(ctx, authz.ActionGatewayManage)
	if !ok {
		return
	}
	if !h.ensureProjectCanMutate(ctx, project) {
		return
	}
	route, ok := h.findGatewayRoute(ctx)
	if !ok {
		return
	}
	if !h.ensureGatewayRouteCanMutate(ctx, route) {
		return
	}
	var input gatewayRouteInput
	if !bindJSON(ctx, &input) {
		return
	}
	next, ok := h.gatewayRouteFromInput(ctx, project, user, route.CreatedBy, input, route.ID)
	if !ok {
		return
	}
	if !h.ensureGatewayRouteBackendAvailable(ctx, next) {
		return
	}
	route.ApplicationID = next.ApplicationID
	route.EnvironmentID = next.EnvironmentID
	route.DeploymentTargetID = next.DeploymentTargetID
	route.Host = next.Host
	route.DomainSuffix = next.DomainSuffix
	route.Path = next.Path
	route.ServicePort = next.ServicePort
	route.TLSMode = next.TLSMode
	route.CNAMEName = next.CNAMEName
	route.CNAMETarget = next.CNAMETarget
	route.Enabled = next.Enabled
	route.IsDefault = next.IsDefault
	route.ParentGatewayName = next.ParentGatewayName
	route.ParentGatewayNamespace = next.ParentGatewayNamespace
	route.SectionName = next.SectionName
	route.PathMatchType = next.PathMatchType
	route.RequestHeaders = next.RequestHeaders
	route.ResponseHeaders = next.ResponseHeaders
	route.URLRewrite = next.URLRewrite
	route.RequestRedirect = next.RequestRedirect
	route.BackendWeight = next.BackendWeight
	route.HostnameAliases = next.HostnameAliases
	if err := h.dbFor(ctx).Save(&route).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if !h.enqueueGatewayApply(ctx.Request.Context(), route, user.ID) {
		writeError(ctx, http.StatusServiceUnavailable, "网关任务投递失败，请稍后重试")
		return
	}
	route.Status = "progressing"
	route.ObservationCode = "gateway_route.apply_queued"
	ctx.JSON(http.StatusOK, h.gatewayRouteWithAccessURL(route, ctx.Request.Context()))
}

func (h *Handlers) DeleteGatewayRoute(ctx *gin.Context) {
	user, project, ok := h.authorizeProject(ctx, authz.ActionGatewayDelete)
	if !ok {
		return
	}
	if !h.ensureProjectCanMutate(ctx, project) {
		return
	}
	route, ok := h.findGatewayRoute(ctx)
	if !ok {
		return
	}
	if !deleteStatusCanStart(route.DeleteStatus) {
		writeError(ctx, http.StatusConflict, "访问入口正在删除中，请等待资源清理完成")
		return
	}
	if err := markResourceDeleting(h.dbFor(ctx), &model.GatewayRoute{}, route.ID); err != nil {
		if errors.Is(err, errResourceDeleteAlreadyStarted) {
			writeErrorCode(ctx, http.StatusConflict, "gateway_route.delete_in_progress", "访问入口正在删除中，请等待资源清理完成")
			return
		}
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if !h.enqueueResourceCleanup(ctx.Request.Context(), "gateway_route", route.ID, route.ProjectID, user.ID) {
		_ = markResourceDeleteFailed(h.dbFor(ctx), &model.GatewayRoute{}, route.ID, "资源清理任务投递失败，请稍后重试")
		h.auditWithContext(user.ID, "gateway.delete", route.ID, false, "cleanup_enqueue_failed", ctx.Request.Context())
		writeError(ctx, http.StatusServiceUnavailable, "资源清理任务投递失败，请稍后重试")
		return
	}
	h.auditWithContext(user.ID, "gateway.delete", route.ID, true, "cleanup_queued", ctx.Request.Context())
	ctx.Status(http.StatusNoContent)
}

func (h *Handlers) findGatewayRoute(ctx *gin.Context) (model.GatewayRoute, bool) {
	var route model.GatewayRoute
	if err := h.dbFor(ctx).First(&route, "id = ? and project_id = ?", ctx.Param("routeId"), ctx.Param("projectId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "gateway route not found")
		return route, false
	}
	return route, true
}

func (h *Handlers) enqueueGatewayApply(ctx context.Context, route model.GatewayRoute, actorID string) bool {
	if h.taskClient == nil {
		return false
	}
	_, err := h.taskClient.EnqueueGatewayApply(ctx, tasks.GatewayApplyPayload{
		GatewayRouteID:          route.ID,
		ProjectID:               route.ProjectID,
		ActorID:                 strings.TrimSpace(actorID),
		RouteUpdatedAtUnixMicro: route.UpdatedAt.UTC().UnixMicro(),
	})
	return err == nil
}
