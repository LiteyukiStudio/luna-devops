package runtimeapi

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/observation"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/runtimecluster"
	"github.com/gin-gonic/gin"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

const maxRuntimeClusterPressureBatch = 100

type runtimeClusterPressureResource struct {
	Requests       int64    `json:"requests"`
	Allocatable    int64    `json:"allocatable"`
	Usage          *int64   `json:"usage,omitempty"`
	RequestPercent float64  `json:"requestPercent"`
	UsagePercent   *float64 `json:"usagePercent,omitempty"`
}

type runtimeClusterPressureDetails struct {
	CPU              runtimeClusterPressureResource `json:"cpu"`
	Memory           runtimeClusterPressureResource `json:"memory"`
	NodeCount        int                            `json:"nodeCount"`
	PodCount         int                            `json:"podCount"`
	MetricsAvailable bool                           `json:"metricsAvailable"`
}

type runtimeClusterPressureResponse struct {
	ClusterID       string                         `json:"clusterId"`
	Status          string                         `json:"status"`
	PressureLevel   string                         `json:"pressureLevel"`
	PressureScore   *float64                       `json:"pressureScore,omitempty"`
	ObservationCode string                         `json:"observationCode,omitempty"`
	ObservedAt      time.Time                      `json:"observedAt"`
	Details         *runtimeClusterPressureDetails `json:"details,omitempty"`
}

type runtimeClusterPressureListResponse struct {
	Items []runtimeClusterPressureResponse `json:"items"`
}

func (h *Handlers) ObserveRuntimeClusterPressure(ctx *gin.Context) {
	markLiveObservationResponse(ctx)
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	clusterIDs, valid := runtimeClusterPressureIDs(ctx.QueryArray("clusterId"))
	if !valid {
		writeErrorCode(ctx, http.StatusBadRequest, "runtime_cluster.pressure_cluster_ids_invalid", "clusterId must contain between 1 and 100 unique runtime cluster identifiers")
		return
	}

	query := runtimecluster.ActiveScope(h.dbFor(ctx)).Model(&model.RuntimeCluster{}).Where("id IN ?", clusterIDs)
	query, visible := h.applyScopedResourceVisibility(ctx, query, scopedResourceRuntimeCluster, user, strings.TrimSpace(ctx.Query("projectId")))
	if !visible {
		return
	}
	var clusters []model.RuntimeCluster
	if err := query.Find(&clusters).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	byID := make(map[string]model.RuntimeCluster, len(clusters))
	for _, cluster := range clusters {
		byID[cluster.ID] = cluster
	}
	ordered := make([]model.RuntimeCluster, 0, len(clusters))
	for _, clusterID := range clusterIDs {
		if cluster, exists := byID[clusterID]; exists {
			ordered = append(ordered, cluster)
		}
	}

	detailed := authz.IsPlatformAdmin(user.Role)
	items := h.observeRuntimeClusterPressure(ctx.Request.Context(), ordered, detailed)
	ctx.JSON(http.StatusOK, runtimeClusterPressureListResponse{Items: items})
}

func (h *Handlers) observeRuntimeClusterPressure(ctx context.Context, clusters []model.RuntimeCluster, detailed bool) []runtimeClusterPressureResponse {
	const concurrency = 6
	items := make([]runtimeClusterPressureResponse, len(clusters))
	guard := make(chan struct{}, concurrency)
	var wait sync.WaitGroup
	for index := range clusters {
		index := index
		wait.Add(1)
		go func() {
			defer wait.Done()
			select {
			case guard <- struct{}{}:
				defer func() { <-guard }()
			case <-ctx.Done():
				items[index] = unavailableRuntimeClusterPressure(clusters[index].ID, "runtime_cluster.pressure_observation_cancelled")
				return
			}
			items[index] = h.observeOneRuntimeClusterPressure(ctx, clusters[index], detailed)
		}()
	}
	wait.Wait()
	return items
}

func (h *Handlers) observeOneRuntimeClusterPressure(ctx context.Context, cluster model.RuntimeCluster, detailed bool) runtimeClusterPressureResponse {
	if strings.TrimSpace(cluster.KubeconfigRef) == "" {
		return unavailableRuntimeClusterPressure(cluster.ID, "runtime_cluster.kubeconfig_not_configured")
	}
	kubeconfig := strings.TrimSpace(h.secrets.ResolveContext(ctx, cluster.KubeconfigRef))
	if kubeconfig == "" {
		return unavailableRuntimeClusterPressure(cluster.ID, "runtime_cluster.kubeconfig_unavailable")
	}
	client, err := kubeprovider.NewClientFromKubeconfig(kubeconfig)
	if err != nil {
		return unavailableRuntimeClusterPressure(cluster.ID, "runtime_cluster.invalid_kubeconfig")
	}
	probeCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	snapshot, err := client.ClusterPressure(probeCtx)
	if err != nil {
		code := "runtime_cluster.pressure_upstream_unavailable"
		if apierrors.IsForbidden(err) {
			code = "runtime_cluster.pressure_forbidden"
		} else if errors.Is(err, context.DeadlineExceeded) {
			code = "runtime_cluster.pressure_timeout"
		}
		return unavailableRuntimeClusterPressure(cluster.ID, code)
	}
	return runtimeClusterPressureFromSnapshot(cluster.ID, snapshot, detailed)
}

