package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/billing"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/observation"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const gatewayTrafficReportFreshness = 10 * time.Minute

type gatewayTrafficStatusResponse struct {
	Available             bool       `json:"available"`
	Installed             bool       `json:"installed"`
	Status                string     `json:"status"`
	ComponentID           string     `json:"componentId"`
	InstallableTemplateID string     `json:"installableTemplateId"`
	ObservationCode       string     `json:"observationCode"`
	ObservedAt            *time.Time `json:"observedAt"`
	LastReportedAt        *time.Time `json:"lastReportedAt"`
	LastWindowStart       *time.Time `json:"lastWindowStart"`
	LastWindowEnd         *time.Time `json:"lastWindowEnd"`
	LastError             string     `json:"lastError"`
}

func (h *Handlers) GetGatewayTrafficStatus(ctx *gin.Context) {
	markLiveObservationResponse(ctx)
	if _, ok := h.currentUser(ctx); !ok {
		return
	}

	var installations []model.SystemComponentInstallation
	if err := h.dbFor(ctx).Where("component_id = ?", systemComponentGatewayTrafficProbe).Order("created_at desc").Find(&installations).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if len(installations) == 0 {
		observedAt := time.Now().UTC()
		ctx.JSON(http.StatusOK, gatewayTrafficStatusResponse{
			Available:             false,
			Installed:             false,
			Status:                observation.StatusNotConfigured,
			ComponentID:           systemComponentGatewayTrafficProbe,
			InstallableTemplateID: "luna-gateway-traffic-probe",
			ObservationCode:       "billing.gateway_traffic_probe_not_configured",
			ObservedAt:            &observedAt,
		})
		return
	}

	h.observeSystemComponentInstallations(ctx.Request.Context(), installations)
	runtimeStatus, observationCode, observedAt := summarizeGatewayTrafficProbeObservations(installations)

	var latestUsage model.BillingUsageRecord
	usageResult := h.dbFor(ctx).
		Where("resource_type = ? and meter = ? and status = ?", billing.ResourceTypeGateway, "gateway.egress_gib", "settled").
		Order("period_end desc").
		First(&latestUsage)
	if usageResult.Error != nil && !errors.Is(usageResult.Error, gorm.ErrRecordNotFound) {
		writeError(ctx, http.StatusInternalServerError, usageResult.Error.Error())
		return
	}
	hasReport := usageResult.Error == nil
	var lastReportedAt *time.Time
	var lastWindowStart *time.Time
	var lastWindowEnd *time.Time
	reportFresh := false
	if hasReport {
		reportedAt := latestUsage.CreatedAt.UTC()
		if latestUsage.SettledAt != nil {
			reportedAt = latestUsage.SettledAt.UTC()
		}
		windowStart := latestUsage.PeriodStart.UTC()
		windowEnd := latestUsage.PeriodEnd.UTC()
		lastReportedAt = &reportedAt
		lastWindowStart = &windowStart
		lastWindowEnd = &windowEnd
		reportFresh = windowEnd.After(time.Now().UTC().Add(-gatewayTrafficReportFreshness))
	}
	if runtimeStatus == observation.StatusReady {
		switch {
		case !hasReport:
			observationCode = "billing.gateway_traffic_waiting_report"
		case !reportFresh:
			observationCode = "billing.gateway_traffic_report_stale"
		}
	}

	ctx.JSON(http.StatusOK, gatewayTrafficStatusResponse{
		Available:             runtimeStatus == observation.StatusReady && reportFresh,
		Installed:             true,
		Status:                runtimeStatus,
		ComponentID:           systemComponentGatewayTrafficProbe,
		InstallableTemplateID: "luna-gateway-traffic-probe",
		ObservationCode:       observationCode,
		ObservedAt:            observedAt,
		LastReportedAt:        lastReportedAt,
		LastWindowStart:       lastWindowStart,
		LastWindowEnd:         lastWindowEnd,
	})
}

func summarizeGatewayTrafficProbeObservations(items []model.SystemComponentInstallation) (string, string, *time.Time) {
	bestStatus := observation.StatusUnknown
	bestCode := "billing.gateway_traffic_probe_unknown"
	var bestObservedAt *time.Time
	bestRank := -1
	for index := range items {
		rank := gatewayTrafficObservationRank(items[index].RuntimeStatus)
		if rank <= bestRank {
			continue
		}
		bestRank = rank
		bestStatus = items[index].RuntimeStatus
		bestCode = items[index].ObservationCode
		bestObservedAt = items[index].ObservedAt
	}
	return bestStatus, bestCode, bestObservedAt
}

func gatewayTrafficObservationRank(status string) int {
	switch status {
	case observation.StatusReady:
		return 6
	case observation.StatusProgressing:
		return 5
	case observation.StatusDegraded:
		return 4
	case observation.StatusUnavailable:
		return 3
	case observation.StatusNotFound:
		return 2
	case observation.StatusNotConfigured:
		return 1
	default:
		return 0
	}
}

