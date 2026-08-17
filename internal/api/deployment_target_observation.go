package api

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/observation"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

const deploymentTargetObservationTimeout = 8 * time.Second

func (h *Handlers) observeDeploymentTargets(ctx context.Context, project model.Project, targets []model.DeploymentTarget) {
	const concurrency = 6
	guard := make(chan struct{}, concurrency)
	var wait sync.WaitGroup
	for index := range targets {
		index := index
		wait.Add(1)
		go func() {
			defer wait.Done()
			select {
			case guard <- struct{}{}:
				defer func() { <-guard }()
			case <-ctx.Done():
				targets[index] = unavailableDeploymentTarget(targets[index], "deployment_target.observation_cancelled")
				return
			}
			targets[index] = h.observeDeploymentTarget(ctx, project, targets[index])
		}()
	}
	wait.Wait()
}

func (h *Handlers) observeDeploymentTarget(ctx context.Context, project model.Project, target model.DeploymentTarget) model.DeploymentTarget {
	observedAt := time.Now().UTC()
	target.LastCheckedAt = &observedAt
	if !target.Enabled {
		target.Status = "disabled"
		return target
	}
	if strings.TrimSpace(target.KubernetesName) == "" {
		target.Status = observation.StatusNotConfigured
		target.ObservationCode = "deployment_target.resource_name_not_configured"
		return target
	}

	cluster, err := h.deploymentTargetRuntimeCluster(project.ID, target.ClusterID, ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		target.Status = observation.StatusNotConfigured
		target.ObservationCode = "deployment_target.runtime_cluster_not_configured"
		return target
	}
	if err != nil {
		return unavailableDeploymentTarget(target, "deployment_target.runtime_cluster_unavailable")
	}
	kubeconfig := strings.TrimSpace(h.secrets.ResolveContext(ctx, cluster.KubeconfigRef))
	if strings.TrimSpace(cluster.KubeconfigRef) == "" || kubeconfig == "" {
		target.Status = observation.StatusNotConfigured
		target.ObservationCode = "deployment_target.kubeconfig_not_configured"
		return target
	}
	client, err := kubeprovider.NewClientFromKubeconfig(kubeconfig)
	if err != nil {
		return unavailableDeploymentTarget(target, "deployment_target.invalid_kubeconfig")
	}

	namespace := strings.TrimSpace(target.Namespace)
	if namespace == "" {
		namespace = runtimeProjectNamespace(project)
	}
	probeCtx, cancel := context.WithTimeout(ctx, deploymentTargetObservationTimeout)
	defer cancel()
	snapshot, err := client.GetWorkloadSnapshot(probeCtx, namespace, target.KubernetesName, normalizeWorkloadType(target.WorkloadType))
	switch {
	case apierrors.IsNotFound(err):
		target.Status = observation.StatusNotFound
		target.ObservationCode = "deployment_target.workload_not_found"
		return target
	case err != nil:
		return unavailableDeploymentTarget(target, "deployment_target.workload_unavailable")
	}

	target.DesiredReplicas = snapshot.DesiredReplicas
	target.UpdatedReplicas = snapshot.UpdatedReplicas
	target.ReadyReplicas = snapshot.ReadyReplicas
	target.AvailableReplicas = snapshot.AvailableReplicas
	target.Status = deploymentObservationFromSnapshot(snapshot)
	return target
}

func (h *Handlers) deploymentTargetRuntimeCluster(projectID, clusterID string, ctx context.Context) (model.RuntimeCluster, error) {
	var cluster model.RuntimeCluster
	query := h.dbWithContext(ctx).Where("type in ?", []string{"kubernetes", "k3s"})
	if strings.TrimSpace(clusterID) != "" {
		return cluster, query.First(&cluster, "id = ?", clusterID).Error
	}
	if err := query.Where("is_default = ?", true).Order("created_at asc").First(&cluster).Error; err == nil {
		return cluster, nil
	}
	return cluster, query.
		Where("scope = ? OR (scope = ? AND owner_ref = ?)", "global", "project", projectID).
		Order("created_at asc").
		First(&cluster).Error
}

func deploymentObservationFromSnapshot(snapshot kubeprovider.DeploymentSnapshot) string {
	switch snapshot.Phase {
	case kubeprovider.DeploymentSucceeded:
		if snapshot.DesiredReplicas == 0 {
			return observation.StatusScaledToZero
		}
		return observation.StatusReady
	case kubeprovider.DeploymentFailed:
		return observation.StatusDegraded
	default:
		return observation.StatusProgressing
	}
}

func unavailableDeploymentTarget(target model.DeploymentTarget, code string) model.DeploymentTarget {
	observedAt := time.Now().UTC()
	target.Status = observation.StatusUnavailable
	target.ObservationCode = code
	target.LastCheckedAt = &observedAt
	return target
}