func runtimeClusterPressureFromSnapshot(clusterID string, snapshot kubeprovider.ClusterPressureSnapshot, detailed bool) runtimeClusterPressureResponse {
	cpuRequestPercent := percent(snapshot.CPURequestsMilli, snapshot.CPUAllocatableMilli)
	memoryRequestPercent := percent(snapshot.MemoryRequestsBytes, snapshot.MemoryAllocatableBytes)
	var cpuUsagePercent, memoryUsagePercent *float64
	if snapshot.MetricsAvailable {
		cpu := percent(snapshot.CPUUsageMilli, snapshot.CPUAllocatableMilli)
		memory := percent(snapshot.MemoryUsageBytes, snapshot.MemoryAllocatableBytes)
		cpuUsagePercent = &cpu
		memoryUsagePercent = &memory
	}
	score := runtimeClusterPressureScore(cpuRequestPercent, memoryRequestPercent, cpuUsagePercent, memoryUsagePercent)
	response := runtimeClusterPressureResponse{
		ClusterID: clusterID, Status: observation.StatusReady, PressureLevel: runtimeClusterPressureLevel(score),
		ObservedAt: snapshot.ObservedAt,
	}
	if !detailed {
		return response
	}
	response.PressureScore = &score
	response.Details = &runtimeClusterPressureDetails{
		CPU: runtimeClusterPressureResource{
			Requests: snapshot.CPURequestsMilli, Allocatable: snapshot.CPUAllocatableMilli,
			Usage: optionalInt64(snapshot.CPUUsageMilli, snapshot.MetricsAvailable), RequestPercent: cpuRequestPercent, UsagePercent: cpuUsagePercent,
		},
		Memory: runtimeClusterPressureResource{
			Requests: snapshot.MemoryRequestsBytes, Allocatable: snapshot.MemoryAllocatableBytes,
			Usage: optionalInt64(snapshot.MemoryUsageBytes, snapshot.MetricsAvailable), RequestPercent: memoryRequestPercent, UsagePercent: memoryUsagePercent,
		},
		NodeCount: snapshot.NodeCount, PodCount: snapshot.PodCount, MetricsAvailable: snapshot.MetricsAvailable,
	}
	return response
}

func runtimeClusterPressureScore(cpuRequest, memoryRequest float64, cpuUsage, memoryUsage *float64) float64 {
	weighted := cpuRequest*0.45 + memoryRequest*0.55
	maxPressure := max(cpuRequest, memoryRequest)
	if cpuUsage != nil && memoryUsage != nil {
		weighted = cpuRequest*0.30 + memoryRequest*0.35 + *cpuUsage*0.15 + *memoryUsage*0.20
		maxPressure = max(maxPressure, *cpuUsage, *memoryUsage)
	}
	// A saturated single dimension must not be hidden by otherwise idle resources.
	return roundPercent(max(weighted, maxPressure*0.90))
}

func runtimeClusterPressureLevel(score float64) string {
	switch {
	case score < 20:
		return "idle"
	case score < 45:
		return "light"
	case score < 70:
		return "moderate"
	case score < 90:
		return "heavy"
	default:
		return "full"
	}
}

func unavailableRuntimeClusterPressure(clusterID, code string) runtimeClusterPressureResponse {
	return runtimeClusterPressureResponse{
		ClusterID: clusterID, Status: observation.StatusUnavailable, PressureLevel: "unavailable",
		ObservationCode: code, ObservedAt: time.Now().UTC(),
	}
}

func runtimeClusterPressureIDs(raw []string) ([]string, bool) {
	if len(raw) == 0 || len(raw) > maxRuntimeClusterPressureBatch {
		return nil, false
	}
	seen := make(map[string]struct{}, len(raw))
	ids := make([]string, 0, len(raw))
	for _, value := range raw {
		id := strings.TrimSpace(value)
		if id == "" || !strings.HasPrefix(id, "clu_") {
			return nil, false
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, true
}

func percent(value, total int64) float64 {
	if total <= 0 {
		return 0
	}
	return roundPercent(float64(value) * 100 / float64(total))
}

func roundPercent(value float64) float64 {
	return math.Round(value*10) / 10
}

func optionalInt64(value int64, available bool) *int64 {
	if !available {
		return nil
	}
	return &value
}
