package deploymentapi

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/observation"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/gin-gonic/gin"
)

const (
	deploymentTargetDiagnosticsMaxPods     = 10
	deploymentTargetDiagnosticsMaxEvents   = 10
	deploymentTargetDiagnosticsMaxReleases = 5
)

var deploymentDiagnosticsExcludedRouteDeleteStatuses = []string{"deleting", "deleted"}

type deploymentTargetDiagnosticsResponse struct {
	Target         deploymentTargetDiagnosticTarget        `json:"target"`
	Pods           deploymentTargetDiagnosticPods          `json:"pods"`
	Events         deploymentTargetDiagnosticEvents        `json:"events"`
	RecentReleases []deploymentTargetDiagnosticReleaseItem `json:"recentReleases"`
	AccessRoutes   []deploymentTargetDiagnosticRouteItem   `json:"accessRoutes"`
}

type deploymentTargetDiagnosticTarget struct {
	ID                 string     `json:"id"`
	ProjectID          string     `json:"projectId"`
	ApplicationID      string     `json:"applicationId"`
	Name               string     `json:"name"`
	Stage              string     `json:"stage"`
	WorkloadType       string     `json:"workloadType"`
	Enabled            bool       `json:"enabled"`
	EffectiveClusterID string     `json:"effectiveClusterId,omitempty"`
	ImageRef           string     `json:"imageRef,omitempty"`
	ConfiguredReplicas int        `json:"configuredReplicas"`
	DesiredReplicas    int32      `json:"desiredReplicas"`
	ReadyReplicas      int32      `json:"readyReplicas"`
	AvailableReplicas  int32      `json:"availableReplicas"`
	UpdatedReplicas    int32      `json:"updatedReplicas"`
	Status             string     `json:"status"`
	ObservationCode    string     `json:"observationCode,omitempty"`
	ObservedAt         *time.Time `json:"observedAt,omitempty"`
}

type deploymentTargetDiagnosticPods struct {
	Status         string                            `json:"status"`
	Code           string                            `json:"observationCode"`
	ObservedAt     time.Time                         `json:"observedAt"`
	Total          int                               `json:"total"`
	UnhealthyCount int                               `json:"unhealthyCount"`
	Items          []kubeprovider.RuntimePodSnapshot `json:"items"`
}

type deploymentTargetDiagnosticEvents struct {
	Status       string                                       `json:"status"`
	Code         string                                       `json:"observationCode"`
	ObservedAt   time.Time                                    `json:"observedAt"`
	Total        int                                          `json:"total"`
	WarningCount int                                          `json:"warningCount"`
	Items        []kubeprovider.DeploymentTargetEventSnapshot `json:"items"`
}

