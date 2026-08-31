package api

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/gin-gonic/gin"
)

type applicationTopologyTargetResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Stage       string `json:"stage"`
	ClusterID   string `json:"clusterId"`
	ClusterName string `json:"clusterName"`
	Namespace   string `json:"namespace"`
}

type applicationTopologyWarningResponse struct {
	Code                 string `json:"code"`
	DeploymentTargetID   string `json:"deploymentTargetId"`
	DeploymentTargetName string `json:"deploymentTargetName"`
	ClusterID            string `json:"clusterId"`
	ClusterName          string `json:"clusterName"`
}

type applicationTopologyResponse struct {
	GeneratedAt time.Time                              `json:"generatedAt"`
	Targets     []applicationTopologyTargetResponse    `json:"targets"`
	Nodes       []kubeprovider.ApplicationTopologyNode `json:"nodes"`
	Edges       []kubeprovider.ApplicationTopologyEdge `json:"edges"`
	Warnings    []applicationTopologyWarningResponse   `json:"warnings"`
}

func (h *Handlers) GetApplicationTopology(ctx *gin.Context) {
	markLiveObservationResponse(ctx)
	user, project, ok := h.authorizeProject(ctx, authz.ActionApplicationRead)
	if !ok {
		return
	}
	if _, ok := h.findApplication(ctx); !ok {
		return
	}

	var targets []model.DeploymentTarget
	if err := h.dbFor(ctx).WithContext(ctx).Where(
		"project_id = ? and application_id = ? and delete_status <> ?",
		project.ID,
		ctx.Param("applicationId"),
		"deleted",
	).Order("created_at asc").Find(&targets).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, "failed to load deployment targets")
		return
	}

	nodes := map[string]kubeprovider.ApplicationTopologyNode{}
	edges := map[string]kubeprovider.ApplicationTopologyEdge{}
	response := applicationTopologyResponse{
		GeneratedAt: time.Now().UTC(),
		Targets:     make([]applicationTopologyTargetResponse, 0, len(targets)),
		Nodes:       []kubeprovider.ApplicationTopologyNode{},
		Edges:       []kubeprovider.ApplicationTopologyEdge{},
		Warnings:    []applicationTopologyWarningResponse{},
	}

	for _, target := range targets {
		cluster, allowed := h.topologyClusterForTarget(ctx, user, project.ID, target)
		if !allowed {
			return
		}
		namespace := deploymentTargetNamespace(project, target)
		response.Targets = append(response.Targets, applicationTopologyTargetResponse{
			ID:          target.ID,
			Name:        target.Name,
			Stage:       target.Stage,
			ClusterID:   cluster.ID,
			ClusterName: cluster.Name,
			Namespace:   namespace,
		})
		warning := applicationTopologyWarningResponse{
			DeploymentTargetID:   target.ID,
			DeploymentTargetName: target.Name,
			ClusterID:            cluster.ID,
			ClusterName:          cluster.Name,
		}
		if strings.TrimSpace(cluster.KubeconfigRef) == "" {
			warning.Code = "cluster_kubeconfig_missing"
			response.Warnings = append(response.Warnings, warning)
			continue
		}
		kubeconfig := h.secrets.ResolveContext(ctx.Request.Context(), cluster.KubeconfigRef)
		if strings.TrimSpace(kubeconfig) == "" {
			warning.Code = "cluster_kubeconfig_unavailable"
			response.Warnings = append(response.Warnings, warning)
			continue
		}
		client, err := kubeprovider.NewClientFromKubeconfig(kubeconfig)
		if err != nil {
			warning.Code = "cluster_client_failed"
			response.Warnings = append(response.Warnings, warning)
			continue
		}
		readContext, cancelRead := context.WithTimeout(ctx, 12*time.Second)
		snapshot, err := client.BuildApplicationTopology(readContext, kubeprovider.ApplicationTopologyOptions{
			ClusterID:          cluster.ID,
			ClusterName:        cluster.Name,
			Namespace:          namespace,
			ProjectID:          project.ID,
			ApplicationID:      ctx.Param("applicationId"),
			DeploymentTargetID: target.ID,
		})
		cancelRead()
		if err != nil {
			warning.Code = "topology_read_failed"
			response.Warnings = append(response.Warnings, warning)
			continue
		}
		for _, node := range snapshot.Nodes {
			nodes[node.ID] = node
		}
		for _, edge := range snapshot.Edges {
			edges[edge.ID] = edge
		}
	}

	for _, node := range nodes {
		response.Nodes = append(response.Nodes, node)
	}
	for _, edge := range edges {
		response.Edges = append(response.Edges, edge)
	}
	sort.Slice(response.Nodes, func(i, j int) bool { return response.Nodes[i].ID < response.Nodes[j].ID })
	sort.Slice(response.Edges, func(i, j int) bool { return response.Edges[i].ID < response.Edges[j].ID })
	ctx.JSON(http.StatusOK, response)
}

func (h *Handlers) topologyClusterForTarget(ctx *gin.Context, user model.User, projectID string, target model.DeploymentTarget) (model.RuntimeCluster, bool) {
	if strings.TrimSpace(target.ClusterID) != "" {
		return h.runtimeClusterForProjectUse(ctx, user, projectID, target.ClusterID)
	}
	return h.runtimeClusterForDeploymentTarget(ctx, target)
}
