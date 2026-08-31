package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type kubectlGatewayAccessManager interface {
	ReconcileGatewayAccess(context.Context, kubeprovider.GatewayAccessSpec) (kubeprovider.GatewayAccessObservation, error)
	CleanupGatewayAccess(context.Context, kubeprovider.GatewayAccessSpec) error
}

func (r *Runner) handleKubectlGateway(ctx context.Context, task *asynq.Task) error {
	var payload tasks.KubectlGatewayPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return err
	}
	return r.reconcileKubectlGatewayCluster(ctx, strings.TrimSpace(payload.ClusterID))
}

func (r *Runner) reconcileKubectlGatewayCluster(ctx context.Context, clusterID string) error {
	clusterID = strings.TrimSpace(clusterID)
	if clusterID == "" || r == nil || r.db == nil {
		return nil
	}
	return r.withKubectlGatewayClusterLock(ctx, clusterID, func(lockCtx context.Context) error {
		cluster, projects, err := r.loadKubectlGatewayClusterState(lockCtx, clusterID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if kubectlGatewayDeleteStatus(cluster) != "active" {
			return nil
		}
		manager, err := r.kubectlGatewayManagerForCluster(lockCtx, cluster)
		if err != nil {
			return err
		}
		spec := kubectlGatewayAccessSpec(cluster, projects)
		if !spec.Enabled {
			return manager.CleanupGatewayAccess(lockCtx, spec)
		}
		_, err = manager.ReconcileGatewayAccess(lockCtx, spec)
		return err
	})
}

func (r *Runner) kubectlGatewayManagerForCluster(ctx context.Context, cluster model.RuntimeCluster) (*kubeprovider.KubectlGatewayManager, error) {
	kubeconfig := r.secrets.ResolveContext(ctx, cluster.KubeconfigRef)
	if strings.TrimSpace(kubeconfig) == "" {
		return nil, errors.New("runtime cluster kubeconfig is missing")
	}
	client, err := kubeprovider.NewClientFromKubeconfig(kubeconfig)
	if err != nil {
		return nil, runtimeClusterKubeconfigError(err)
	}
	return kubeprovider.NewKubectlGatewayManager(client), nil
}

func kubectlGatewayAccessSpec(cluster model.RuntimeCluster, projects []model.Project) kubeprovider.GatewayAccessSpec {
	spec := kubeprovider.GatewayAccessSpec{
		RuntimeClusterID: cluster.ID,
		Enabled:          cluster.KubeGatewayEnabled,
		Projects:         make([]kubeprovider.GatewayAccessProjectSpec, 0, len(projects)),
	}
	spec.ExtraProjectRules, spec.ExtraManagedResources = kubectlGatewayExtraAccess(cluster.KubeGatewayExtraResourceRules)
	for _, project := range projects {
		namespace := projectNamespace(project)
		if strings.TrimSpace(project.ID) == "" || namespace == "" {
			continue
		}
		spec.Projects = append(spec.Projects, kubeprovider.GatewayAccessProjectSpec{
			ProjectID: project.ID,
			Namespace: namespace,
		})
	}
	return spec
}

type kubectlGatewayStoredRule struct {
	APIGroup     string   `json:"apiGroup"`
	APIVersion   string   `json:"apiVersion"`
	Resource     string   `json:"resource"`
	Subresources []string `json:"subresources"`
	Verbs        []string `json:"verbs"`
}

func kubectlGatewayExtraAccess(raw string) ([]rbacv1.PolicyRule, []schema.GroupVersionResource) {
	var stored []kubectlGatewayStoredRule
	if strings.TrimSpace(raw) == "" || json.Unmarshal([]byte(raw), &stored) != nil {
		return nil, nil
	}
	rules := make([]rbacv1.PolicyRule, 0, len(stored))
	resourcesToClean := make([]schema.GroupVersionResource, 0, len(stored))
	for _, item := range stored {
		resource := strings.ToLower(strings.TrimSpace(item.Resource))
		if resource == "" {
			continue
		}
		version := strings.ToLower(strings.TrimSpace(item.APIVersion))
		if version == "" {
			continue
		}
		resources := []string{resource}
		for _, subresource := range item.Subresources {
			if subresource = strings.ToLower(strings.TrimSpace(subresource)); subresource != "" {
				resources = append(resources, resource+"/"+subresource)
			}
		}
		rules = append(rules, rbacv1.PolicyRule{
			APIGroups: []string{strings.TrimSpace(item.APIGroup)},
			Resources: resources,
			Verbs:     append([]string(nil), item.Verbs...),
		})
		resourcesToClean = append(resourcesToClean, schema.GroupVersionResource{
			Group: strings.TrimSpace(item.APIGroup), Version: version, Resource: resource,
		})
	}
	return rules, resourcesToClean
}

func (r *Runner) loadKubectlGatewayClusterState(ctx context.Context, clusterID string) (model.RuntimeCluster, []model.Project, error) {
	var cluster model.RuntimeCluster
	if err := r.db.WithContext(ctx).First(&cluster, "id = ? and type in ?", clusterID, []string{"kubernetes", "k3s"}).Error; err != nil {
		return model.RuntimeCluster{}, nil, err
	}
	projects, err := r.listKubectlGatewayProjects(ctx, cluster)
	if err != nil {
		return model.RuntimeCluster{}, nil, err
	}
	return cluster, projects, nil
}

func (r *Runner) listKubectlGatewayProjects(ctx context.Context, cluster model.RuntimeCluster) ([]model.Project, error) {
	var projects []model.Project
	query := r.db.WithContext(ctx).Model(&model.Project{}).Where("delete_status = ?", "active")
	switch strings.TrimSpace(cluster.Scope) {
	case "project":
		query = query.Where("id IN (?)", r.db.WithContext(ctx).Model(&model.ScopedResourceProjectBinding{}).
			Select("project_id").Where("resource_type = ? and resource_id = ?", "runtime_cluster", cluster.ID))
	case "user":
		query = query.Where("id IN (?)", r.db.WithContext(ctx).Model(&model.ProjectMember{}).
			Select("project_id").Where("user_id = ?", cluster.OwnerRef))
	}
	if err := query.Order("created_at asc").Find(&projects).Error; err != nil {
		return nil, err
	}
	return projects, nil
}

func (r *Runner) withKubectlGatewayClusterLock(ctx context.Context, clusterID string, run func(context.Context) error) error {
	if r == nil || r.db == nil {
		return run(ctx)
	}
	lockKey := kubectlGatewayAdvisoryLockKey(clusterID)
	if r.db.Dialector.Name() != "postgres" {
		return run(ctx)
	}
	return r.db.WithContext(ctx).Connection(func(connection *gorm.DB) error {
		var acquired bool
		if err := connection.Raw("SELECT pg_try_advisory_lock(?)", lockKey).Scan(&acquired).Error; err != nil {
			return err
		}
		if !acquired {
			return fmt.Errorf("kubectl gateway lock busy for cluster %s", clusterID)
		}
		defer func() {
			unlockCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			var released bool
			_ = connection.WithContext(unlockCtx).Raw("SELECT pg_advisory_unlock(?)", lockKey).Scan(&released).Error
		}()
		return run(ctx)
	})
}

func kubectlGatewayAdvisoryLockKey(clusterID string) int64 {
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(strings.TrimSpace(clusterID)))
	return int64(hasher.Sum64())
}

func kubectlGatewayDeleteStatus(cluster model.RuntimeCluster) string {
	if status := strings.TrimSpace(cluster.DeleteStatus); status != "" {
		return status
	}
	return "active"
}

func clusterKubectlGatewayEnabled(cluster model.RuntimeCluster) bool {
	return cluster.KubeGatewayEnabled
}
