package deploymentapi

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/observation"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/runtimecluster"
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
				targets[index] = withDisabledStatus(unavailableDeploymentTarget(targets[index], "deployment_target.observation_cancelled"))
				return
			}
			targets[index] = h.observeDeploymentTarget(ctx, project, targets[index])
		}()
	}
	wait.Wait()
}

// observeDeploymentTarget reads the target's workload from the cluster
// resolved by runtimecluster.ResolveForTarget. Disabled targets are observed
// too, because disabling does not stop a running workload: their status stays
// "disabled" while replica counts and observationCode reflect Kubernetes.
func (h *Handlers) observeDeploymentTarget(ctx context.Context, project model.Project, target model.DeploymentTarget) model.DeploymentTarget {
	return withDisabledStatus(h.observeDeploymentTargetWorkload(ctx, project, target))
}

func withDisabledStatus(target model.DeploymentTarget) model.DeploymentTarget {
	if !target.Enabled {
		target.Status = observation.StatusDisabled
	}
	return target
}

func (h *Handlers) observeDeploymentTargetWorkload(ctx context.Context, project model.Project, target model.DeploymentTarget) model.DeploymentTarget {
	observedAt := time.Now().UTC()
	target.LastCheckedAt = &observedAt
	cluster, err := runtimecluster.ResolveForTarget(h.dbWithContext(ctx), target.ClusterID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		target.Status = observation.StatusNotConfigured
		target.ObservationCode = "deployment_target.runtime_cluster_not_configured"
		return target
	}
	if err != nil {
		return unavailableDeploymentTarget(target, "deployment_target.runtime_cluster_unavailable")
	}
	target.EffectiveClusterID = cluster.ID
	if strings.TrimSpace(target.KubernetesName) == "" {
		target.Status = observation.StatusNotConfigured
		target.ObservationCode = "deployment_target.resource_name_not_configured"
		return target
	}
	kubeconfig := ""
	if strings.TrimSpace(cluster.KubeconfigRef) != "" {
		kubeconfig = strings.TrimSpace(h.secrets.ResolveContext(ctx, cluster.KubeconfigRef))
	}
	if kubeconfig == "" {
		target.Status = observation.StatusNotConfigured
		target.ObservationCode = "deployment_target.kubeconfig_not_configured"
		return target
	}
	client, err := kubeprovider.NewClientFromKubeconfig(kubeconfig)
	if err != nil {
		return unavailableDeploymentTarget(target, "deployment_target.invalid_kubeconfig")
	}

	namespace := runtimeProjectNamespace(project)
	probeCtx, cancel := context.WithTimeout(ctx, deploymentTargetObservationTimeout)
	defer cancel()
	observed, err := client.ObserveWorkloadRuntime(probeCtx, namespace, target.ID, target.KubernetesName, normalizeWorkloadType(target.WorkloadType))
	switch {
	case apierrors.IsNotFound(err):
		target.Status = observation.StatusNotFound
		target.ObservationCode = "deployment_target.workload_not_found"
		return target
	case err != nil:
		return unavailableDeploymentTarget(target, "deployment_target.workload_unavailable")
	}

	target.DesiredReplicas = observed.Workload.DesiredReplicas
	target.UpdatedReplicas = observed.Workload.UpdatedReplicas
	target.ReadyReplicas = observed.Workload.ReadyReplicas
	target.AvailableReplicas = observed.Workload.AvailableReplicas
	target.Status = observed.Status
	target.ObservationCode = observed.ObservationCode
	return target
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
