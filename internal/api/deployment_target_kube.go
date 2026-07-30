package api

import (
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/gin-gonic/gin"
)

func (h *Handlers) kubernetesClientForEnvironment(ctx *gin.Context, project model.Project, environment model.Environment, errorMessage string) (*kubeprovider.Client, string, bool) {
	managerCluster, ok := h.runtimeClusterForEnvironment(ctx, environment)
	if !ok {
		return nil, "", false
	}
	kubeconfig := h.secrets.Resolve(managerCluster.KubeconfigRef)
	if strings.TrimSpace(kubeconfig) == "" {
		writeError(ctx, http.StatusBadRequest, errorMessage)
		return nil, "", false
	}
	client, err := kubeprovider.NewClientFromKubeconfig(kubeconfig)
	if err != nil {
		writeError(ctx, http.StatusBadRequest, "运行集群 kubeconfig 无效")
		return nil, "", false
	}
	namespace := runtimeProjectNamespace(project)
	return client, namespace, true
}

func (h *Handlers) kubernetesClientForDeploymentTarget(ctx *gin.Context, project model.Project, target model.DeploymentTarget, errorMessage string) (*kubeprovider.Client, string, bool) {
	managerCluster, ok := h.runtimeClusterForDeploymentTarget(ctx, target)
	if !ok {
		return nil, "", false
	}
	kubeconfig := h.secrets.Resolve(managerCluster.KubeconfigRef)
	if strings.TrimSpace(kubeconfig) == "" {
		writeError(ctx, http.StatusBadRequest, errorMessage)
		return nil, "", false
	}
	client, err := kubeprovider.NewClientFromKubeconfig(kubeconfig)
	if err != nil {
		writeError(ctx, http.StatusBadRequest, "运行集群 kubeconfig 无效")
		return nil, "", false
	}
	return client, deploymentTargetNamespace(project, target), true
}

// kubernetesClientForDeploymentTargetObservation resolves a read-only live
// observation dependency without writing an HTTP response. Callers can return
// a stable unavailable observation instead of failing the whole resource API.
func (h *Handlers) kubernetesClientForDeploymentTargetObservation(project model.Project, target model.DeploymentTarget) (*kubeprovider.Client, string, string) {
	cluster, err := h.runtimeClusterForDeploymentTargetValue(target)
	if err != nil {
		return nil, "", "runtime_cluster_not_found"
	}
	kubeconfig := h.secrets.Resolve(cluster.KubeconfigRef)
	if strings.TrimSpace(kubeconfig) == "" {
		return nil, "", "kubeconfig_not_configured"
	}
	client, err := kubeprovider.NewClientFromKubeconfig(kubeconfig)
	if err != nil {
		return nil, "", "kubeconfig_invalid"
	}
	return client, deploymentTargetNamespace(project, target), ""
}
