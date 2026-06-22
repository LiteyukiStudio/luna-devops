package api

import (
	"sort"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
)

type clusterResourceYAMLResponse struct {
	YAML string `json:"yaml"`
}

type clusterResourceResponse struct {
	ID                   string            `json:"id"`
	Kind                 string            `json:"kind"`
	Name                 string            `json:"name"`
	Namespace            string            `json:"namespace"`
	Status               string            `json:"status"`
	Summary              string            `json:"summary"`
	ProjectID            string            `json:"projectId"`
	ApplicationID        string            `json:"applicationId"`
	EnvironmentID        string            `json:"environmentId"`
	DeploymentTargetID   string            `json:"deploymentTargetId"`
	ReleaseID            string            `json:"releaseId"`
	RouteID              string            `json:"routeId"`
	ProjectName          string            `json:"projectName"`
	ApplicationName      string            `json:"applicationName"`
	DeploymentTargetName string            `json:"deploymentTargetName"`
	Labels               map[string]string `json:"labels"`
	CreatedAt            time.Time         `json:"createdAt"`
	UpdatedAt            time.Time         `json:"updatedAt"`
}

func (h *Handlers) clusterResourceResponses(items []kubeprovider.ResourceSnapshot) ([]clusterResourceResponse, error) {
	responses := make([]clusterResourceResponse, 0, len(items))
	releaseIDs := make(map[string]bool)
	routeIDs := make(map[string]bool)
	for _, item := range items {
		responses = append(responses, clusterResourceResponse{
			ID:                 item.ID,
			Kind:               item.Kind,
			Name:               item.Name,
			Namespace:          item.Namespace,
			Status:             item.Status,
			Summary:            item.Summary,
			ProjectID:          item.ProjectID,
			ApplicationID:      item.ApplicationID,
			EnvironmentID:      item.EnvironmentID,
			DeploymentTargetID: item.DeploymentTargetID,
			ReleaseID:          item.ReleaseID,
			RouteID:            item.RouteID,
			Labels:             item.Labels,
			CreatedAt:          item.CreatedAt,
			UpdatedAt:          item.UpdatedAt,
		})
		addStringID(releaseIDs, item.ReleaseID)
		addStringID(routeIDs, item.RouteID)
	}

	releasesByID := make(map[string]model.Release)
	if ids := stringSetValues(releaseIDs); len(ids) > 0 {
		var releases []model.Release
		if err := h.db.Unscoped().Where("id in ?", ids).Find(&releases).Error; err != nil {
			return nil, err
		}
		for _, release := range releases {
			releasesByID[release.ID] = release
		}
	}

	routesByID := make(map[string]model.GatewayRoute)
	if ids := stringSetValues(routeIDs); len(ids) > 0 {
		var routes []model.GatewayRoute
		if err := h.db.Unscoped().Where("id in ?", ids).Find(&routes).Error; err != nil {
			return nil, err
		}
		for _, route := range routes {
			routesByID[route.ID] = route
		}
	}

	deploymentTargetIDs := make(map[string]bool)
	for index := range responses {
		response := &responses[index]
		if route, ok := routesByID[response.RouteID]; ok {
			if strings.TrimSpace(response.DeploymentTargetID) == "" {
				response.DeploymentTargetID = strings.TrimSpace(route.DeploymentTargetID)
			}
			fillResourceOwnerIDs(response, route.ProjectID, route.ApplicationID)
			if strings.TrimSpace(response.EnvironmentID) == "" {
				response.EnvironmentID = strings.TrimSpace(route.EnvironmentID)
			}
		}
		if release, ok := releasesByID[response.ReleaseID]; ok {
			if strings.TrimSpace(response.DeploymentTargetID) == "" {
				response.DeploymentTargetID = strings.TrimSpace(release.DeploymentTargetID)
			}
			fillResourceOwnerIDs(response, release.ProjectID, release.ApplicationID)
		}
		addStringID(deploymentTargetIDs, response.DeploymentTargetID)
	}

	targetsByID := make(map[string]model.DeploymentTarget)
	if ids := stringSetValues(deploymentTargetIDs); len(ids) > 0 {
		var targets []model.DeploymentTarget
		if err := h.db.Unscoped().Where("id in ?", ids).Find(&targets).Error; err != nil {
			return nil, err
		}
		for _, target := range targets {
			targetsByID[target.ID] = target
		}
	}

	projectIDs := make(map[string]bool)
	applicationIDs := make(map[string]bool)
	deploymentTargetNameByID := make(map[string]string)
	for index := range responses {
		response := &responses[index]
		if target, ok := targetsByID[response.DeploymentTargetID]; ok {
			fillResourceOwnerIDs(response, target.ProjectID, target.ApplicationID)
			deploymentTargetNameByID[target.ID] = target.Name
		}
		addStringID(projectIDs, response.ProjectID)
		addStringID(applicationIDs, response.ApplicationID)
	}

	projectNames, err := h.projectNamesByID(projectIDs)
	if err != nil {
		return nil, err
	}
	applicationNames, err := h.applicationNamesByID(applicationIDs)
	if err != nil {
		return nil, err
	}
	for index := range responses {
		responses[index].ProjectName = projectNames[responses[index].ProjectID]
		responses[index].ApplicationName = applicationNames[responses[index].ApplicationID]
		responses[index].DeploymentTargetName = deploymentTargetNameByID[responses[index].DeploymentTargetID]
	}
	return responses, nil
}

