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

// applicationDeploymentSummary is the single application runtime verdict
// shared by the dashboard, application list and application detail.
// Only enabled deployment targets with at least one succeeded release take
// part in Status and replica totals; disabled or never deployed targets are
// listed with their own status but never affect application health.
type applicationDeploymentSummary struct {
	TargetCount     int                                  `json:"targetCount"`
	DesiredReplicas int32                                `json:"desiredReplicas"`
	ReadyReplicas   int32                                `json:"readyReplicas"`
	Status          string                               `json:"status"`
	Targets         []applicationDeploymentTargetSummary `json:"targets"`
}

type applicationDeploymentTargetSummary struct {
	ID              string                     `json:"id"`
	Stage           string                     `json:"stage"`
	Enabled         bool                       `json:"enabled"`
	Deployed        bool                       `json:"deployed"`
	DesiredReplicas int32                      `json:"desiredReplicas"`
	ReadyReplicas   int32                      `json:"readyReplicas"`
	Status          string                     `json:"status"`
	ObservationCode string                     `json:"observationCode,omitempty"`
	CurrentRelease  *applicationCurrentRelease `json:"currentRelease,omitempty"`
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
	targetReleases, err := h.deploymentTargetCurrentReleases(ctx, project, targets)
	if err != nil {
		return nil, err
	}
	for _, application := range applications {
		applicationTargets := targetsByApplication[application.ID]
		items = append(items, applicationListItemResponse{
			Application:       application,
			DeploymentSummary: summarizeApplicationDeploymentTargets(applicationTargets, targetReleases),
			PrimaryAccessURL:  accessURLs[application.ID],
			CurrentRelease:    applicationCurrentReleaseOf(applicationTargets, targetReleases),
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

// deploymentTargetCurrentReleases 读取每个现存部署配置最近一次成功发布（按 finished_at）。
// 这是工作流结果，不代表 Kubernetes 当前运行的镜像。
func (h *Handlers) deploymentTargetCurrentReleases(ctx context.Context, project model.Project, targets []model.DeploymentTarget) (map[string]*applicationCurrentRelease, error) {
	if len(targets) == 0 {
		return map[string]*applicationCurrentRelease{}, nil
	}
	targetIDs := make([]string, 0, len(targets))
	for _, target := range targets {
		targetIDs = append(targetIDs, target.ID)
	}
	var releases []model.Release
	if err := h.dbWithContext(ctx).Model(&model.Release{}).
		Select("distinct on (deployment_target_id) *").
		Where("project_id = ? and deployment_target_id in ? and status = ? and finished_at is not null", project.ID, targetIDs, releaseStatusSucceeded).
		Order("deployment_target_id, finished_at desc, id desc").
		Find(&releases).Error; err != nil {
		return nil, fmt.Errorf("list deployment target current releases: %w", err)
	}
	return latestSucceededReleasesByTarget(releases), nil
}

func latestSucceededReleasesByTarget(releases []model.Release) map[string]*applicationCurrentRelease {
	result := make(map[string]*applicationCurrentRelease)
	for _, release := range releases {
		if release.Status != releaseStatusSucceeded || release.FinishedAt == nil {
			continue
		}
		candidate := &applicationCurrentRelease{
			ID:                 release.ID,
			DeploymentTargetID: release.DeploymentTargetID,
			ImageRef:           release.ImageRef,
			DeployedAt:         *release.FinishedAt,
		}
		if current, exists := result[release.DeploymentTargetID]; exists && !releaseFinishedLater(candidate, current) {
			continue
		}
		result[release.DeploymentTargetID] = candidate
	}
	return result
}

// applicationCurrentReleaseOf 是应用在现存部署配置上最近完成的成功发布。
func applicationCurrentReleaseOf(targets []model.DeploymentTarget, targetReleases map[string]*applicationCurrentRelease) *applicationCurrentRelease {
	var latest *applicationCurrentRelease
	for _, target := range targets {
		release := targetReleases[target.ID]
		if release != nil && (latest == nil || releaseFinishedLater(release, latest)) {
			latest = release
		}
	}
	return latest
}

// summarizeApplicationDeploymentTargets is the only application runtime
// aggregation. Per target status is "disabled" for disabled targets,
// "not-deployed" for enabled targets without a succeeded release, and the live
// Kubernetes observation otherwise. The application status is the most severe
// status among participating (enabled and deployed) targets, or "not-deployed"
// when none participates; replica totals only count participating targets.
func summarizeApplicationDeploymentTargets(targets []model.DeploymentTarget, targetReleases map[string]*applicationCurrentRelease) applicationDeploymentSummary {
	summary := applicationDeploymentSummary{
		TargetCount: len(targets),
		Status:      applicationRuntimeNotDeployed,
		Targets:     make([]applicationDeploymentTargetSummary, 0, len(targets)),
	}
	for _, target := range targets {
		release := targetReleases[target.ID]
		item := applicationDeploymentTargetSummary{
			ID:              target.ID,
			Stage:           target.Stage,
			Enabled:         target.Enabled,
			Deployed:        release != nil,
			DesiredReplicas: target.DesiredReplicas,
			ReadyReplicas:   target.ReadyReplicas,
			Status:          applicationTargetRuntimeStatus(target, release != nil),
			ObservationCode: target.ObservationCode,
			CurrentRelease:  release,
		}
		summary.Targets = append(summary.Targets, item)
		if !item.Enabled || !item.Deployed {
			continue
		}
		summary.DesiredReplicas += item.DesiredReplicas
		summary.ReadyReplicas += item.ReadyReplicas
		if summary.Status == applicationRuntimeNotDeployed || applicationDeploymentStatusPriority(item.Status) < applicationDeploymentStatusPriority(summary.Status) {
			summary.Status = item.Status
		}
	}
	sort.SliceStable(summary.Targets, func(i, j int) bool {
		left, right := summary.Targets[i], summary.Targets[j]
		if applicationDeploymentStatusPriority(left.Status) != applicationDeploymentStatusPriority(right.Status) {
			return applicationDeploymentStatusPriority(left.Status) < applicationDeploymentStatusPriority(right.Status)
		}
		return applicationDeploymentStagePriority(left.Stage) < applicationDeploymentStagePriority(right.Stage)
	})
	return summary
}

func applicationTargetRuntimeStatus(target model.DeploymentTarget, deployed bool) string {
	switch {
	case !target.Enabled:
		return observation.StatusDisabled
	case !deployed:
		return applicationRuntimeNotDeployed
	case target.Status == "":
		return observation.StatusUnknown
	default:
		return target.Status
	}
}

// applicationDeploymentStatusPriority orders statuses by how much attention
// they need; ready sorts before scaled-to-zero so a mix of both is ready.
func applicationDeploymentStatusPriority(status string) int {
	switch status {
	case observation.StatusUnavailable:
		return 0
	case observation.StatusDegraded:
		return 1
	case observation.StatusNotFound:
		return 2
	case observation.StatusNotConfigured:
		return 3
	case observation.StatusProgressing:
		return 4
	case observation.StatusReady:
		return 6
	case observation.StatusScaledToZero:
		return 7
	case applicationRuntimeNotDeployed:
		return 8
	case observation.StatusDisabled:
		return 9
	default:
		return 5
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

func releaseFinishedLater(release, current *applicationCurrentRelease) bool {
	if release.DeployedAt.Equal(current.DeployedAt) {
		return release.ID > current.ID
	}
	return release.DeployedAt.After(current.DeployedAt)
}