func (h *Handlers) CreateGatewayTrafficUsage(ctx *gin.Context) {
	actorID := ""
	var component model.SystemComponentInstallation
	componentAuthenticated := false
	if token := bearerTokenFromHeader(ctx.GetHeader("Authorization")); token != "" {
		if item, ok := h.systemComponentForBearerToken(token, systemComponentGatewayTrafficProbe, ctx.Request.Context()); ok {
			component = item
			componentAuthenticated = true
			actorID = item.ID
		}
	}
	if !componentAuthenticated {
		user, ok := h.currentUser(ctx)
		if !ok {
			return
		}
		if user.Role != authz.PlatformRoleAdmin {
			writeErrorKey(ctx, http.StatusForbidden, user.Language, "config.admin.required")
			return
		}
		actorID = user.ID
	}
	var input gatewayTrafficUsageInput
	if !bindJSON(ctx, &input) {
		return
	}
	routeID := strings.TrimSpace(input.RouteID)
	if routeID == "" {
		writeErrorCode(ctx, http.StatusBadRequest, "billing.gateway_route_required", "gateway route is required")
		return
	}
	if input.ResponseBytes <= 0 {
		writeErrorCode(ctx, http.StatusBadRequest, "billing.gateway_response_bytes_invalid", "gateway response bytes must be positive")
		return
	}
	periodStart, err := time.Parse(time.RFC3339, strings.TrimSpace(input.PeriodStart))
	if err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "billing.period_start_invalid", "periodStart must be RFC3339 time")
		return
	}
	periodEnd, err := time.Parse(time.RFC3339, strings.TrimSpace(input.PeriodEnd))
	if err != nil || !periodEnd.After(periodStart) {
		writeErrorCode(ctx, http.StatusBadRequest, "billing.period_end_invalid", "periodEnd must be RFC3339 time after periodStart")
		return
	}
	var route model.GatewayRoute
	if err := h.dbFor(ctx).First(&route, "id = ? and delete_status = ?", routeID, "active").Error; err != nil {
		writeError(ctx, http.StatusNotFound, "gateway route not found")
		return
	}
	if componentAuthenticated && !h.gatewayRouteBelongsToRuntimeCluster(route, component.RuntimeClusterID, ctx.Request.Context()) {
		writeErrorCode(ctx, http.StatusForbidden, "billing.gateway_route_cluster_forbidden", "gateway route does not belong to the probe runtime cluster")
		return
	}
	err = (billing.Service{DB: h.dbFor(ctx)}).SettleGatewayTrafficWindow(billing.GatewayTrafficUsageInput{
		Route:         route,
		ResponseBytes: input.ResponseBytes,
		RequestCount:  input.RequestCount,
		PeriodStart:   periodStart,
		PeriodEnd:     periodEnd,
		ActorID:       actorID,
	})
	if errors.Is(err, billing.ErrAlreadySettled) {
		ctx.JSON(http.StatusOK, gin.H{"status": "already_settled"})
		return
	}
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.auditWithContext(actorID, "billing.gateway_traffic", route.ID, true, "", ctx.Request.Context())
	ctx.JSON(http.StatusCreated, gin.H{"status": "settled"})
}

func (h *Handlers) CreateGatewayTrafficProbeHello(ctx *gin.Context) {
	token := bearerTokenFromHeader(ctx.GetHeader("Authorization"))
	_, ok := h.systemComponentForBearerToken(token, systemComponentGatewayTrafficProbe, ctx.Request.Context())
	if !ok {
		writeError(ctx, http.StatusUnauthorized, "gateway traffic probe token is invalid")
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func bearerTokenFromHeader(header string) string {
	header = strings.TrimSpace(header)
	if len(header) < len("Bearer ") || !strings.EqualFold(header[:len("Bearer ")], "Bearer ") {
		return ""
	}
	return strings.TrimSpace(header[len("Bearer "):])
}

func (h *Handlers) gatewayRouteBelongsToRuntimeCluster(route model.GatewayRoute, clusterID string, ctx context.Context) bool {
	clusterID = strings.TrimSpace(clusterID)
	if clusterID == "" {
		return false
	}
	var target model.DeploymentTarget
	if err := h.dbWithContext(ctx).Select("id", "cluster_id").First(&target, "id = ? and project_id = ?", route.DeploymentTargetID, route.ProjectID).Error; err != nil {
		return false
	}
	targetClusterID := strings.TrimSpace(target.ClusterID)
	if targetClusterID == "" {
		targetClusterID = h.defaultRuntimeClusterID(ctx)
	}
	return targetClusterID == clusterID
}
