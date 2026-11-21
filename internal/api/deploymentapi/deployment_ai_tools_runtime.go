package deploymentapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/aitool"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/observation"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"gorm.io/gorm"
)

const (
	aiRuntimeExecStreamCapBytes = 32768
)

// --- Tool 15: runtime_get_target_diagnostics ---

type runtimeGetTargetDiagnosticsInput struct {
	Project     string `json:"project" required:"true" desc:"项目 ID（prj_xxx）或唯一标识（identifier）"`
	Application string `json:"application" required:"true" desc:"应用 ID（app_xxx）、唯一标识或唯一名称"`
	Target      string `json:"target" required:"true" desc:"部署目标 ID（dplt_xxx）、stage 或同应用唯一名称"`
}

type aiDiagnosticVolumeMountItem struct {
	BindingID           string                         `json:"bindingId"`
	LogicalName         string                         `json:"logicalName"`
	SourceType          string                         `json:"sourceType"`
	MountPath           string                         `json:"mountPath,omitempty"`
	DevicePath          string                         `json:"devicePath,omitempty"`
	ReadOnly            bool                           `json:"readOnly"`
	ActivationState     string                         `json:"activationState"`
	EmptyDir            *deploymentTargetEmptyDirInput `json:"emptyDir,omitempty"`
	ProjectVolumeID     string                         `json:"projectVolumeId,omitempty"`
	VolumeDisplayName   string                         `json:"volumeDisplayName,omitempty"`
	ClaimName           string                         `json:"claimName,omitempty"`
	VolumeLifecycle     string                         `json:"volumeLifecycleState,omitempty"`
	RequestedCapacity   string                         `json:"requestedCapacity,omitempty"`
	StorageClassName    string                         `json:"storageClassName,omitempty"`
	AccessMode          string                         `json:"accessMode,omitempty"`
	VolumeMode          string                         `json:"volumeMode,omitempty"`
	PVCExists           *bool                          `json:"pvcExists,omitempty"`
	PVCPhase            string                         `json:"pvcPhase,omitempty"`
	PVCObservedCapacity string                         `json:"pvcObservedCapacity,omitempty"`
	ObservationStatus   string                         `json:"status,omitempty"`
	ObservationCode     string                         `json:"observationCode,omitempty"`
}

type runtimeGetTargetDiagnosticsOutput struct {
	ProjectID          string                                  `json:"projectId"`
	ApplicationID      string                                  `json:"applicationId"`
	DeploymentTargetID string                                  `json:"deploymentTargetId"`
	Stage              string                                  `json:"stage"`
	Target             deploymentTargetDiagnosticTarget        `json:"target"`
	MetricsAvailable   bool                                    `json:"metricsAvailable"`
	Metrics            deploymentTargetMetricsResponse         `json:"metrics"`
	Pods               deploymentTargetDiagnosticPods          `json:"pods"`
	Events             deploymentTargetDiagnosticEvents        `json:"events"`
	VolumeMounts       []aiDiagnosticVolumeMountItem           `json:"volumeMounts"`
	RecentReleases     []deploymentTargetDiagnosticReleaseItem `json:"recentReleases"`
	AccessRoutes       []deploymentTargetDiagnosticRouteItem   `json:"accessRoutes"`
}

// --- Tool 16: runtime_list_target_pods_and_events ---

type runtimeListTargetPodsAndEventsInput struct {
	Project       string `json:"project" required:"true" desc:"项目 ID（prj_xxx）或唯一标识（identifier）"`
	Application   string `json:"application" required:"true" desc:"应用 ID（app_xxx）、唯一标识或唯一名称"`
	Target        string `json:"target" required:"true" desc:"部署目标 ID（dplt_xxx）、stage 或同应用唯一名称"`
	ReleaseID     string `json:"releaseId,omitempty" desc:"可选按特定发布记录 ID 筛选关联 Pod"`
	EventType     string `json:"eventType,omitempty" enum:"Warning|Normal" desc:"可选筛选事件级别（排障推荐 Warning）"`
	Search        string `json:"search,omitempty" desc:"可选按 Pod 名、事件原因或消息模糊过滤"`
	IncludePods   *bool  `json:"includePods,omitempty" default:"true" desc:"是否返回实时 Pod 明细列表"`
	IncludeEvents *bool  `json:"includeEvents,omitempty" default:"true" desc:"是否返回关联 K8s 事件流"`
	PodLimit      *int   `json:"podLimit,omitempty" min:"1" max:"50" default:"20" desc:"返回 Pod 条数上限（1..50，默认 20）"`
	EventLimit    *int   `json:"eventLimit,omitempty" min:"1" max:"50" default:"20" desc:"返回 Event 条数上限（1..50，默认 20）"`
}

type aiPodsSectionOutput struct {
	Status          string                            `json:"status"`
	ObservationCode string                            `json:"observationCode"`
	ObservedAt      time.Time                         `json:"observedAt"`
	Total           int                               `json:"total"`
	Items           []kubeprovider.RuntimePodSnapshot `json:"items"`
}

type aiEventsSectionOutput struct {
	Status          string                                       `json:"status"`
	ObservationCode string                                       `json:"observationCode"`
	ObservedAt      time.Time                                    `json:"observedAt"`
	Total           int                                          `json:"total"`
	Items           []kubeprovider.DeploymentTargetEventSnapshot `json:"items"`
}

type runtimeListTargetPodsAndEventsOutput struct {
	ProjectID          string                 `json:"projectId"`
	ApplicationID      string                 `json:"applicationId"`
	DeploymentTargetID string                 `json:"deploymentTargetId"`
	Stage              string                 `json:"stage"`
	Pods               *aiPodsSectionOutput   `json:"pods,omitempty"`
	Events             *aiEventsSectionOutput `json:"events,omitempty"`
}

// --- Tool 17: runtime_get_pod_logs ---