func fillResourceOwnerIDs(response *clusterResourceResponse, projectID string, applicationID string) {
	if strings.TrimSpace(response.ProjectID) == "" {
		response.ProjectID = strings.TrimSpace(projectID)
	}
	if strings.TrimSpace(response.ApplicationID) == "" {
		response.ApplicationID = strings.TrimSpace(applicationID)
	}
}

func (h *Handlers) projectNamesByID(ids map[string]bool) (map[string]string, error) {
	names := make(map[string]string)
	if values := stringSetValues(ids); len(values) > 0 {
		var projects []model.Project
		if err := h.db.Unscoped().Where("id in ?", values).Find(&projects).Error; err != nil {
			return nil, err
		}
		for _, project := range projects {
			names[project.ID] = project.Name
		}
	}
	return names, nil
}

func sortClusterResourceResponses(items []clusterResourceResponse, pagination paginationParams) {
	sortBy := normalizeClusterResourceSortBy(pagination.SortBy)
	desc := pagination.SortOrder != "asc"
	sort.SliceStable(items, func(i, j int) bool {
		result := compareClusterResourceResponse(items[i], items[j], sortBy)
		if result != 0 {
			if desc {
				return result > 0
			}
			return result < 0
		}
		return compareClusterResourceTieBreaker(items[i], items[j]) < 0
	})
}

func normalizeClusterResourceSortBy(sortBy string) string {
	switch strings.ToLower(strings.TrimSpace(sortBy)) {
	case "kind", "name", "namespace", "status", "owner", "summary":
		return strings.ToLower(strings.TrimSpace(sortBy))
	case "createdat":
		return "createdAt"
	case "updatedat":
		return "updatedAt"
	default:
		return "updatedAt"
	}
}

func compareClusterResourceResponse(left clusterResourceResponse, right clusterResourceResponse, sortBy string) int {
	switch sortBy {
	case "kind":
		return compareFold(left.Kind, right.Kind)
	case "name":
		return compareFold(left.Name, right.Name)
	case "namespace":
		return compareFold(left.Namespace, right.Namespace)
	case "status":
		return compareFold(left.Status, right.Status)
	case "owner":
		return compareFold(clusterResourceOwnerSortValue(left), clusterResourceOwnerSortValue(right))
	case "summary":
		return compareFold(left.Summary, right.Summary)
	case "createdAt":
		return compareTime(left.CreatedAt, right.CreatedAt)
	default:
		return compareTime(left.UpdatedAt, right.UpdatedAt)
	}
}

func compareClusterResourceTieBreaker(left clusterResourceResponse, right clusterResourceResponse) int {
	for _, result := range []int{
		compareFold(left.Kind, right.Kind),
		compareFold(left.Namespace, right.Namespace),
		compareFold(left.Name, right.Name),
	} {
		if result != 0 {
			return result
		}
	}
	return 0
}

func clusterResourceOwnerSortValue(item clusterResourceResponse) string {
	parts := []string{item.ProjectName, item.ApplicationName, item.DeploymentTargetName}
	return strings.Join(parts, "/")
}

func compareFold(left string, right string) int {
	return strings.Compare(strings.ToLower(strings.TrimSpace(left)), strings.ToLower(strings.TrimSpace(right)))
}

func compareTime(left time.Time, right time.Time) int {
	if left.Before(right) {
		return -1
	}
	if left.After(right) {
		return 1
	}
	return 0
}

func (h *Handlers) applicationNamesByID(ids map[string]bool) (map[string]string, error) {
	names := make(map[string]string)
	if values := stringSetValues(ids); len(values) > 0 {
		var applications []model.Application
		if err := h.db.Unscoped().Where("id in ?", values).Find(&applications).Error; err != nil {
			return nil, err
		}
		for _, application := range applications {
			names[application.ID] = application.Name
		}
	}
	return names, nil
}

func addStringID(ids map[string]bool, value string) {
	normalized := strings.TrimSpace(value)
	if normalized != "" {
		ids[normalized] = true
	}
}

func stringSetValues(ids map[string]bool) []string {
	values := make([]string, 0, len(ids))
	for value := range ids {
		values = append(values, value)
	}
	return values
}
