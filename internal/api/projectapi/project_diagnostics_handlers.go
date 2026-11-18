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

const (
	projectDiagnosticsWindowDays      = 7
	projectDiagnosticsMaxIssueItems   = 5
	projectDiagnosticsMaxUnhealthyRts = 10
)

type projectDiagnosticsResponse struct {
	WindowDays   int                            `json:"windowDays"`
	Applications int64                          `json:"applications"`
	Builds       projectDiagnosticsBuilds       `json:"builds"`
	Releases     projectDiagnosticsReleases     `json:"releases"`
	AccessRoutes projectDiagnosticsAccessRoutes `json:"accessRoutes"`
}

type projectDiagnosticsBuilds struct {
	ActiveCount  int64                        `json:"activeCount"`
	FailedCount  int64                        `json:"failedCount"`
	RecentIssues []projectDiagnosticBuildItem `json:"recentIssues"`
}

type projectDiagnosticBuildItem struct {
	ID                 string     `json:"id"`
	ApplicationID      string     `json:"applicationId"`
	DeploymentTargetID string     `json:"deploymentTargetId"`
	Status             string     `json:"status"`
	FailureCode        string     `json:"failureCode,omitempty"`
	TriggerType        string     `json:"triggerType"`
	SourceBranch       string     `json:"sourceBranch,omitempty"`
	SourceCommit       string     `json:"sourceCommit,omitempty"`
	CreatedAt          time.Time  `json:"createdAt"`
	FinishedAt         *time.Time `json:"finishedAt,omitempty"`
}

type projectDiagnosticsReleases struct {
	ActiveCount  int64                          `json:"activeCount"`
	FailedCount  int64                          `json:"failedCount"`
	RecentIssues []projectDiagnosticReleaseItem `json:"recentIssues"`
}

type projectDiagnosticReleaseItem struct {
	ID                 string     `json:"id"`
	ApplicationID      string     `json:"applicationId"`
	DeploymentTargetID string     `json:"deploymentTargetId"`
	ImageRef           string     `json:"imageRef"`
	Type               string     `json:"type"`
	Status             string     `json:"status"`
	FailureCode        string     `json:"failureCode,omitempty"`
	Message            string     `json:"message,omitempty"`
	CreatedAt          time.Time  `json:"createdAt"`
	FinishedAt         *time.Time `json:"finishedAt,omitempty"`
}

type projectDiagnosticsAccessRoutes struct {
	Summary        projectOverviewAccessRoutes  `json:"summary"`
	UnhealthyItems []projectDiagnosticRouteItem `json:"unhealthyItems"`
}

type projectDiagnosticRouteItem struct {
	ID                 string `json:"id"`
	ApplicationID      string `json:"applicationId"`
	DeploymentTargetID string `json:"deploymentTargetId"`
	Host               string `json:"host"`
	Path               string `json:"path"`
	TLSMode            string `json:"tlsMode"`
	Status             string `json:"status"`
	ObservationCode    string `json:"observationCode,omitempty"`
	CertificateStatus  string `json:"certificateStatus,omitempty"`
	AccessURL          string `json:"accessUrl,omitempty"`
}

// GetProjectDiagnostics returns a point-in-time read-only delivery and gateway
// diagnostic summary for one project space so the AI assistant or operator can
// locate the first failing boundary without issuing multiple list queries.
func (h *Handler) GetProjectDiagnostics(ctx *gin.Context) {
	user, project, ok := h.authorizeProject(ctx, authz.ActionProjectRead)
	if !ok {
		return
	}
	reqCtx := ctx.Request.Context()
	subject := authz.ProjectSubject{UserID: user.ID, PlatformRole: user.Role}
	authorizer := h.projectAuthorizer(reqCtx)
	for _, action := range []authz.Action{authz.ActionBuildRead, authz.ActionDeploymentRead, authz.ActionGatewayRead} {
		if _, err := authorizer.AuthorizeProject(reqCtx, subject, project.ID, action); err != nil {
			writeProjectAuthorizationError(ctx, err)
			return
		}
	}

	transportapi.MarkLiveObservationResponse(ctx)
	now := time.Now().UTC()
	response, err := loadProjectDiagnosticsSnapshot(h.dbFor(ctx), project.ID, now)
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
	observedRoutes := h.host.ObserveGatewayRouteDiagnostics(reqCtx, routes)
	observedAt := time.Now().UTC()
	response.AccessRoutes = summarizeProjectDiagnosticsAccessRoutes(observedRoutes, observedAt)
	ctx.JSON(http.StatusOK, response)
}

