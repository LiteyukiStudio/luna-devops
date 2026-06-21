package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/gin-gonic/gin"
	"k8s.io/apimachinery/pkg/api/resource"
)

func (h *Handlers) StreamDeploymentTargetMetrics(ctx *gin.Context) {
	project, ok := h.findProjectForCurrentUser(ctx)
	if !ok {
		return
	}
	app, ok := h.findApplication(ctx)
	if !ok {
		return
	}
	var target model.DeploymentTarget
	if err := h.db.First(&target, "id = ? and project_id = ? and application_id = ?", ctx.Param("targetId"), project.ID, app.ID).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "deployment target not found")
		return
	}
	client, unavailableReason := h.deploymentTargetMetricsClient(target)

	writer := ctx.Writer
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.Header().Set("Cache-Control", "no-cache")
	writer.Header().Set("Connection", "keep-alive")
	writer.Header().Set("X-Accel-Buffering", "no")
	writer.WriteHeader(http.StatusOK)
	flushSSE(writer)

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	sequence := 0
	for {
		sequence++
		h.writeDeploymentTargetMetricsEvent(ctx, client, unavailableReason, project, target, sequence)
		flushSSE(writer)
		select {
		case <-ctx.Request.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

func (h *Handlers) writeDeploymentTargetMetricsEvent(ctx *gin.Context, client *kubeprovider.Client, unavailableReason string, project model.Project, target model.DeploymentTarget, sequence int) {
	if client == nil {
		writeSSE(ctx.Writer, "metrics", strconv.Itoa(sequence), deploymentTargetMetricsResponse{
			Available: false,
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
	})
	if err != nil {
		writeSSE(ctx.Writer, "metrics", "", deploymentTargetMetricsResponse{
			Available: false,
			Reason:    "metrics_error",
			UpdatedAt: time.Now(),
		})
		return
	}
	response := deploymentTargetMetricsResponseFromSnapshot(snapshot, target)
	writeSSE(ctx.Writer, "metrics", strconv.Itoa(sequence), response)
}

func (h *Handlers) deploymentTargetMetricsClient(target model.DeploymentTarget) (*kubeprovider.Client, string) {
	var cluster model.RuntimeCluster
	var err error
	if clusterID := strings.TrimSpace(target.ClusterID); clusterID != "" {
		err = h.db.First(&cluster, "id = ? and type in ?", clusterID, []string{"kubernetes", "k3s"}).Error
	} else {
		err = h.db.Where("scope = ? and type in ?", "global", []string{"kubernetes", "k3s"}).Order("is_default desc, created_at asc").First(&cluster).Error
	}
	if err != nil {
		return nil, "cluster_unavailable"
	}
	kubeconfig := h.secrets.Resolve(cluster.KubeconfigRef)
	if strings.TrimSpace(kubeconfig) == "" {
		return nil, "cluster_unavailable"
	}
	client, err := kubeprovider.NewClientFromKubeconfig(kubeconfig)
	if err != nil {
		return nil, "cluster_unavailable"
	}
	return client, ""
}

type deploymentTargetMetricsResponse struct {
	Available           bool      `json:"available"`
	Reason              string    `json:"reason,omitempty"`
	PodCount            int       `json:"podCount"`
	ContainerCount      int       `json:"containerCount"`
	CPUUsageMilli       int64     `json:"cpuUsageMilli"`
	CPUCapacityMilli    int64     `json:"cpuCapacityMilli"`
	CPUUsagePercent     float64   `json:"cpuUsagePercent"`
	MemoryUsageBytes    int64     `json:"memoryUsageBytes"`
	MemoryCapacityBytes int64     `json:"memoryCapacityBytes"`
	MemoryUsagePercent  float64   `json:"memoryUsagePercent"`
	UpdatedAt           time.Time `json:"updatedAt"`
}

func deploymentTargetMetricsResponseFromSnapshot(snapshot kubeprovider.RuntimeMetricsSnapshot, target model.DeploymentTarget) deploymentTargetMetricsResponse {
	replicas := target.Replicas
	if replicas <= 0 {
		replicas = 1
	}
	cpuCapacityMilli := quantityMilliValue(target.CPURequest) * int64(replicas)
	memoryCapacityBytes := quantityValue(target.MemoryRequest) * int64(replicas)
	return deploymentTargetMetricsResponse{
		Available:           snapshot.Available,
		Reason:              snapshot.Reason,
		PodCount:            snapshot.PodCount,
		ContainerCount:      snapshot.ContainerCount,
		CPUUsageMilli:       snapshot.CPUUsageMilli,
		CPUCapacityMilli:    cpuCapacityMilli,
		CPUUsagePercent:     usagePercent(snapshot.CPUUsageMilli, cpuCapacityMilli),
		MemoryUsageBytes:    snapshot.MemoryUsageBytes,
		MemoryCapacityBytes: memoryCapacityBytes,
		MemoryUsagePercent:  usagePercent(snapshot.MemoryUsageBytes, memoryCapacityBytes),
		UpdatedAt:           snapshot.UpdatedAt,
	}
}

func quantityMilliValue(value string) int64 {
	quantity, err := resource.ParseQuantity(strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return quantity.MilliValue()
}

func quantityValue(value string) int64 {
	quantity, err := resource.ParseQuantity(strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return quantity.Value()
}

func usagePercent(usage int64, capacity int64) float64 {
	if usage <= 0 || capacity <= 0 {
		return 0
	}
	return float64(usage) / float64(capacity) * 100
}
