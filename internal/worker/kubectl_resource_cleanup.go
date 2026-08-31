package worker

import (
	"context"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/runtimecluster"
)

// kubectlCleanupClustersForProject resolves the clusters on which this
// project has ever received a kube binding. Revoked credentials retain their
// bindings, so deletion can still find and clean resources created earlier.
func (r *Runner) kubectlCleanupClustersForProject(ctx context.Context, projectID string) ([]model.RuntimeCluster, error) {
	projectID = strings.TrimSpace(projectID)
	if r == nil || r.db == nil || projectID == "" {
		return nil, nil
	}
	clusterIDs := r.db.WithContext(ctx).Model(&model.KubeAccessBinding{}).
		Select("DISTINCT runtime_cluster_id").Where("project_id = ?", projectID)
	var clusters []model.RuntimeCluster
	if err := runtimecluster.ActiveScope(r.db.WithContext(ctx)).
		Where("type in ? and id in (?)", []string{"kubernetes", "k3s"}, clusterIDs).
		Order("created_at asc").Find(&clusters).Error; err != nil {
		return nil, err
	}
	return clusters, nil
}

func (r *Runner) cleanupApplicationKubectlResources(ctx context.Context, project model.Project, applicationID string) error {
	clusters, err := r.kubectlCleanupClustersForProject(ctx, project.ID)
	if err != nil {
		return err
	}
	for _, cluster := range clusters {
		manager, err := r.kubectlGatewayManagerForCluster(ctx, cluster)
		if err != nil {
			return err
		}
		_, extra := kubectlGatewayExtraAccess(cluster.KubeGatewayExtraResourceRules)
		if err := manager.CleanupBindingManagedResources(ctx, kubeprovider.KubectlManagedCleanupSpec{
			ProjectID: project.ID, ApplicationID: strings.TrimSpace(applicationID),
			Namespace: projectNamespace(project), ExtraGVRs: extra,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) cleanupProjectKubectlNamespaces(ctx context.Context, project model.Project) error {
	clusters, err := r.kubectlCleanupClustersForProject(ctx, project.ID)
	if err != nil {
		return err
	}
	for _, cluster := range clusters {
		manager, err := r.kubectlGatewayManagerForCluster(ctx, cluster)
		if err != nil {
			return err
		}
		if err := manager.DeleteManagedProjectNamespace(ctx, projectNamespace(project)); err != nil && !isKubernetesNotFound(err) {
			return err
		}
	}
	return nil
}
