package applicationapi

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/observation"
	"gorm.io/gorm"
)

const applicationRuntimeNotDeployed = "not-deployed"

type applicationDeploymentSummary struct {
	TargetCount     int                                  `json:"targetCount"`
	DesiredReplicas int32                                `json:"desiredReplicas"`
	ReadyReplicas   int32                                `json:"readyReplicas"`
	Status          string                               `json:"status"`
	Targets         []applicationDeploymentTargetSummary `json:"targets"`
}

type applicationDeploymentTargetSummary struct {
	ID              string `json:"id"`
	Stage           string `json:"stage"`
	DesiredReplicas int32  `json:"desiredReplicas"`
	ReadyReplicas   int32  `json:"readyReplicas"`
	Status          string `json:"status"`
}

// applicationCurrentRelease 是应用最近一次成功发布的工作流结果，不代表 Kubernetes 当前实时状态。
type applicationCurrentRelease struct {
	ID                 string    `json:"id"`
	DeploymentTargetID string    `json:"deploymentTargetId"`
	ImageRef           string    `json:"imageRef"`
	DeployedAt         time.Time `json:"deployedAt"`
}

type applicationListItemResponse struct {
	model.Application
	DeploymentSummary applicationDeploymentSummary `json:"deploymentSummary"`
	PrimaryAccessURL  string                       `json:"primaryAccessUrl,omitempty"`
	CurrentRelease    *applicationCurrentRelease   `json:"currentRelease,omitempty"`
}

const releaseStatusSucceeded = "succeeded"

func (h *Handlers) applicationListItemsWithRuntime(ctx context.Context, project model.Project, applications []model.Application) ([]applicationListItemResponse, error) {
	items := make([]applicationListItemResponse, 0, len(applications))
	if len(applications) == 0 {
		return items, nil
	}

	applicationIDs := make([]string, 0, len(applications))
	for _, application := range applications {
		applicationIDs = append(applicationIDs, application.ID)
	}
	var targets []model.DeploymentTarget
	if err := h.dbWithContext(ctx).
		Where("project_id = ? and application_id in ? and delete_status <> ?", project.ID, applicationIDs, "deleted").
		Find(&targets).Error; err != nil {
		return nil, err
	}
	h.host.ObserveDeploymentTargets(ctx, project, targets)

	targetsByApplication := make(map[string][]model.DeploymentTarget, len(applications))
	for _, target := range targets {
		targetsByApplication[target.ApplicationID] = append(targetsByApplication[target.ApplicationID], target)
	}
	accessURLs, err := h.applicationPrimaryAccessURLs(ctx, project, applicationIDs, targets)
	if err != nil {
		return nil, err
	}
	currentReleases, err := h.applicationCurrentReleases(ctx, project, applicationIDs, targets)
	if err != nil {
		return nil, err
	}
	for _, application := range applications {
		items = append(items, applicationListItemResponse{
			Application:       application,
			DeploymentSummary: summarizeApplicationDeploymentTargets(targetsByApplication[application.ID]),
			PrimaryAccessURL:  accessURLs[application.ID],
			CurrentRelease:    currentReleases[application.ID],
		})
	}
	return items, nil
}

// applicationPrimaryAccessURLs 按网关路由配置生成每个应用的主访问地址；只读取期望配置，不探测连通性。
func (h *Handlers) applicationPrimaryAccessURLs(ctx context.Context, project model.Project, applicationIDs []string, targets []model.DeploymentTarget) (map[string]string, error) {
	result := make(map[string]string)
	if len(targets) == 0 {
		return result, nil
	}
	var routes []model.GatewayRoute
	if err := h.dbWithContext(ctx).
		Where("project_id = ? and application_id in ? and enabled = ? and delete_status = ?", project.ID, applicationIDs, true, "active").
		Find(&routes).Error; err != nil {
		return nil, fmt.Errorf("list application gateway routes: %w", err)
	}
	targetsByID := make(map[string]model.DeploymentTarget, len(targets))
	for _, target := range targets {
		targetsByID[target.ID] = target
	}
	for applicationID, route := range primaryGatewayRoutes(routes, targetsByID) {
		accessURL, err := h.host.GatewayRouteAccessURL(ctx, route, targetsByID[route.DeploymentTargetID])
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("resolve application primary access URL: %w", err)
		}
		if accessURL != "" {
			result[applicationID] = accessURL
		}
	}
	return result, nil
}

// primaryGatewayRoutes 为每个应用选择主路由：默认路由优先，其次按部署阶段优先级和创建时间。
func primaryGatewayRoutes(routes []model.GatewayRoute, targetsByID map[string]model.DeploymentTarget) map[string]model.GatewayRoute {
	candidates := make([]model.GatewayRoute, 0, len(routes))
	for _, route := range routes {
		if !route.Enabled || route.Host == "" {
			continue
		}
		if _, ok := targetsByID[route.DeploymentTargetID]; !ok {
			continue
		}
		candidates = append(candidates, route)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		left, right := candidates[i], candidates[j]
		if left.IsDefault != right.IsDefault {
			return left.IsDefault
		}
		leftStage := applicationDeploymentStagePriority(targetsByID[left.DeploymentTargetID].Stage)
		rightStage := applicationDeploymentStagePriority(targetsByID[right.DeploymentTargetID].Stage)
		if leftStage != rightStage {
			return leftStage < rightStage
		}
		if !left.CreatedAt.Equal(right.CreatedAt) {
			return left.CreatedAt.Before(right.CreatedAt)
		}
		return left.ID < right.ID
	})
	result := make(map[string]model.GatewayRoute)
	for _, route := range candidates {
		if _, exists := result[route.ApplicationID]; !exists {
			result[route.ApplicationID] = route
		}
	}
	return result
}