type runtimeGetPodLogsInput struct {
	Project        string `json:"project" required:"true" desc:"项目 ID（prj_xxx）或唯一标识（identifier）"`
	Application    string `json:"application,omitempty" desc:"应用 ID、唯一标识或唯一名称（传 target 时必填）"`
	Target         string `json:"target,omitempty" desc:"部署目标 ID、stage 或唯一名称（target 与 releaseId 至少传其一）"`
	ReleaseID      string `json:"releaseId,omitempty" desc:"发布记录 ID（rel_xxx；与 target 至少传其一）"`
	Pod            string `json:"pod,omitempty" desc:"可选指定单个 Pod 名称（省略且 preferAbnormal=true 时优先选取异常 Pod）"`
	Container      string `json:"container,omitempty" desc:"可选指定普通容器或 initContainer 名称"`
	Previous       *bool  `json:"previous,omitempty" default:"false" desc:"是否读取上一次崩溃退出容器的终止日志"`
	PreferAbnormal *bool  `json:"preferAbnormal,omitempty" default:"true" desc:"省略 pod 时是否自动优先选取异常或重启中的 Pod"`
	Timestamps     *bool  `json:"timestamps,omitempty" default:"true" desc:"是否在每行日志前附加时间戳"`
	TailLines      *int   `json:"tailLines,omitempty" min:"1" max:"1000" default:"200" desc:"读取尾部行数（1..1000，默认 200）"`
	MaxBytes       *int   `json:"maxBytes,omitempty" min:"256" max:"65536" default:"32768" desc:"返回日志字节硬上限（默认 32768，上限 65536，按 UTF-8 字符边界尾部截断）"`
}

type runtimeGetPodLogsOutput struct {
	ProjectID          string `json:"projectId"`
	ApplicationID      string `json:"applicationId"`
	DeploymentTargetID string `json:"deploymentTargetId"`
	Stage              string `json:"stage"`
	ReleaseID          string `json:"releaseId,omitempty"`
	Pod                string `json:"pod"`
	Container          string `json:"container"`
	Previous           bool   `json:"previous"`
	Content            string `json:"content"`
	Truncated          bool   `json:"truncated"`
	TruncatedBytes     int    `json:"truncatedBytes,omitempty"`
}

// --- Tool 18: runtime_exec_command ---

type runtimeExecCommandInput struct {
	Project     string `json:"project" required:"true" desc:"项目 ID（prj_xxx）或唯一标识（identifier）"`
	ReleaseID   string `json:"releaseId,omitempty" desc:"可选指定发布记录 ID（省略时必填 application + target）"`
	Application string `json:"application,omitempty" desc:"应用 ID、唯一标识或唯一名称（省略 releaseId 时必填）"`
	Target      string `json:"target,omitempty" desc:"部署目标 ID、stage 或唯一名称（省略 releaseId 时必填）"`
	Pod         string `json:"pod,omitempty" desc:"可选指定单个运行 Pod 名称"`
	Container   string `json:"container,omitempty" desc:"可选指定运行容器名称"`
	Command     string `json:"command" required:"true" min:"1" max:"2000" desc:"通过 /bin/sh -lc 执行的单次非交互诊断命令（1..2000 字符，30s 超时）"`
}

type runtimeExecCommandOutput struct {
	ProjectID          string `json:"projectId"`
	ApplicationID      string `json:"applicationId"`
	DeploymentTargetID string `json:"deploymentTargetId"`
	Stage              string `json:"stage"`
	ReleaseID          string `json:"releaseId"`
	Pod                string `json:"pod"`
	Container          string `json:"container"`
	Stdout             string `json:"stdout"`
	Stderr             string `json:"stderr"`
	ExitCode           int    `json:"exitCode"`
	Truncated          bool   `json:"truncated"`
	DurationMs         int64  `json:"durationMs"`
}

