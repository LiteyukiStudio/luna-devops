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
