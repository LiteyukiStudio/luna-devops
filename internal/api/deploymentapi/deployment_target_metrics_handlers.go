package deploymentapi

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/observation"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/runtimecluster"
	"github.com/gin-gonic/gin"
)

func (h *Handlers) StreamDeploymentTargetMetrics(ctx *gin.Context) {
	user, project, ok := h.authorizeProject(ctx, authz.ActionDeploymentRead)
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
	binding, ok := h.requireContinuousAuthorizationBinding(ctx, user)
	if !ok {
		return
	}
	streamCtx, cancelStream := context.WithCancel(ctx.Request.Context())
	defer cancelStream()
	restoreRequestContext := replaceRequestContext(ctx, streamCtx)
	defer restoreRequestContext()
	reference := deploymentMetricsAuthorizationReference{
		ProjectID: project.ID, ApplicationID: app.ID, TargetID: target.ID,
		ClusterID: target.ClusterID, Namespace: project.KubernetesNamespace, KubernetesName: target.KubernetesName,
	}
	authorizationRevoked, authorizationActive := h.monitorContinuousAuthorization(
		streamCtx,
		binding,
		func(checkCtx context.Context, currentUser model.User) bool {
			return h.deploymentMetricsAuthorizationAllowed(checkCtx, currentUser, reference)
		},
		cancelStream,
	)
	if !authorizationActive {
		writeContinuousAuthorizationRevoked(ctx)
		return
	}
	client, unavailableReason := h.deploymentTargetMetricsClient(target, ctx.Request.Context())

	writer := ctx.Writer
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.Header().Set("Cache-Control", "no-store, no-transform")
	writer.Header().Set("Pragma", "no-cache")
	writer.Header().Set("Connection", "keep-alive")
	writer.Header().Set("X-Accel-Buffering", "no")
	writer.WriteHeader(http.StatusOK)
	flushSSE(writer)

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	sequence := 0
	for {
		select {
		case <-authorizationRevoked:
			return
		case <-streamCtx.Done():
			return
		default:
		}
		sequence++
		h.writeDeploymentTargetMetricsEvent(ctx, client, unavailableReason, project, target, sequence)
		flushSSE(writer)
		select {
		case <-streamCtx.Done():
			return
		case <-ticker.C:
		}
	}
}

type deploymentMetricsAuthorizationReference struct {
	ProjectID      string
	ApplicationID  string
	TargetID       string
	ClusterID      string
	Namespace      string
	KubernetesName string
}

func (h *Handlers) deploymentMetricsAuthorizationAllowed(ctx context.Context, user model.User, reference deploymentMetricsAuthorizationReference) bool {
	if !h.projectContinuousAuthorizationAllowed(ctx, user, reference.ProjectID, authz.ActionDeploymentRead) {
		return false
	}
	db := h.dbWithContext(ctx)
	if db == nil {
		return false
	}
	var application model.Application
	if err := db.Select("id").First(&application, "id = ? and project_id = ?", reference.ApplicationID, reference.ProjectID).Error; err != nil {
		return false
	}
	var project model.Project
	if err := db.Select("id", "kubernetes_namespace").First(&project, "id = ?", reference.ProjectID).Error; err != nil || strings.TrimSpace(project.KubernetesNamespace) != strings.TrimSpace(reference.Namespace) {
		return false
	}
	var target model.DeploymentTarget
	if err := db.First(&target, "id = ? and project_id = ? and application_id = ?", reference.TargetID, reference.ProjectID, reference.ApplicationID).Error; err != nil {
		return false
	}
	return h.host.ResourceCanMutateDuringDelete(target.DeleteStatus) &&
		target.ClusterID == reference.ClusterID &&
		target.KubernetesName == reference.KubernetesName
}

func (h *Handlers) writeDeploymentTargetMetricsEvent(ctx *gin.Context, client *kubeprovider.Client, unavailableReason string, project model.Project, target model.DeploymentTarget, sequence int) {
	if ctx.Request.Context().Err() != nil {
		return
	}
	if client == nil {
		writeSSE(ctx.Writer, "metrics", strconv.Itoa(sequence), deploymentTargetMetricsResponse{
			Available: false,
			Status:    observation.StatusUnavailable,
			Reason:    unavailableReason,
			UpdatedAt: time.Now(),
		})
		return
	}
	requestCtx, cancel := context.WithTimeout(ctx.Request.Context(), 8*time.Second)
	defer cancel()
	snapshot, err := client.RuntimeMetrics(requestCtx, kubeprovider.RuntimeMetricsOptions{
		Namespace:          deploymentTargetNamespace(project, target),
		DeploymentTargetID: target.ID,
		WorkloadName:       target.KubernetesName,
		WorkloadType:       normalizeWorkloadType(target.WorkloadType),
	})
	if ctx.Request.Context().Err() != nil {
		return
	}
	if err != nil {
		writeSSE(ctx.Writer, "metrics", "", deploymentTargetMetricsResponse{
			Available: false,
			Status:    observation.StatusUnavailable,
			Reason:    "metrics_error",
			UpdatedAt: time.Now(),
		})
		return
	}
	response := deploymentTargetMetricsResponseFromSnapshot(snapshot, target)
	writeSSE(ctx.Writer, "metrics", strconv.Itoa(sequence), response)
}

