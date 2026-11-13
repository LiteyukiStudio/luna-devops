package deploymentapi

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"time"

	transportapi "github.com/LiteyukiStudio/devops/internal/api/transport"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/observation"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/gin-gonic/gin"
)

var deploymentTargetEventSortFields = map[string]string{
	"lastSeen":  "lastSeen",
	"firstSeen": "firstSeen",
	"count":     "count",
	"reason":    "reason",
}

type deploymentTargetEventsResponse struct {
	transportapi.PaginatedResponseBody[kubeprovider.DeploymentTargetEventSnapshot]
	observation.Result
}

func (h *Handlers) ListDeploymentTargetEvents(ctx *gin.Context) {
	markLiveObservationResponse(ctx)
	_, project, ok := h.authorizeProject(ctx, authz.ActionDeploymentRead)
	if !ok {
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
	pagination := paginationFromQueryWithSort(ctx, deploymentTargetEventSortFields, "lastSeen")
	client, namespace, code := h.kubernetesClientForDeploymentTargetObservation(project, target, ctx.Request.Context())
	if code != "" {
		ctx.JSON(http.StatusOK, deploymentTargetEventsResponse{
			PaginatedResponseBody: paginatedResponse([]kubeprovider.DeploymentTargetEventSnapshot{}, 0, pagination),
			Result:                observation.Result{Status: observation.StatusUnavailable, Code: "deployment_target.events_" + code, ObservedAt: time.Now().UTC()},
		})
		return
	}
	probeCtx, cancel := context.WithTimeout(ctx.Request.Context(), deploymentTargetObservationTimeout)
	defer cancel()
	events, err := client.ListDeploymentTargetEvents(
		probeCtx,
		namespace,
		target.ID,
		target.KubernetesName,
		normalizeWorkloadType(target.WorkloadType),
	)
	if err != nil {
		ctx.JSON(http.StatusOK, deploymentTargetEventsResponse{
			PaginatedResponseBody: paginatedResponse([]kubeprovider.DeploymentTargetEventSnapshot{}, 0, pagination),
			Result:                observation.Result{Status: observation.StatusUnavailable, Code: "deployment_target.events_unavailable", ObservedAt: time.Now().UTC()},
		})
		return
	}
	events = filterDeploymentTargetEvents(events, ctx.Query("type"), ctx.Query("search"))
	sortDeploymentTargetEvents(events, pagination)
	ctx.JSON(http.StatusOK, deploymentTargetEventsResponse{
		PaginatedResponseBody: paginatedResponse(paginateSlice(events, pagination), int64(len(events)), pagination),
		Result:                observation.Result{Status: observation.StatusReady, Code: "deployment_target.events_ready", ObservedAt: time.Now().UTC()},
	})
}

func filterDeploymentTargetEvents(events []kubeprovider.DeploymentTargetEventSnapshot, eventType string, search string) []kubeprovider.DeploymentTargetEventSnapshot {
	typeFilter := strings.ToLower(strings.TrimSpace(eventType))
	keyword := strings.ToLower(strings.TrimSpace(search))
	if typeFilter == "" && keyword == "" {
		return events
	}
	return slices.DeleteFunc(events, func(item kubeprovider.DeploymentTargetEventSnapshot) bool {
		if typeFilter != "" && strings.ToLower(strings.TrimSpace(item.Type)) != typeFilter {
			return true
		}
		if keyword != "" {
			matched := strings.Contains(strings.ToLower(item.Reason), keyword) ||
				strings.Contains(strings.ToLower(item.Message), keyword) ||
				strings.Contains(strings.ToLower(item.InvolvedName), keyword) ||
				strings.Contains(strings.ToLower(item.InvolvedKind), keyword)
			if !matched {
				return true
			}
		}
		return false
	})
}

func sortDeploymentTargetEvents(events []kubeprovider.DeploymentTargetEventSnapshot, pagination paginationParams) {
	switch pagination.SortBy {
	case "firstSeen":
		slices.SortStableFunc(events, func(left, right kubeprovider.DeploymentTargetEventSnapshot) int {
			return left.FirstSeen.Compare(right.FirstSeen)
		})
	case "count":
		slices.SortStableFunc(events, func(left, right kubeprovider.DeploymentTargetEventSnapshot) int {
			if left.Count < right.Count {
				return -1
			}
			if left.Count > right.Count {
				return 1
			}
			return left.LastSeen.Compare(right.LastSeen)
		})
	case "reason":
		slices.SortStableFunc(events, func(left, right kubeprovider.DeploymentTargetEventSnapshot) int {
			return strings.Compare(left.Reason, right.Reason)
		})
	default:
		// Provider already returns lastSeen descending; reverse for ascending.
		if pagination.SortOrder == "asc" {
			slices.Reverse(events)
		}
		return
	}
	if pagination.SortOrder == "desc" {
		slices.Reverse(events)
	}
}