type deploymentTargetDiagnosticReleaseItem struct {
	ID          string     `json:"id"`
	Revision    int        `json:"revision"`
	Type        string     `json:"type"`
	ImageRef    string     `json:"imageRef"`
	Status      string     `json:"status"`
	FailureCode string     `json:"failureCode,omitempty"`
	Message     string     `json:"message,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	FinishedAt  *time.Time `json:"finishedAt,omitempty"`
}

type deploymentTargetDiagnosticRouteItem struct {
	ID                string `json:"id"`
	Host              string `json:"host"`
	Path              string `json:"path"`
	ServicePort       int    `json:"servicePort"`
	TLSMode           string `json:"tlsMode"`
	Enabled           bool   `json:"enabled"`
	Status            string `json:"status"`
	ObservationCode   string `json:"observationCode,omitempty"`
	CertificateStatus string `json:"certificateStatus,omitempty"`
	AccessURL         string `json:"accessUrl,omitempty"`
}

// GetDeploymentTargetDiagnostics aggregates point-in-time workload observation,
// runtime pods, Kubernetes events, recent releases, and bound gateway routes
// for one deployment target without exposing secrets or environment values.
func (h *Handlers) GetDeploymentTargetDiagnostics(ctx *gin.Context) {
	markLiveObservationResponse(ctx)
	user, project, ok := h.authorizeProject(ctx, authz.ActionDeploymentRead)
	if !ok {
		return
	}
	reqCtx := ctx.Request.Context()
	allowedGateway, err := h.projectRoleActionAllowed(reqCtx, user, project.ID, authz.ActionGatewayRead)
	if err != nil {
		writeProjectAuthorizationError(ctx, err)
		return
	}
	if !allowedGateway {
		writeProjectAuthorizationError(ctx, authz.ErrProjectAccessDenied)
		return
	}

	app, ok := h.findApplication(ctx)
	if !ok {
		return
	}
	var target model.DeploymentTarget
	if err := h.dbFor(ctx).First(&target, "id = ? and project_id = ? and application_id = ?", ctx.Param("targetId"), project.ID, app.ID).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "deployment target not found")
		return
	}

	observedTarget := h.observeDeploymentTarget(reqCtx, project, target)
	podsSnapshot, eventsSnapshot := h.observeDeploymentTargetPodsAndEvents(reqCtx, project, target)

	var releases []model.Release
	if err := h.dbFor(ctx).
		Where("project_id = ? and application_id = ? and deployment_target_id = ?", project.ID, app.ID, target.ID).
		Order("created_at desc, id desc").
		Limit(deploymentTargetDiagnosticsMaxReleases).
		Find(&releases).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	releaseItems := make([]deploymentTargetDiagnosticReleaseItem, 0, len(releases))
	for _, rel := range releases {
		releaseItems = append(releaseItems, deploymentTargetDiagnosticReleaseItem{
			ID:          rel.ID,
			Revision:    rel.Revision,
			Type:        rel.Type,
			ImageRef:    rel.ImageRef,
			Status:      rel.Status,
			FailureCode: rel.FailureCode,
			Message:     rel.Message,
			CreatedAt:   rel.CreatedAt,
			StartedAt:   rel.StartedAt,
			FinishedAt:  rel.FinishedAt,
		})
	}

	var routes []model.GatewayRoute
	if err := h.dbFor(ctx).
		Where(
			"project_id = ? and application_id = ? and deployment_target_id = ? and delete_status not in ?",
			project.ID,
			app.ID,
			target.ID,
			deploymentDiagnosticsExcludedRouteDeleteStatuses,
		).
		Order("created_at asc, id asc").
		Find(&routes).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	observedRoutes := h.host.ObserveGatewayRouteDiagnostics(reqCtx, routes)
	routeItems := make([]deploymentTargetDiagnosticRouteItem, 0, len(observedRoutes))
	for _, route := range observedRoutes {
		routeItems = append(routeItems, deploymentTargetDiagnosticRouteItem{
			ID:                route.ID,
			Host:              route.Host,
			Path:              route.Path,
			ServicePort:       route.ServicePort,
			TLSMode:           route.TLSMode,
			Enabled:           route.Enabled,
			Status:            route.Status,
			ObservationCode:   route.ObservationCode,
			CertificateStatus: route.CertificateStatus,
			AccessURL:         route.AccessURL,
		})
	}

	ctx.JSON(http.StatusOK, deploymentTargetDiagnosticsResponse{
		Target: deploymentTargetDiagnosticTarget{
			ID:                 observedTarget.ID,
			ProjectID:          observedTarget.ProjectID,
			ApplicationID:      observedTarget.ApplicationID,
			Name:               observedTarget.Name,
			Stage:              observedTarget.Stage,
			WorkloadType:       normalizeWorkloadType(observedTarget.WorkloadType),
			Enabled:            observedTarget.Enabled,
			EffectiveClusterID: observedTarget.EffectiveClusterID,
			ImageRef:           observedTarget.ImageRef,
			ConfiguredReplicas: observedTarget.Replicas,
			DesiredReplicas:    observedTarget.DesiredReplicas,
			ReadyReplicas:      observedTarget.ReadyReplicas,
			AvailableReplicas:  observedTarget.AvailableReplicas,
			UpdatedReplicas:    observedTarget.UpdatedReplicas,
			Status:             observedTarget.Status,
			ObservationCode:    observedTarget.ObservationCode,
			ObservedAt:         observedTarget.LastCheckedAt,
		},
		Pods:           podsSnapshot,
		Events:         eventsSnapshot,
		RecentReleases: releaseItems,
		AccessRoutes:   routeItems,
	})
}

func (h *Handlers) observeDeploymentTargetPodsAndEvents(
	ctx context.Context,
	project model.Project,
	target model.DeploymentTarget,
) (deploymentTargetDiagnosticPods, deploymentTargetDiagnosticEvents) {
	now := time.Now().UTC()
	if strings.TrimSpace(target.KubernetesName) == "" {
		return deploymentTargetDiagnosticPods{
				Status:     observation.StatusNotConfigured,
				Code:       "deployment_target.resource_name_not_configured",
				ObservedAt: now,
				Items:      []kubeprovider.RuntimePodSnapshot{},
			},
			deploymentTargetDiagnosticEvents{
				Status:     observation.StatusNotConfigured,
				Code:       "deployment_target.resource_name_not_configured",
				ObservedAt: now,
				Items:      []kubeprovider.DeploymentTargetEventSnapshot{},
			}
	}
	client, namespace, code := h.kubernetesClientForDeploymentTargetObservation(project, target, ctx)
	if code != "" {
		return deploymentTargetDiagnosticPods{
				Status:     observation.StatusUnavailable,
				Code:       "deployment_target.runtime_pods_" + code,
				ObservedAt: now,
				Items:      []kubeprovider.RuntimePodSnapshot{},
			},
			deploymentTargetDiagnosticEvents{
				Status:     observation.StatusUnavailable,
				Code:       "deployment_target.events_" + code,
				ObservedAt: now,
				Items:      []kubeprovider.DeploymentTargetEventSnapshot{},
			}
	}

	podsProbeCtx, cancelPods := context.WithTimeout(ctx, deploymentTargetObservationTimeout)
	rawPods, podsErr := client.RuntimePods(podsProbeCtx, namespace, target.ID, "")
	cancelPods()
	podsObservedAt := time.Now().UTC()
	var podsResult deploymentTargetDiagnosticPods
	if podsErr != nil {
		podsResult = deploymentTargetDiagnosticPods{
			Status:     observation.StatusUnavailable,
			Code:       "deployment_target.runtime_pods_unavailable",
			ObservedAt: podsObservedAt,
			Items:      []kubeprovider.RuntimePodSnapshot{},
		}
	} else {
		unhealthy := 0
		for _, pod := range rawPods {
			if !pod.Ready || pod.RestartCount > 0 {
				unhealthy++
			}
		}
		rankedPods := slices.Clone(rawPods)
		slices.SortStableFunc(rankedPods, func(left, right kubeprovider.RuntimePodSnapshot) int {
			if left.Ready != right.Ready {
				if !left.Ready {
					return -1
				}
				return 1
			}
			if left.RestartCount != right.RestartCount {
				if left.RestartCount > right.RestartCount {
					return -1
				}
				return 1
			}
			return 0
		})
		if len(rankedPods) > deploymentTargetDiagnosticsMaxPods {
			rankedPods = rankedPods[:deploymentTargetDiagnosticsMaxPods]
		}
		if rankedPods == nil {
			rankedPods = []kubeprovider.RuntimePodSnapshot{}
		}
		podsResult = deploymentTargetDiagnosticPods{
			Status:         observation.StatusReady,
			Code:           "deployment_target.runtime_pods_ready",
			ObservedAt:     podsObservedAt,
			Total:          len(rawPods),
			UnhealthyCount: unhealthy,
			Items:          rankedPods,
		}
	}

	eventsProbeCtx, cancelEvents := context.WithTimeout(ctx, deploymentTargetObservationTimeout)
	rawEvents, eventsErr := client.ListDeploymentTargetEvents(
		eventsProbeCtx,
		namespace,
		target.ID,
		target.KubernetesName,
		normalizeWorkloadType(target.WorkloadType),
	)
	cancelEvents()
	eventsObservedAt := time.Now().UTC()
	var eventsResult deploymentTargetDiagnosticEvents
	if eventsErr != nil {
		eventsResult = deploymentTargetDiagnosticEvents{
			Status:     observation.StatusUnavailable,
			Code:       "deployment_target.events_unavailable",
			ObservedAt: eventsObservedAt,
			Items:      []kubeprovider.DeploymentTargetEventSnapshot{},
		}
	} else {
		warnings := 0
		for _, ev := range rawEvents {
			if strings.EqualFold(strings.TrimSpace(ev.Type), "Warning") {
				warnings++
			}
		}
		rankedEvents := slices.Clone(rawEvents)
		slices.SortStableFunc(rankedEvents, func(left, right kubeprovider.DeploymentTargetEventSnapshot) int {
			leftWarn := strings.EqualFold(strings.TrimSpace(left.Type), "Warning")
			rightWarn := strings.EqualFold(strings.TrimSpace(right.Type), "Warning")
			if leftWarn != rightWarn {
				if leftWarn {
					return -1
				}
				return 1
			}
			return right.LastSeen.Compare(left.LastSeen)
		})
		if len(rankedEvents) > deploymentTargetDiagnosticsMaxEvents {
			rankedEvents = rankedEvents[:deploymentTargetDiagnosticsMaxEvents]
		}
		if rankedEvents == nil {
			rankedEvents = []kubeprovider.DeploymentTargetEventSnapshot{}
		}
		eventsResult = deploymentTargetDiagnosticEvents{
			Status:       observation.StatusReady,
			Code:         "deployment_target.events_ready",
			ObservedAt:   eventsObservedAt,
			Total:        len(rawEvents),
			WarningCount: warnings,
			Items:        rankedEvents,
		}
	}

	return podsResult, eventsResult
}