func (h *Handler) registerRuntimeDiagnosticsAITools(reg *aitool.Registry) {
	aitool.Register(reg, aitool.Spec[runtimeGetTargetDiagnosticsInput, runtimeGetTargetDiagnosticsOutput]{
		Name:           "runtime_get_target_diagnostics",
		Domain:         aitool.DomainRuntime,
		Title:          "一站式读取部署环境健康诊断与实时指标快照",
		Summary:        "一站式读取单个部署环境的健康状态、单帧 Pod CPU/内存实时指标、异常优先 Top 10 Pods、Warning 优先 Top 10 Events、数据卷 PVC 实时状态、最近 5 次发布及网关路由状态。",
		AliasesZH:      []string{"环境诊断", "容器排障快照", "查看CPU内存监控", "健康巡检"},
		AliasesEN:      []string{"target diagnostics", "pod health snapshot", "pod metrics"},
		Purpose:        aitool.Text("容器排障首选入口：一次获取工作负载健康度、实时 CPU/内存利用率、异常 Pod、告警事件、存储挂载状态、最近发布与网关证书状态。", "Primary container troubleshooting entry point aggregating workload status, live CPU/memory metrics, top unhealthy Pods, warning Events, PVC volume mount status, recent releases, and gateway routes."),
		ApprovalTier:   aitool.ApprovalTierL0,
		Idempotent:     true,
		RequiredScopes: []string{string(authz.ActionDeploymentRead), string(authz.ActionGatewayRead), string(authz.ActionVolumeRead)},
		Effect:         aitool.EffectRead,
		Execute:        h.executeRuntimeGetTargetDiagnostics,
	})

	aitool.Register(reg, aitool.Spec[runtimeListTargetPodsAndEventsInput, runtimeListTargetPodsAndEventsOutput]{
		Name:           "runtime_list_target_pods_and_events",
		Domain:         aitool.DomainRuntime,
		Title:          "查看部署环境实时 Pod 列表与集群事件流",
		Summary:        "实时读取部署目标下的 Kubernetes Pod 明细列表与关联集群事件流，支持按发布版本、事件级别或关键词过滤。",
		AliasesZH:      []string{"查看Pod列表", "查看K8s事件", "容器事件排障"},
		AliasesEN:      []string{"list target pods", "list k8s events"},
		Purpose:        aitool.Text("读取指定部署环境的实时 Pod 状态明细及 Kubernetes Warning/Normal 事件流。", "Read live Kubernetes Pod snapshots and cluster Events for a deployment target."),
		ApprovalTier:   aitool.ApprovalTierL0,
		Idempotent:     true,
		RequiredScopes: []string{string(authz.ActionDeploymentRead)},
		Effect:         aitool.EffectRead,
		Execute:        h.executeRuntimeListTargetPodsAndEvents,
	})

	aitool.Register(reg, aitool.Spec[runtimeGetPodLogsInput, runtimeGetPodLogsOutput]{
		Name:           "runtime_get_pod_logs",
		Domain:         aitool.DomainRuntime,
		Title:          "读取容器实时或崩溃前终止日志",
		Summary:        "读取部署环境或指定发布版本的容器当前日志或崩溃前日志（带 UTF-8 字节硬上限截断保护）。",
		AliasesZH:      []string{"查看容器日志", "Pod日志", "崩溃日志"},
		AliasesEN:      []string{"get pod logs", "container logs", "crashloop logs"},
		Purpose:        aitool.Text("按字节与行数上限安全读取线上 Pod 容器（含 initContainer 或崩溃前实例）的标准输出日志。", "Read current or previous terminated container logs from a runtime Pod with UTF-8 byte-cap protection."),
		ApprovalTier:   aitool.ApprovalTierL0,
		Idempotent:     true,
		RequiredScopes: []string{string(authz.ActionDeploymentRead)},
		Effect:         aitool.EffectRead,
		Execute:        h.executeRuntimeGetPodLogs,
	})

	aitool.Register(reg, aitool.Spec[runtimeExecCommandInput, runtimeExecCommandOutput]{
		Name:           "runtime_exec_command",
		Domain:         aitool.DomainRuntime,
		Title:          "在线上容器中执行单次非交互诊断命令",
		Summary:        "在已开启 Web 控制台权限的线上容器中通过 /bin/sh -lc 执行单次非交互诊断命令（30s 超时，返回 exitCode/stdout/stderr）。",
		AliasesZH:      []string{"容器执行命令", "Pod Exec", "容器内诊断"},
		AliasesEN:      []string{"exec command in pod", "container exec"},
		Purpose:        aitool.Text("在允许 Web 控制台的运行容器内执行单次非交互 Shell 命令用于深度排障，并记录完整审计。", "Execute a single non-interactive `/bin/sh -lc` diagnostic command inside a live container when Web Console policy allows."),
		ApprovalTier:   aitool.ApprovalTierL2,
		RequiredScopes: []string{string(authz.ActionDeploymentExec)},
		Effect:         aitool.EffectExecute,
		Preview:        h.previewRuntimeExecCommand,
		Execute:        h.executeRuntimeExecCommand,
	})
}

// ============================================================================
// Tool 15 Implementation: runtime_get_target_diagnostics
// ============================================================================

