package projectapi

import (
	"net/http"
	"time"

	transportapi "github.com/LiteyukiStudio/devops/internal/api/transport"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/observation"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// projectOverviewFailedReleaseWindowDays bounds the failed release count so the
// overview reports recent delivery problems instead of all history.
const projectOverviewFailedReleaseWindowDays = 7

var projectOverviewExcludedApplicationDeleteStatuses = []string{"deleting", "deleted"}

type projectOverviewResponse struct {
	Applications            int64                       `json:"applications"`
	ActiveReleases          int64                       `json:"activeReleases"`
	FailedReleases          int64                       `json:"failedReleases"`
	FailedReleaseWindowDays int                         `json:"failedReleaseWindowDays"`
	AccessRoutes            projectOverviewAccessRoutes `json:"accessRoutes"`
}

// projectOverviewAccessRoutes 是项目空间启用访问入口的实时就绪汇总。Ready 只在全部入口都被观察到时返回；
// 任一入口无法观察时整体为 unavailable，不以部分结果冒充当前事实。
type projectOverviewAccessRoutes struct {
	Status          string    `json:"status"`
	Ready           *int64    `json:"ready,omitempty"`
	Total           int64     `json:"total"`
	ObservationCode string    `json:"observationCode,omitempty"`
	ObservedAt      time.Time `json:"observedAt"`
}

// GetProjectOverview returns project-level counts computed by the database so
// the overview does not count a truncated page of resources in the browser.
// Access route readiness is observed from the gateway on every request.
func (h *Handler) GetProjectOverview(ctx *gin.Context) {
	_, project, ok := h.authorizeProject(ctx, authz.ActionProjectRead)
	if !ok {
		return
	}
	transportapi.MarkLiveObservationResponse(ctx)
	response, err := projectOverviewCounts(h.dbFor(ctx), project.ID, time.Now().UTC())
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	var routes []model.GatewayRoute
	if err := h.dbFor(ctx).
		Where("project_id = ? and enabled = ? and delete_status not in ?", project.ID, true, projectOverviewExcludedRouteDeleteStatuses).
		Order("created_at asc, id asc").
		Find(&routes).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.AccessRoutes = summarizeProjectOverviewAccessRoutes(h.host.ObserveGatewayRouteStatuses(ctx.Request.Context(), routes), time.Now().UTC())
	ctx.JSON(http.StatusOK, response)
}

var projectOverviewExcludedRouteDeleteStatuses = []string{"deleting", "deleted"}

func summarizeProjectOverviewAccessRoutes(routes []model.GatewayRoute, observedAt time.Time) projectOverviewAccessRoutes {
	summary := projectOverviewAccessRoutes{Total: int64(len(routes)), ObservedAt: observedAt}
	if len(routes) == 0 {
		summary.Status = observation.StatusNotConfigured
		summary.ObservationCode = "project_overview.access_routes_not_configured"
		return summary
	}
	var ready int64
	for _, route := range routes {
		switch route.Status {
		case observation.StatusUnavailable:
			summary.Status = observation.StatusUnavailable
			summary.ObservationCode = "project_overview.access_routes_unavailable"
			return summary
		case observation.StatusReady:
			ready++
		}
	}
	summary.Ready = &ready
	if ready == summary.Total {
		summary.Status = observation.StatusReady
		return summary
	}
	summary.Status = observation.StatusDegraded
	summary.ObservationCode = "project_overview.access_routes_not_ready"
	return summary
}

func projectOverviewCounts(db *gorm.DB, projectID string, now time.Time) (projectOverviewResponse, error) {
	response := projectOverviewResponse{FailedReleaseWindowDays: projectOverviewFailedReleaseWindowDays}
	failedSince := now.AddDate(0, 0, -projectOverviewFailedReleaseWindowDays)
	if err := db.Model(&model.Application{}).
		Where("project_id = ? and delete_status not in ?", projectID, projectOverviewExcludedApplicationDeleteStatuses).
		Count(&response.Applications).Error; err != nil {
		return response, err
	}
	if err := db.Model(&model.Release{}).
		Where("project_id = ? and status in ?", projectID, []string{"pending", "running"}).
		Count(&response.ActiveReleases).Error; err != nil {
		return response, err
	}
	if err := db.Model(&model.Release{}).
		Where("project_id = ? and status = ? and coalesce(finished_at, created_at) >= ?", projectID, "failed", failedSince).
		Count(&response.FailedReleases).Error; err != nil {
		return response, err
	}
	return response, nil
}