func (h *Handlers) deploymentTargetMetricsClient(target model.DeploymentTarget, ctx context.Context) (*kubeprovider.Client, string) {
	cluster, err := runtimecluster.ResolveForTarget(h.dbWithContext(ctx), target.ClusterID)
	if err != nil {
		return nil, "cluster_unavailable"
	}
	kubeconfig := h.secrets.ResolveContext(ctx, cluster.KubeconfigRef)
	if strings.TrimSpace(kubeconfig) == "" {
		return nil, "cluster_unavailable"
	}
	client, err := kubeprovider.NewClientFromKubeconfig(kubeconfig)
	if err != nil {
		return nil, "cluster_unavailable"
	}
	return client, ""
}

// deploymentTargetMetricsResponse compares like with like: usage, request
// floor and limit all cover every regular container of every replica. The
// request and limit come from the workload Pod template observed together with
// the usage, so they reflect what was rendered rather than current settings.
type deploymentTargetMetricsResponse struct {
	Available          bool   `json:"available"`
	Status             string `json:"status"`
	Reason             string `json:"reason,omitempty"`
	ConfiguredReplicas int    `json:"configuredReplicas"`
	DesiredReplicas    int32  `json:"desiredReplicas"`
	ReadyReplicas      int32  `json:"readyReplicas"`
	AvailableReplicas  int32  `json:"availableReplicas"`
	PodCount           int    `json:"podCount"`
	ContainerCount     int    `json:"containerCount"`
	// Aggregates across all replicas.
	CPUUsageMilli      int64 `json:"cpuUsageMilli"`
	CPURequestMilli    int64 `json:"cpuRequestMilli"`
	CPULimitMilli      int64 `json:"cpuLimitMilli"`
	MemoryUsageBytes   int64 `json:"memoryUsageBytes"`
	MemoryRequestBytes int64 `json:"memoryRequestBytes"`
	MemoryLimitBytes   int64 `json:"memoryLimitBytes"`
	// Percentages are usage divided by the aggregate limit; omitted when the
	// workload has no bounded limit for that resource.
	CPUUsagePercent    *float64 `json:"cpuUsagePercent,omitempty"`
	MemoryUsagePercent *float64 `json:"memoryUsagePercent,omitempty"`
	// Per-replica Pod template envelope.
	ReplicaCPURequestMilli    int64     `json:"replicaCpuRequestMilli"`
	ReplicaCPULimitMilli      int64     `json:"replicaCpuLimitMilli"`
	ReplicaMemoryRequestBytes int64     `json:"replicaMemoryRequestBytes"`
	ReplicaMemoryLimitBytes   int64     `json:"replicaMemoryLimitBytes"`
	UpdatedAt                 time.Time `json:"updatedAt"`
}

func deploymentTargetMetricsResponseFromSnapshot(snapshot kubeprovider.RuntimeMetricsSnapshot, target model.DeploymentTarget) deploymentTargetMetricsResponse {
	replicas := int64(snapshot.DesiredReplicas)
	resources := snapshot.Resources
	cpuLimit := resources.CPULimitMilli * replicas
	memoryLimit := resources.MemoryLimitBytes * replicas
	return deploymentTargetMetricsResponse{
		Available:                 snapshot.Available,
		Status:                    deploymentTargetMetricsStatus(snapshot.Available),
		Reason:                    snapshot.Reason,
		ConfiguredReplicas:        target.Replicas,
		DesiredReplicas:           snapshot.DesiredReplicas,
		ReadyReplicas:             snapshot.ReadyReplicas,
		AvailableReplicas:         snapshot.AvailableReplicas,
		PodCount:                  snapshot.PodCount,
		ContainerCount:            snapshot.ContainerCount,
		CPUUsageMilli:             snapshot.CPUUsageMilli,
		CPURequestMilli:           resources.CPURequestMilli * replicas,
		CPULimitMilli:             cpuLimit,
		CPUUsagePercent:           usagePercent(snapshot.CPUUsageMilli, cpuLimit),
		MemoryUsageBytes:          snapshot.MemoryUsageBytes,
		MemoryRequestBytes:        resources.MemoryRequestBytes * replicas,
		MemoryLimitBytes:          memoryLimit,
		MemoryUsagePercent:        usagePercent(snapshot.MemoryUsageBytes, memoryLimit),
		ReplicaCPURequestMilli:    resources.CPURequestMilli,
		ReplicaCPULimitMilli:      resources.CPULimitMilli,
		ReplicaMemoryRequestBytes: resources.MemoryRequestBytes,
		ReplicaMemoryLimitBytes:   resources.MemoryLimitBytes,
		UpdatedAt:                 snapshot.UpdatedAt,
	}
}

func deploymentTargetMetricsStatus(available bool) string {
	if available {
		return observation.StatusReady
	}
	return observation.StatusUnavailable
}

func usagePercent(usage int64, limit int64) *float64 {
	if limit <= 0 {
		return nil
	}
	percent := float64(max(usage, 0)) / float64(limit) * 100
	return &percent
}