// applicationCurrentReleases 读取每个应用在现存部署配置上最近一次成功的发布记录。
func (h *Handlers) applicationCurrentReleases(ctx context.Context, project model.Project, applicationIDs []string, targets []model.DeploymentTarget) (map[string]*applicationCurrentRelease, error) {
	if len(targets) == 0 {
		return map[string]*applicationCurrentRelease{}, nil
	}
	targetIDs := make([]string, 0, len(targets))
	for _, target := range targets {
		targetIDs = append(targetIDs, target.ID)
	}
	db := h.dbWithContext(ctx)
	latest := db.Model(&model.Release{}).
		Select("application_id, max(finished_at)").
		Where("project_id = ? and application_id in ? and deployment_target_id in ? and status = ?", project.ID, applicationIDs, targetIDs, releaseStatusSucceeded).
		Group("application_id")
	var releases []model.Release
	if err := db.
		Where("project_id = ? and deployment_target_id in ? and status = ? and (application_id, finished_at) in (?)", project.ID, targetIDs, releaseStatusSucceeded, latest).
		Find(&releases).Error; err != nil {
		return nil, fmt.Errorf("list application current releases: %w", err)
	}
	return latestSucceededReleases(releases), nil
}

func latestSucceededReleases(releases []model.Release) map[string]*applicationCurrentRelease {
	result := make(map[string]*applicationCurrentRelease)
	for _, release := range releases {
		if release.Status != releaseStatusSucceeded || release.FinishedAt == nil {
			continue
		}
		if current, exists := result[release.ApplicationID]; exists && !releaseFinishedLater(release, current) {
			continue
		}
		result[release.ApplicationID] = &applicationCurrentRelease{
			ID:                 release.ID,
			DeploymentTargetID: release.DeploymentTargetID,
			ImageRef:           release.ImageRef,
			DeployedAt:         *release.FinishedAt,
		}
	}
	return result
}

func summarizeApplicationDeploymentTargets(targets []model.DeploymentTarget) applicationDeploymentSummary {
	summary := applicationDeploymentSummary{
		TargetCount: len(targets),
		Status:      applicationRuntimeNotDeployed,
		Targets:     make([]applicationDeploymentTargetSummary, 0, len(targets)),
	}
	if len(targets) == 0 {
		return summary
	}

	allObservedTargetsHealthy := true
	hasRuntimeObservation := false
	for _, target := range targets {
		summary.Targets = append(summary.Targets, applicationDeploymentTargetSummary{
			ID:              target.ID,
			Stage:           target.Stage,
			DesiredReplicas: target.DesiredReplicas,
			ReadyReplicas:   target.ReadyReplicas,
			Status:          target.Status,
		})
		summary.DesiredReplicas += target.DesiredReplicas
		summary.ReadyReplicas += target.ReadyReplicas
		if target.Status == observation.StatusReady || target.Status == observation.StatusScaledToZero || target.Status == observation.StatusProgressing || target.Status == observation.StatusDegraded {
			hasRuntimeObservation = true
		}
		if target.Status != observation.StatusReady && target.Status != observation.StatusScaledToZero {
			allObservedTargetsHealthy = false
		}
		switch target.Status {
		case observation.StatusUnavailable:
			summary.Status = observation.StatusUnavailable
		case observation.StatusDegraded:
			if summary.Status != observation.StatusUnavailable {
				summary.Status = observation.StatusDegraded
			}
		}
	}
	sort.SliceStable(summary.Targets, func(i, j int) bool {
		left, right := summary.Targets[i], summary.Targets[j]
		if applicationDeploymentStatusPriority(left.Status) != applicationDeploymentStatusPriority(right.Status) {
			return applicationDeploymentStatusPriority(left.Status) < applicationDeploymentStatusPriority(right.Status)
		}
		return applicationDeploymentStagePriority(left.Stage) < applicationDeploymentStagePriority(right.Stage)
	})

	if summary.Status == observation.StatusUnavailable || summary.Status == observation.StatusDegraded {
		return summary
	}
	if allObservedTargetsHealthy && hasRuntimeObservation && summary.DesiredReplicas == 0 {
		summary.Status = observation.StatusScaledToZero
		return summary
	}
	if allObservedTargetsHealthy && hasRuntimeObservation && summary.ReadyReplicas >= summary.DesiredReplicas {
		summary.Status = observation.StatusReady
		return summary
	}
	if hasRuntimeObservation {
		summary.Status = observation.StatusProgressing
	}
	return summary
}

func applicationDeploymentStatusPriority(status string) int {
	switch status {
	case observation.StatusUnavailable:
		return 0
	case observation.StatusDegraded:
		return 1
	case observation.StatusProgressing:
		return 2
	case observation.StatusNotFound:
		return 3
	case observation.StatusNotConfigured:
		return 4
	case observation.StatusReady:
		return 5
	case observation.StatusScaledToZero:
		return 6
	case "disabled":
		return 7
	default:
		return 8
	}
}

func applicationDeploymentStagePriority(stage string) int {
	switch stage {
	case "prod":
		return 0
	case "staging":
		return 1
	case "test":
		return 2
	case "dev":
		return 3
	default:
		return 4
	}
}

func releaseFinishedLater(release model.Release, current *applicationCurrentRelease) bool {
	if release.FinishedAt.Equal(current.DeployedAt) {
		return release.ID > current.ID
	}
	return release.FinishedAt.After(current.DeployedAt)
}