func (h *Handler) executeRuntimeGetTargetDiagnostics(
	tc *aitool.ToolContext,
	in runtimeGetTargetDiagnosticsInput,
) (runtimeGetTargetDiagnosticsOutput, error) {
	tc.SetNoStore()
	user, project, err := h.authorizeAIProject(tc, in.Project, authz.ActionDeploymentRead)
	if err != nil {
		return runtimeGetTargetDiagnosticsOutput{}, err
	}
	ctx := tc.Context()
	for _, extraAction := range []authz.Action{authz.ActionGatewayRead, authz.ActionVolumeRead} {
		allowed, aErr := h.projectRoleActionAllowed(ctx, user, project.ID, extraAction)
		if aErr != nil || !allowed {
			return runtimeGetTargetDiagnosticsOutput{}, aitool.Forbidden("project.access_denied", string(extraAction)+" permission is required")
		}
	}

	db := h.aiDB(tc)
	app, err := aitool.ResolveApplication(db, project.ID, in.Application)
	if err != nil {
		return runtimeGetTargetDiagnosticsOutput{}, err
	}
	target, err := aitool.ResolveDeploymentTarget(db, project.ID, app.ID, in.Target)
	if err != nil {
		return runtimeGetTargetDiagnosticsOutput{}, err
	}

	observedTarget := h.observeDeploymentTarget(ctx, project, target)
	podsSnapshot, eventsSnapshot := h.observeDeploymentTargetPodsAndEvents(ctx, project, target)

	// Single-frame live Pod CPU/Memory metrics (degrading gracefully with metricsAvailable: false)
	metricsResp := h.observeSingleFrameTargetMetrics(ctx, project, target)

	// Aggregate volumeMounts + live ProjectVolume PVC phase/observationCode
	volItems, err := h.observeTargetDiagnosticVolumeMounts(ctx, db, project, target)
	if err != nil {
		return runtimeGetTargetDiagnosticsOutput{}, err
	}

	// Recent 5 Releases
	var releases []model.Release
	if err := db.Where("project_id = ? AND application_id = ? AND deployment_target_id = ?", project.ID, app.ID, target.ID).
		Order("created_at desc, id desc").
		Limit(deploymentTargetDiagnosticsMaxReleases).
		Find(&releases).Error; err != nil {
		return runtimeGetTargetDiagnosticsOutput{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
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

	// GatewayRoutes + TLS status
	var routes []model.GatewayRoute
	if err := db.Where(
		"project_id = ? AND application_id = ? AND deployment_target_id = ? AND delete_status NOT IN ?",
		project.ID, app.ID, target.ID, deploymentDiagnosticsExcludedRouteDeleteStatuses,
	).Order("created_at asc, id asc").Find(&routes).Error; err != nil {
		return runtimeGetTargetDiagnosticsOutput{}, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	observedRoutes := h.host.ObserveGatewayRouteDiagnostics(ctx, routes)
	routeItems := make([]deploymentTargetDiagnosticRouteItem, 0, len(observedRoutes))
	for _, r := range observedRoutes {
		routeItems = append(routeItems, deploymentTargetDiagnosticRouteItem{
			ID:                r.ID,
			Host:              r.Host,
			Path:              r.Path,
			ServicePort:       r.ServicePort,
			TLSMode:           r.TLSMode,
			Enabled:           r.Enabled,
			Status:            r.Status,
			ObservationCode:   r.ObservationCode,
			CertificateStatus: r.CertificateStatus,
			AccessURL:         r.AccessURL,
		})
	}

	return runtimeGetTargetDiagnosticsOutput{
		ProjectID:          project.ID,
		ApplicationID:      app.ID,
		DeploymentTargetID: target.ID,
		Stage:              observedTarget.Stage,
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
		MetricsAvailable: metricsResp.Available,
		Metrics:          metricsResp,
		Pods:             podsSnapshot,
		Events:           eventsSnapshot,
		VolumeMounts:     volItems,
		RecentReleases:   releaseItems,
		AccessRoutes:     routeItems,
	}, nil
}

func (h *Handler) observeSingleFrameTargetMetrics(
	ctx context.Context,
	project model.Project,
	target model.DeploymentTarget,
) deploymentTargetMetricsResponse {
	now := time.Now().UTC()
	client, unavailableReason := h.deploymentTargetMetricsClient(target, ctx)
	if client == nil {
		return deploymentTargetMetricsResponse{
			Available:          false,
			Status:             observation.StatusUnavailable,
			Reason:             unavailableReason,
			ConfiguredReplicas: target.Replicas,
			UpdatedAt:          now,
		}
	}
	probeCtx, cancel := context.WithTimeout(ctx, deploymentTargetObservationTimeout)
	defer cancel()
	snapshot, err := client.RuntimeMetrics(probeCtx, kubeprovider.RuntimeMetricsOptions{
		Namespace:          deploymentTargetNamespace(project, target),
		DeploymentTargetID: target.ID,
		WorkloadName:       target.KubernetesName,
		WorkloadType:       normalizeWorkloadType(target.WorkloadType),
	})
	if err != nil {
		return deploymentTargetMetricsResponse{
			Available:          false,
			Status:             observation.StatusUnavailable,
			Reason:             "metrics_error",
			ConfiguredReplicas: target.Replicas,
			UpdatedAt:          now,
		}
	}
	return deploymentTargetMetricsResponseFromSnapshot(snapshot, target)
}

func (h *Handler) observeTargetDiagnosticVolumeMounts(
	ctx context.Context,
	db interface {
		Where(any, ...any) *gorm.DB
	},
	project model.Project,
	target model.DeploymentTarget,
) ([]aiDiagnosticVolumeMountItem, error) {
	mountsByTarget, err := h.deploymentTargetVolumeMountsByTarget(ctx, []model.DeploymentTarget{target})
	if err != nil {
		return nil, aitool.NewError(http.StatusInternalServerError, "database.query_failed", err.Error())
	}
	mounts := mountsByTarget[target.ID]
	if len(mounts) == 0 {
		return []aiDiagnosticVolumeMountItem{}, nil
	}

	var pvIDs []string
	for _, m := range mounts {
		if m.SourceType == model.DeploymentVolumeSourceProjectVolume && m.ProjectVolumeID != nil && strings.TrimSpace(*m.ProjectVolumeID) != "" {
			pvIDs = append(pvIDs, strings.TrimSpace(*m.ProjectVolumeID))
		}
	}

	volsByID := map[string]model.ProjectVolume{}
	var claimNames []string
	if len(pvIDs) > 0 {
		var vols []model.ProjectVolume
		if qErr := db.Where("project_id = ? AND id IN ?", project.ID, pvIDs).Find(&vols).Error; qErr == nil {
			for _, v := range vols {
				volsByID[v.ID] = v
				if strings.TrimSpace(v.ClaimName) != "" {
					claimNames = append(claimNames, strings.TrimSpace(v.ClaimName))
				}
			}
		}
	}

	var (
		pvcObsByClaim map[string]kubeprovider.ProjectVolumeClaimObservation
		pvcUnavail    string
	)
	if len(claimNames) > 0 {
		client, namespace, code := h.kubernetesClientForDeploymentTargetObservation(project, target, ctx)
		if client == nil {
			pvcUnavail = "volume.pvc_" + code
		} else {
			probeCtx, cancel := context.WithTimeout(ctx, deploymentTargetObservationTimeout)
			obsMap, obsErr := client.ObserveProjectVolumeClaims(probeCtx, namespace, project.ID, claimNames)
			cancel()
			if obsErr != nil {
				pvcUnavail = "volume.pvc_unavailable"
			} else {
				pvcObsByClaim = obsMap
			}
		}
	}

	items := make([]aiDiagnosticVolumeMountItem, 0, len(mounts))
	for _, m := range mounts {
		item := aiDiagnosticVolumeMountItem{
			BindingID:       m.ID,
			LogicalName:     m.LogicalName,
			MountPath:       optionalStringValue(m.MountPath),
			DevicePath:      optionalStringValue(m.DevicePath),
			ReadOnly:        m.ReadOnly,
			ActivationState: m.ActivationState,
		}
		if m.SourceType == model.DeploymentVolumeSourceEmptyDir {
			item.SourceType = "emptyDir"
			item.EmptyDir = &deploymentTargetEmptyDirInput{Medium: m.EmptyDirMedium, SizeLimit: m.EmptyDirSizeLimit}
			items = append(items, item)
			continue
		}
		item.SourceType = "projectVolume"
		pvID := optionalStringValue(m.ProjectVolumeID)
		item.ProjectVolumeID = pvID
		vol, hasVol := volsByID[pvID]
		if !hasVol {
			item.ObservationStatus = observation.StatusNotFound
			item.ObservationCode = "volume.not_found"
			items = append(items, item)
			continue
		}
		item.VolumeDisplayName = vol.DisplayName
		item.ClaimName = vol.ClaimName
		item.VolumeLifecycle = vol.LifecycleState
		item.RequestedCapacity = vol.CapacityRequest
		item.StorageClassName = vol.StorageClassName
		item.AccessMode = vol.AccessMode
		item.VolumeMode = vol.VolumeMode

		if pvcUnavail != "" {
			item.ObservationStatus = observation.StatusUnavailable
			item.ObservationCode = pvcUnavail
		} else if pvcObs, ok := pvcObsByClaim[strings.TrimSpace(vol.ClaimName)]; ok {
			exists := pvcObs.Exists
			item.PVCExists = &exists
			item.PVCPhase = pvcObs.Phase
			item.PVCObservedCapacity = pvcObs.Capacity
			if !pvcObs.Exists {
				item.ObservationStatus = observation.StatusNotFound
				item.ObservationCode = "volume.pvc_not_found"
			} else if strings.EqualFold(pvcObs.Phase, "Bound") {
				item.ObservationStatus = observation.StatusReady
				item.ObservationCode = "volume.pvc_bound"
			} else if strings.EqualFold(pvcObs.Phase, "Pending") {
				item.ObservationStatus = observation.StatusProgressing
				item.ObservationCode = "volume.pvc_pending"
			} else {
				item.ObservationStatus = observation.StatusDegraded
				item.ObservationCode = "volume.pvc_" + strings.ToLower(strings.TrimSpace(pvcObs.Phase))
			}
		} else if strings.TrimSpace(vol.ClaimName) == "" {
			item.ObservationStatus = observation.StatusNotConfigured
			item.ObservationCode = "volume.claim_not_configured"
		}
		items = append(items, item)
	}
	return items, nil
}

// ============================================================================
// Tool 16 Implementation: runtime_list_target_pods_and_events
// ============================================================================

func (h *Handler) executeRuntimeListTargetPodsAndEvents(
	tc *aitool.ToolContext,
	in runtimeListTargetPodsAndEventsInput,
) (runtimeListTargetPodsAndEventsOutput, error) {
	tc.SetNoStore()
	_, project, err := h.authorizeAIProject(tc, in.Project, authz.ActionDeploymentRead)
	if err != nil {
		return runtimeListTargetPodsAndEventsOutput{}, err
	}
	db := h.aiDB(tc)
	ctx := tc.Context()
	app, err := aitool.ResolveApplication(db, project.ID, in.Application)
	if err != nil {
		return runtimeListTargetPodsAndEventsOutput{}, err
	}
	target, err := aitool.ResolveDeploymentTarget(db, project.ID, app.ID, in.Target)
	if err != nil {
		return runtimeListTargetPodsAndEventsOutput{}, err
	}

	includePods := boolVal(in.IncludePods, true)
	includeEvents := boolVal(in.IncludeEvents, true)
	podLimit := intVal(in.PodLimit, 20)
	if podLimit < 1 {
		podLimit = 1
	} else if podLimit > 50 {
		podLimit = 50
	}
	eventLimit := intVal(in.EventLimit, 20)
	if eventLimit < 1 {
		eventLimit = 1
	} else if eventLimit > 50 {
		eventLimit = 50
	}

	if evType := strings.TrimSpace(in.EventType); evType != "" && evType != "Warning" && evType != "Normal" {
		return runtimeListTargetPodsAndEventsOutput{}, aitool.BadRequest("argument_invalid", "eventType must be Warning or Normal")
	}

	out := runtimeListTargetPodsAndEventsOutput{
		ProjectID:          project.ID,
		ApplicationID:      app.ID,
		DeploymentTargetID: target.ID,
		Stage:              target.Stage,
	}

	now := time.Now().UTC()
	client, namespace, code := h.kubernetesClientForDeploymentTargetObservation(project, target, ctx)
	if code != "" {
		if includePods {
			out.Pods = &aiPodsSectionOutput{
				Status:          observation.StatusUnavailable,
				ObservationCode: "deployment_target.runtime_pods_" + code,
				ObservedAt:      now,
				Items:           []kubeprovider.RuntimePodSnapshot{},
			}
		}
		if includeEvents {
			out.Events = &aiEventsSectionOutput{
				Status:          observation.StatusUnavailable,
				ObservationCode: "deployment_target.events_" + code,
				ObservedAt:      now,
				Items:           []kubeprovider.DeploymentTargetEventSnapshot{},
			}
		}
		return out, nil
	}

	relFilter := strings.TrimSpace(in.ReleaseID)
	if includePods {
		probeCtx, cancel := context.WithTimeout(ctx, deploymentTargetObservationTimeout)
		pods, pErr := client.RuntimePods(probeCtx, namespace, target.ID, relFilter)
		cancel()
		obsAt := time.Now().UTC()
		if pErr != nil {
			out.Pods = &aiPodsSectionOutput{
				Status:          observation.StatusUnavailable,
				ObservationCode: "deployment_target.runtime_pods_unavailable",
				ObservedAt:      obsAt,
				Items:           []kubeprovider.RuntimePodSnapshot{},
			}
		} else {
			if relFilter != "" {
				pods = slices.DeleteFunc(pods, func(p kubeprovider.RuntimePodSnapshot) bool {
					return strings.TrimSpace(p.ReleaseID) != relFilter
				})
			}
			pods = filterRuntimePods(pods, in.Search)
			totalPods := len(pods)
			if len(pods) > podLimit {
				pods = pods[:podLimit]
			}
			if pods == nil {
				pods = []kubeprovider.RuntimePodSnapshot{}
			}
			out.Pods = &aiPodsSectionOutput{
				Status:          observation.StatusReady,
				ObservationCode: "deployment_target.runtime_pods_ready",
				ObservedAt:      obsAt,
				Total:           totalPods,
				Items:           pods,
			}
		}
	}

	if includeEvents {
		probeCtx, cancel := context.WithTimeout(ctx, deploymentTargetObservationTimeout)
		events, eErr := client.ListDeploymentTargetEvents(
			probeCtx,
			namespace,
			target.ID,
			target.KubernetesName,
			normalizeWorkloadType(target.WorkloadType),
		)
		cancel()
		obsAt := time.Now().UTC()
		if eErr != nil {
			out.Events = &aiEventsSectionOutput{
				Status:          observation.StatusUnavailable,
				ObservationCode: "deployment_target.events_unavailable",
				ObservedAt:      obsAt,
				Items:           []kubeprovider.DeploymentTargetEventSnapshot{},
			}
		} else {
			events = filterDeploymentTargetEvents(events, in.EventType, in.Search)
			totalEvents := len(events)
			if len(events) > eventLimit {
				events = events[:eventLimit]
			}
			if events == nil {
				events = []kubeprovider.DeploymentTargetEventSnapshot{}
			}
			out.Events = &aiEventsSectionOutput{
				Status:          observation.StatusReady,
				ObservationCode: "deployment_target.events_ready",
				ObservedAt:      obsAt,
				Total:           totalEvents,
				Items:           events,
			}
		}
	}

	return out, nil
}

// ============================================================================
// Tool 17 Implementation: runtime_get_pod_logs
// ============================================================================

func (h *Handler) executeRuntimeGetPodLogs(
	tc *aitool.ToolContext,
	in runtimeGetPodLogsInput,
) (runtimeGetPodLogsOutput, error) {
	tc.SetNoStore()
	_, project, err := h.authorizeAIProject(tc, in.Project, authz.ActionDeploymentRead)
	if err != nil {
		return runtimeGetPodLogsOutput{}, err
	}
	db := h.aiDB(tc)

	targetRef := strings.TrimSpace(in.Target)
	appRef := strings.TrimSpace(in.Application)
	releaseID := strings.TrimSpace(in.ReleaseID)
	if targetRef == "" && releaseID == "" {
		return runtimeGetPodLogsOutput{}, aitool.BadRequest("argument_invalid", "at least one of target (with application) or releaseId must be provided")
	}

	var (
		app    model.Application
		target model.DeploymentTarget
	)
	if targetRef != "" {
		if appRef == "" {
			return runtimeGetPodLogsOutput{}, aitool.BadRequest("argument_invalid", "application is required when target is provided")
		}
		var rErr error
		app, rErr = aitool.ResolveApplication(db, project.ID, appRef)
		if rErr != nil {
			return runtimeGetPodLogsOutput{}, rErr
		}
		target, rErr = aitool.ResolveDeploymentTarget(db, project.ID, app.ID, targetRef)
		if rErr != nil {
			return runtimeGetPodLogsOutput{}, rErr
		}
		if releaseID != "" {
			var rel model.Release
			if err := db.First(&rel, "id = ? AND project_id = ?", releaseID, project.ID).Error; err != nil {
				return runtimeGetPodLogsOutput{}, aitool.NotFound("release.not_found", "releaseId was not found")
			}
			if rel.ApplicationID != app.ID || rel.DeploymentTargetID != target.ID {
				return runtimeGetPodLogsOutput{}, aitool.BadRequest("argument_invalid", "releaseId does not belong to the specified deployment target")
			}
		}
	} else {
		var rel model.Release
		if err := db.First(&rel, "id = ? AND project_id = ?", releaseID, project.ID).Error; err != nil {
			return runtimeGetPodLogsOutput{}, aitool.NotFound("release.not_found", "releaseId was not found")
		}
		if err := db.First(&app, "id = ? AND project_id = ? AND deleted_at IS NULL", rel.ApplicationID, project.ID).Error; err != nil {
			return runtimeGetPodLogsOutput{}, aitool.NotFound("application.not_found", "application was not found")
		}
		if err := db.First(&target, "id = ? AND project_id = ? AND application_id = ? AND deleted_at IS NULL", rel.DeploymentTargetID, project.ID, app.ID).Error; err != nil {
			return runtimeGetPodLogsOutput{}, aitool.NotFound("deployment_target.not_found", "deployment target was not found")
		}
	}

	tailLines := intVal(in.TailLines, 200)
	if tailLines < 1 {
		tailLines = 1
	} else if tailLines > 1000 {
		tailLines = 1000
	}
	maxBytes := intVal(in.MaxBytes, 32768)
	if maxBytes < 256 {
		maxBytes = 256
	} else if maxBytes > 65536 {
		maxBytes = 65536
	}

	scratchCtx, rec := newScratchGin(tc)
	client, namespace, _, ok := h.runtimeClientForDeploymentTarget(scratchCtx, project, target)
	if !ok {
		return runtimeGetPodLogsOutput{}, toolErrorFromRecorder(rec, http.StatusBadRequest, "runtime_cluster.unavailable", "runtime cluster is unavailable")
	}

	reqCtx, cancel := context.WithTimeout(tc.Context(), 12*time.Second)
	defer cancel()
	result, logErr := client.RuntimePodLogs(reqCtx, kubeprovider.RuntimePodLogsOptions{
		RuntimePodSelection: kubeprovider.RuntimePodSelection{
			Namespace:          namespace,
			DeploymentTargetID: target.ID,
			Pod:                strings.TrimSpace(in.Pod),
			PreferredReleaseID: releaseID,
			PreferAbnormal:     boolVal(in.PreferAbnormal, true),
			Container:          strings.TrimSpace(in.Container),
		},
		TailLines:  int64(tailLines),
		Previous:   boolVal(in.Previous, false),
		Timestamps: boolVal(in.Timestamps, true),
	})
	if errors.Is(logErr, kubeprovider.ErrRuntimePodNotFound) {
		return runtimeGetPodLogsOutput{}, aitool.NewErrorWithHint(
			http.StatusNotFound,
			"runtime.pod_not_found",
			"runtime pod not found for this deployment target",
			"The Pod may have been replaced by a rolling update, scaled down, or evicted. Do not retry the same `pod` name; omit `pod` in `runtime_get_pod_logs` or call `runtime_get_target_diagnostics` to re-align with current Pods.",
		)
	}
	if errors.Is(logErr, kubeprovider.ErrRuntimeContainerNotStarted) {
		return runtimeGetPodLogsOutput{}, aitool.NewErrorWithHint(
			http.StatusConflict,
			"runtime.container_not_started",
			"runtime container has not started; no logs are available yet",
			"Do not retry logs on the same unstarted container. Call `runtime_get_target_diagnostics` to inspect waiting reasons/events; if an initContainer failed (`Init:*`), pass its name in `container` to read its logs.",
		)
	}
	if errors.Is(logErr, kubeprovider.ErrRuntimePreviousLogNotFound) {
		return runtimeGetPodLogsOutput{}, aitool.NewErrorWithHint(
			http.StatusConflict,
			"runtime.previous_log_not_found",
			"runtime container has no previous terminated logs",
			"Do not retry with `previous: true`. Retry once with `previous: false` (or omit `previous`) to read current logs, or call `runtime_get_target_diagnostics` to inspect container waiting events.",
		)
	}
	if logErr != nil {
		return runtimeGetPodLogsOutput{}, aitool.NewError(http.StatusBadGateway, "runtime.logs_failed", logErr.Error())
	}

	content, truncated, truncatedBytes := truncateUTF8Tail(result.Content, maxBytes)
	return runtimeGetPodLogsOutput{
		ProjectID:          project.ID,
		ApplicationID:      app.ID,
		DeploymentTargetID: target.ID,
		Stage:              target.Stage,
		ReleaseID:          releaseID,
		Pod:                result.Pod,
		Container:          result.Container,
		Previous:           result.Previous,
		Content:            content,
		Truncated:          truncated,
		TruncatedBytes:     truncatedBytes,
	}, nil
}

// ============================================================================
// Tool 18 Implementation: runtime_exec_command
// ============================================================================

type preparedRuntimeExecPlan struct {
	user      model.User
	project   model.Project
	app       model.Application
	target    model.DeploymentTarget
	release   model.Release
	command   string
	client    *kubeprovider.Client
	namespace string
}

func (h *Handler) prepareRuntimeExecPlan(tc *aitool.ToolContext, in runtimeExecCommandInput) (preparedRuntimeExecPlan, error) {
	command := strings.TrimSpace(in.Command)
	if command == "" {
		return preparedRuntimeExecPlan{}, aitool.BadRequest("argument_invalid", "command is required")
	}
	if len(command) > 2000 {
		return preparedRuntimeExecPlan{}, aitool.BadRequest("argument_invalid", "command must not exceed 2000 characters")
	}

	user, project, err := h.authorizeAIProject(tc, in.Project, authz.ActionDeploymentExec)
	if err != nil {
		return preparedRuntimeExecPlan{}, err
	}
	if err := h.ensureAIProjectCanMutate(tc, project); err != nil {
		return preparedRuntimeExecPlan{}, err
	}
	db := h.aiDB(tc)

	var (
		app     model.Application
		target  model.DeploymentTarget
		release model.Release
	)
	if explicitRel := strings.TrimSpace(in.ReleaseID); explicitRel != "" {
		if err := db.First(&release, "id = ? AND project_id = ?", explicitRel, project.ID).Error; err != nil {
			return preparedRuntimeExecPlan{}, aitool.NotFound("release.not_found", "releaseId was not found")
		}
		if err := db.First(&app, "id = ? AND project_id = ? AND deleted_at IS NULL", release.ApplicationID, project.ID).Error; err != nil {
			return preparedRuntimeExecPlan{}, aitool.NotFound("application.not_found", "application was not found")
		}
		if err := db.First(&target, "id = ? AND project_id = ? AND application_id = ? AND deleted_at IS NULL", release.DeploymentTargetID, project.ID, app.ID).Error; err != nil {
			return preparedRuntimeExecPlan{}, aitool.NotFound("deployment_target.not_found", "deployment target was not found")
		}
		if strings.TrimSpace(in.Application) != "" {
			resolvedApp, aErr := aitool.ResolveApplication(db, project.ID, in.Application)
			if aErr != nil || resolvedApp.ID != app.ID {
				return preparedRuntimeExecPlan{}, aitool.BadRequest("argument_invalid", "application does not match releaseId")
			}
		}
		if strings.TrimSpace(in.Target) != "" {
			resolvedTarget, tErr := aitool.ResolveDeploymentTarget(db, project.ID, app.ID, in.Target)
			if tErr != nil || resolvedTarget.ID != target.ID {
				return preparedRuntimeExecPlan{}, aitool.BadRequest("argument_invalid", "target does not match releaseId")
			}
		}
	} else {
		if strings.TrimSpace(in.Application) == "" || strings.TrimSpace(in.Target) == "" {
			return preparedRuntimeExecPlan{}, aitool.BadRequest("argument_invalid", "application and target are required when releaseId is omitted")
		}
		var rErr error
		app, rErr = aitool.ResolveApplication(db, project.ID, in.Application)
		if rErr != nil {
			return preparedRuntimeExecPlan{}, rErr
		}
		target, rErr = aitool.ResolveDeploymentTarget(db, project.ID, app.ID, in.Target)
		if rErr != nil {
			return preparedRuntimeExecPlan{}, rErr
		}
		// Prefer latest succeeded Release; fall back to latest running Release; otherwise 409 deployment_target.not_deployed!
		if sErr := db.Where("project_id = ? AND application_id = ? AND deployment_target_id = ? AND status = ?",
			project.ID, app.ID, target.ID, "succeeded").
			Order("revision desc, created_at desc").
			First(&release).Error; sErr != nil {
			if rErr := db.Where("project_id = ? AND application_id = ? AND deployment_target_id = ? AND status = ?",
				project.ID, app.ID, target.ID, "running").
				Order("revision desc, created_at desc").
				First(&release).Error; rErr != nil {
				return preparedRuntimeExecPlan{}, aitool.NewErrorWithHint(
					http.StatusConflict,
					"deployment_target.not_deployed",
					"deployment target has no succeeded or running release to execute commands against",
					"Deploy the target first via `delivery_deploy_target`.",
				)
			}
		}
	}

	if err := ensureAIApplicationCanMutate(app); err != nil {
		return preparedRuntimeExecPlan{}, err
	}
	if err := h.ensureAIDeploymentTargetCanMutate(tc, target); err != nil {
		return preparedRuntimeExecPlan{}, err
	}
	if !runtimeWebConsoleEnabled(project, target) {
		return preparedRuntimeExecPlan{}, aitool.NewErrorWithHint(
			http.StatusForbidden,
			"runtime.web_console_disabled",
			"web console / container exec is disabled by project or target governance policy",
			"Do not retry `runtime_exec_command`. Troubleshoot via `runtime_get_target_diagnostics` and `runtime_get_pod_logs`, or ask a project owner/admin to enable Web Console policy first.",
		)
	}

	scratchCtx, rec := newScratchGin(tc)
	client, namespace, _, ok := h.runtimeClientForDeploymentTarget(scratchCtx, project, target)
	if !ok {
		return preparedRuntimeExecPlan{}, toolErrorFromRecorder(rec, http.StatusBadRequest, "runtime_cluster.unavailable", "runtime cluster is unavailable")
	}

	return preparedRuntimeExecPlan{
		user:      user,
		project:   project,
		app:       app,
		target:    target,
		release:   release,
		command:   command,
		client:    client,
		namespace: namespace,
	}, nil
}

func (h *Handler) previewRuntimeExecCommand(tc *aitool.ToolContext, in runtimeExecCommandInput) (*aitool.PreviewResult, error) {
	plan, err := h.prepareRuntimeExecPlan(tc, in)
	if err != nil {
		return nil, err
	}
	podDesc := fallback(strings.TrimSpace(in.Pod), "(auto-select running pod)")
	containerDesc := fallback(strings.TrimSpace(in.Container), "(default container)")
	return &aitool.PreviewResult{
		Preview: aitool.ApprovalPreview{
			Summary:        fmt.Sprintf("在 %s/%s 容器中执行诊断命令", plan.app.Identifier, plan.target.Stage),
			TargetResource: fmt.Sprintf("%s/%s", plan.app.Identifier, plan.target.Stage),
			TargetID:       plan.target.ID,
			ProjectID:      plan.project.ID,
			Changes: []aitool.PreviewChange{
				{Field: "command", After: plan.command, Action: "exec"},
				{Field: "pod", After: podDesc, Action: "exec"},
				{Field: "container", After: containerDesc, Action: "exec"},
				{Field: "releaseId", After: plan.release.ID, Action: "exec"},
			},
			Warnings: []string{"Executes `/bin/sh -lc` inside the live container (30s timeout; distroless/scratch images without `/bin/sh` are unsupported)."},
		},
	}, nil
}

func (h *Handler) executeRuntimeExecCommand(tc *aitool.ToolContext, in runtimeExecCommandInput) (runtimeExecCommandOutput, error) {
	tc.SetNoStore()
	plan, err := h.prepareRuntimeExecPlan(tc, in)
	if err != nil {
		return runtimeExecCommandOutput{}, err
	}
	ctx := tc.Context()
	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	result, execErr := plan.client.RuntimeExec(reqCtx, kubeprovider.RuntimeExecOptions{
		RuntimePodSelection: kubeprovider.RuntimePodSelection{
			Namespace:          plan.namespace,
			DeploymentTargetID: plan.target.ID,
			Pod:                strings.TrimSpace(in.Pod),
			PreferredReleaseID: plan.release.ID,
			Container:          strings.TrimSpace(in.Container),
		},
		Command: plan.command,
	})
	if errors.Is(execErr, kubeprovider.ErrRuntimePodNotFound) {
		h.auditWithContext(plan.user.ID, "release_runtime.exec", plan.release.ID, false, execErr.Error(), ctx)
		return runtimeExecCommandOutput{}, aitool.NewErrorWithHint(
			http.StatusNotFound,
			"runtime.pod_not_found",
			"runtime pod not found for this deployment target",
			"The target Pod may have been replaced by a rolling update or scaled to zero. Do not retry the stale `pod` name; call `runtime_get_target_diagnostics` to check live Pods or omit `pod` in a fresh preview.",
		)
	}
	if execErr != nil {
		h.auditWithContext(plan.user.ID, "release_runtime.exec", plan.release.ID, false, execErr.Error(), ctx)
		return runtimeExecCommandOutput{}, aitool.NewError(http.StatusBadGateway, "runtime.exec_failed", execErr.Error())
	}

	h.auditWithContext(plan.user.ID, "release_runtime.exec", plan.release.ID, result.ExitCode == 0, runtimeExecAuditMessage(plan.command, result), ctx)

	stdout, outTrunc, _ := truncateUTF8Head(result.Stdout, aiRuntimeExecStreamCapBytes)
	stderr, errTrunc, _ := truncateUTF8Head(result.Stderr, aiRuntimeExecStreamCapBytes)

	return runtimeExecCommandOutput{
		ProjectID:          plan.project.ID,
		ApplicationID:      plan.app.ID,
		DeploymentTargetID: plan.target.ID,
		Stage:              plan.target.Stage,
		ReleaseID:          plan.release.ID,
		Pod:                result.Pod,
		Container:          result.Container,
		Stdout:             stdout,
		Stderr:             stderr,
		ExitCode:           result.ExitCode,
		Truncated:          result.Truncated || outTrunc || errTrunc,
		DurationMs:         result.Duration,
	}, nil
}