func loadProjectDiagnosticsSnapshot(db *gorm.DB, projectID string, now time.Time) (projectDiagnosticsResponse, error) {
	overview, err := projectOverviewCounts(db, projectID, now)
	if err != nil {
		return projectDiagnosticsResponse{}, err
	}
	windowStart := now.AddDate(0, 0, -projectDiagnosticsWindowDays)
	response := projectDiagnosticsResponse{
		WindowDays:   projectDiagnosticsWindowDays,
		Applications: overview.Applications,
		Builds: projectDiagnosticsBuilds{
			RecentIssues: []projectDiagnosticBuildItem{},
		},
		Releases: projectDiagnosticsReleases{
			ActiveCount:  overview.ActiveReleases,
			FailedCount:  overview.FailedReleases,
			RecentIssues: []projectDiagnosticReleaseItem{},
		},
	}

	if err := db.Model(&model.BuildRun{}).
		Where("project_id = ? and status in ?", projectID, []string{"queued", "running"}).
		Count(&response.Builds.ActiveCount).Error; err != nil {
		return projectDiagnosticsResponse{}, err
	}
	if err := db.Model(&model.BuildRun{}).
		Where("project_id = ? and status = ? and coalesce(finished_at, created_at) >= ?", projectID, "failed", windowStart).
		Count(&response.Builds.FailedCount).Error; err != nil {
		return projectDiagnosticsResponse{}, err
	}

	var buildRuns []model.BuildRun
	if err := db.
		Where(
			"project_id = ? and (status in ? or (status = ? and coalesce(finished_at, created_at) >= ?))",
			projectID,
			[]string{"queued", "running"},
			"failed",
			windowStart,
		).
		Order("created_at desc, id desc").
		Limit(projectDiagnosticsMaxIssueItems).
		Find(&buildRuns).Error; err != nil {
		return projectDiagnosticsResponse{}, err
	}
	for _, item := range buildRuns {
		response.Builds.RecentIssues = append(response.Builds.RecentIssues, projectDiagnosticBuildItem{
			ID:                 item.ID,
			ApplicationID:      item.ApplicationID,
			DeploymentTargetID: item.DeploymentTargetID,
			Status:             item.Status,
			FailureCode:        item.FailureCode,
			TriggerType:        item.TriggerType,
			SourceBranch:       item.SourceBranch,
			SourceCommit:       item.SourceCommit,
			CreatedAt:          item.CreatedAt,
			FinishedAt:         item.FinishedAt,
		})
	}

	var releases []model.Release
	if err := db.
		Where(
			"project_id = ? and (status in ? or (status = ? and coalesce(finished_at, created_at) >= ?))",
			projectID,
			[]string{"pending", "running"},
			"failed",
			windowStart,
		).
		Order("created_at desc, id desc").
		Limit(projectDiagnosticsMaxIssueItems).
		Find(&releases).Error; err != nil {
		return projectDiagnosticsResponse{}, err
	}
	for _, item := range releases {
		response.Releases.RecentIssues = append(response.Releases.RecentIssues, projectDiagnosticReleaseItem{
			ID:                 item.ID,
			ApplicationID:      item.ApplicationID,
			DeploymentTargetID: item.DeploymentTargetID,
			ImageRef:           item.ImageRef,
			Type:               item.Type,
			Status:             item.Status,
			FailureCode:        item.FailureCode,
			Message:            item.Message,
			CreatedAt:          item.CreatedAt,
			FinishedAt:         item.FinishedAt,
		})
	}

	return response, nil
}

func summarizeProjectDiagnosticsAccessRoutes(routes []model.GatewayRoute, observedAt time.Time) projectDiagnosticsAccessRoutes {
	summary := projectOverviewAccessRoutes{Total: int64(len(routes)), ObservedAt: observedAt}
	unhealthy := []projectDiagnosticRouteItem{}
	if len(routes) == 0 {
		summary.Status = observation.StatusNotConfigured
		summary.ObservationCode = "project_overview.access_routes_not_configured"
		return projectDiagnosticsAccessRoutes{Summary: summary, UnhealthyItems: unhealthy}
	}

	var ready int64
	anyUnavailable := false
	for _, route := range routes {
		certUnavailable := route.TLSMode == "http-challenge" && route.CertificateStatus == observation.StatusUnavailable
		if route.Status == observation.StatusUnavailable || certUnavailable {
			anyUnavailable = true
		}
		certHealthy := route.TLSMode != "http-challenge" || route.CertificateStatus == "issued"
		if route.Status == observation.StatusReady && certHealthy {
			ready++
			continue
		}
		if len(unhealthy) < projectDiagnosticsMaxUnhealthyRts {
			unhealthy = append(unhealthy, projectDiagnosticRouteItem{
				ID:                 route.ID,
				ApplicationID:      route.ApplicationID,
				DeploymentTargetID: route.DeploymentTargetID,
				Host:               route.Host,
				Path:               route.Path,
				TLSMode:            route.TLSMode,
				Status:             route.Status,
				ObservationCode:    route.ObservationCode,
				CertificateStatus:  route.CertificateStatus,
				AccessURL:          route.AccessURL,
			})
		}
	}

	if anyUnavailable {
		summary.Status = observation.StatusUnavailable
		summary.ObservationCode = "project_overview.access_routes_unavailable"
	} else {
		summary.Ready = &ready
		if ready == summary.Total {
			summary.Status = observation.StatusReady
		} else {
			summary.Status = observation.StatusDegraded
			summary.ObservationCode = "project_overview.access_routes_not_ready"
		}
	}
	return projectDiagnosticsAccessRoutes{
		Summary:        summary,
		UnhealthyItems: unhealthy,
	}
}
