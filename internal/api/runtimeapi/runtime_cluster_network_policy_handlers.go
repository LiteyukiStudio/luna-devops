package runtimeapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/observation"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/runtimecluster"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// runtimeClusterNetworkPolicyResponse 是项目空间网络隔离的实时比对结果，不读取数据库中的同步记录。
type runtimeClusterNetworkPolicyResponse struct {
	ClusterID       string    `json:"clusterId"`
	Status          string    `json:"status"`
	ObservationCode string    `json:"observationCode,omitempty"`
	ObservedAt      time.Time `json:"observedAt"`
	NamespaceCount  int       `json:"namespaceCount"`
	SyncedCount     int       `json:"syncedCount"`
	DriftedCount    int       `json:"driftedCount"`
}

type workloadNetworkPolicyObserver interface {
	ListProjectNamespaces(ctx context.Context) (map[string]string, error)
	ClusterNodeAddresses(ctx context.Context) ([]string, error)
	WorkloadNetworkPolicyState(ctx context.Context, namespace string, desired []networkingv1.NetworkPolicy) (string, error)
}

func (h *Handlers) ObserveRuntimeClusterNetworkPolicies(ctx *gin.Context) {
	markLiveObservationResponse(ctx)
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var cluster model.RuntimeCluster
	if err := h.dbFor(ctx).First(&cluster, "id = ?", ctx.Param("clusterId")).Error; err != nil {
		writeErrorCode(ctx, http.StatusNotFound, "runtime_cluster.not_found", "runtime cluster not found")
		return
	}
	if !runtimecluster.IsActive(cluster) {
		writeErrorCode(ctx, http.StatusConflict, "runtime_cluster.not_active", "runtime cluster is not active")
		return
	}
	if !h.canManageScopedResourceByID(ctx, user, cluster.Scope, cluster.OwnerRef, scopedResourceRuntimeCluster, cluster.ID, "无权查看该运行集群的网络隔离状态") {
		return
	}
	ctx.JSON(http.StatusOK, h.observeRuntimeClusterNetworkPolicies(ctx.Request.Context(), cluster))
}

func (h *Handlers) observeRuntimeClusterNetworkPolicies(ctx context.Context, cluster model.RuntimeCluster) runtimeClusterNetworkPolicyResponse {
	if strings.TrimSpace(cluster.KubeconfigRef) == "" {
		return unavailableRuntimeClusterNetworkPolicy(cluster.ID, "runtime_cluster.kubeconfig_not_configured")
	}
	kubeconfig := strings.TrimSpace(h.secrets.ResolveContext(ctx, cluster.KubeconfigRef))
	if kubeconfig == "" {
		return unavailableRuntimeClusterNetworkPolicy(cluster.ID, "runtime_cluster.kubeconfig_unavailable")
	}
	client, err := kubeprovider.NewClientFromKubeconfig(kubeconfig)
	if err != nil {
		return unavailableRuntimeClusterNetworkPolicy(cluster.ID, "runtime_cluster.invalid_kubeconfig")
	}
	probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return observeWorkloadNetworkPolicies(probeCtx, h.dbWithContext(probeCtx), cluster, client)
}

func observeWorkloadNetworkPolicies(ctx context.Context, db *gorm.DB, cluster model.RuntimeCluster, observer workloadNetworkPolicyObserver) runtimeClusterNetworkPolicyResponse {
	namespaces, err := observer.ListProjectNamespaces(ctx)
	if err != nil {
		return unavailableRuntimeClusterNetworkPolicy(cluster.ID, networkPolicyObservationCode(err))
	}
	nodes, err := observer.ClusterNodeAddresses(ctx)
	if err != nil {
		return unavailableRuntimeClusterNetworkPolicy(cluster.ID, networkPolicyObservationCode(err))
	}
	response := runtimeClusterNetworkPolicyResponse{ClusterID: cluster.ID, Status: observation.StatusReady, ObservedAt: time.Now().UTC()}
	for namespace, projectID := range namespaces {
		var project model.Project
		if err := db.WithContext(ctx).Select("id", "delete_status").
			Where("id = ? and kubernetes_namespace = ?", projectID, namespace).Take(&project).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			return unavailableRuntimeClusterNetworkPolicy(cluster.ID, "runtime_cluster.network_policy_unavailable")
		}
		if !model.DeleteStatusActive(project.DeleteStatus) {
			continue
		}
		desired, err := runtimecluster.DesiredWorkloadNetworkPolicies(ctx, db, cluster, project.ID, namespace, nodes)
		if err != nil {
			return unavailableRuntimeClusterNetworkPolicy(cluster.ID, "runtime_cluster.network_policy_unavailable")
		}
		state, err := observer.WorkloadNetworkPolicyState(ctx, namespace, desired)
		if err != nil {
			return unavailableRuntimeClusterNetworkPolicy(cluster.ID, networkPolicyObservationCode(err))
		}
		response.NamespaceCount++
		if state == kubeprovider.WorkloadNetworkPolicySynced {
			response.SyncedCount++
		} else {
			response.DriftedCount++
		}
	}
	if response.DriftedCount > 0 {
		response.Status = observation.StatusDegraded
		response.ObservationCode = "runtime_cluster.network_policy_drifted"
	}
	return response
}

func networkPolicyObservationCode(err error) string {
	switch {
	case apierrors.IsForbidden(err):
		return "runtime_cluster.network_policy_forbidden"
	case errors.Is(err, context.DeadlineExceeded):
		return "runtime_cluster.network_policy_timeout"
	default:
		return "runtime_cluster.network_policy_unavailable"
	}
}

func unavailableRuntimeClusterNetworkPolicy(clusterID string, code string) runtimeClusterNetworkPolicyResponse {
	return runtimeClusterNetworkPolicyResponse{ClusterID: clusterID, Status: observation.StatusUnavailable, ObservationCode: code, ObservedAt: time.Now().UTC()}
}
