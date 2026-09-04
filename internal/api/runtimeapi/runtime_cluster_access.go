package runtimeapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	projectservice "github.com/LiteyukiStudio/devops/internal/project"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/runtimecluster"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *Handlers) runtimeClusterForDeploymentTarget(ctx *gin.Context, target model.DeploymentTarget) (model.RuntimeCluster, bool) {
	var cluster model.RuntimeCluster
	query := runtimecluster.ActiveScope(h.dbFor(ctx))
	if clusterID := strings.TrimSpace(target.ClusterID); clusterID != "" {
		err := query.First(&cluster, "id = ?", clusterID).Error
		if err != nil {
			writeError(ctx, http.StatusNotFound, "runtime cluster not found")
			return cluster, false
		}
		return cluster, true
	}
	err := query.Where("scope = ? and is_default = ?", "global", true).First(&cluster).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = query.Where("scope = ?", "global").Order("created_at asc").First(&cluster).Error
	}
	if err != nil {
		writeError(ctx, http.StatusNotFound, "runtime cluster not found")
		return cluster, false
	}
	return cluster, true
}

func (h *Handlers) runtimeClusterForProjectUse(ctx *gin.Context, user model.User, projectID string, clusterID string) (model.RuntimeCluster, bool) {
	clusterID = strings.TrimSpace(clusterID)
	if clusterID == "" {
		return model.RuntimeCluster{}, true
	}
	var cluster model.RuntimeCluster
	if err := runtimecluster.ActiveScope(h.dbFor(ctx)).First(&cluster, "id = ?", clusterID).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, "运行集群不存在")
		return cluster, false
	}
	if canUseRuntimeClusterForProject(user, cluster, projectID, h.scopedResourceProjectIDs(scopedResourceRuntimeCluster, cluster.ID, ctx.Request.Context())) {
		return cluster, true
	}
	writeErrorCode(ctx, http.StatusForbidden, "runtime_cluster.forbidden", "无权使用该运行集群")
	return cluster, false
}

func (h *Handlers) defaultRuntimeClusterID(ctx context.Context) string {
	var cluster model.RuntimeCluster
	query := runtimecluster.ActiveScope(h.dbWithContext(ctx))
	err := query.Where("is_default = ?", true).Order("created_at asc").First(&cluster).Error
	if err == nil {
		return cluster.ID
	}
	err = query.Order("created_at asc").First(&cluster).Error
	if err == nil {
		return cluster.ID
	}
	return ""
}

func canUseRuntimeClusterForProject(user model.User, cluster model.RuntimeCluster, projectID string, boundProjectIDs []string) bool {
	if authz.IsPlatformAdmin(user.Role) {
		return true
	}
	switch normalizeOwnerScope(cluster.Scope) {
	case "global":
		return true
	case "project":
		for _, boundProjectID := range boundProjectIDs {
			if boundProjectID == projectID {
				return true
			}
		}
	case "user":
		return cluster.OwnerRef == user.ID
	}
	return false
}

func deploymentTargetNamespace(project model.Project, target model.DeploymentTarget) string {
	return runtimeProjectNamespace(project)
}

func runtimeProjectNamespace(project model.Project) string {
	return strings.TrimSpace(project.KubernetesNamespace)
}

func (h *Handlers) runtimeClusterResponseForUser(user model.User, cluster model.RuntimeCluster, ctx context.Context) (model.RuntimeCluster, error) {
	cluster.ProjectIDs = h.scopedResourceProjectIDs(scopedResourceRuntimeCluster, cluster.ID, ctx)
	cluster.GatewayDomainSuffixes = runtimecluster.DecodeGatewayDomainSuffixes(cluster.GatewayDomainSuffixesRaw)
	cluster.KubeconfigSet = cluster.KubeconfigRef != ""
	cluster.Kubeconfig = ""
	canInspect, err := h.canInspectScopedResourceConfigByID(user, cluster.Scope, cluster.OwnerRef, scopedResourceRuntimeCluster, cluster.ID, ctx)
	if err != nil {
		return model.RuntimeCluster{}, err
	}
	if !canInspect {
		cluster.Endpoint = ""
	}
	return cluster, nil
}

func (h *Handlers) canReplaceRuntimeClusterKubeconfig(user model.User, cluster model.RuntimeCluster) bool {
	return user.Role == authz.PlatformRoleAdmin || cluster.CreatedBy == user.ID
}

func (h *Handlers) canInspectClusterResourceProject(ctx *gin.Context, user model.User, projectID string) bool {
	if user.Role == authz.PlatformRoleAdmin {
		return true
	}
	if _, ok := h.findProjectForCurrentUserByID(ctx, projectID); ok {
		return true
	}
	return false
}

func (h *Handlers) canInspectClusterResourceSnapshot(ctx *gin.Context, user model.User, item kubeprovider.ResourceSnapshot) bool {
	if user.Role == authz.PlatformRoleAdmin {
		return true
	}
	if strings.TrimSpace(item.ProjectID) == "" {
		writeError(ctx, http.StatusForbidden, "无权查看无项目空间归属的集群资源")
		return false
	}
	return h.canInspectClusterResourceProject(ctx, user, item.ProjectID)
}

func (h *Handlers) canManageClusterResourceSnapshot(ctx *gin.Context, user model.User, item kubeprovider.ResourceSnapshot) bool {
	if user.Role == authz.PlatformRoleAdmin {
		return true
	}
	if strings.TrimSpace(item.ProjectID) == "" {
		writeError(ctx, http.StatusForbidden, "无权维护无项目空间归属的集群资源")
		return false
	}
	if _, _, ok := h.authorizeProjectByID(ctx, item.ProjectID, authz.ActionClusterManage); ok {
		return true
	}
	return false
}

func (h *Handlers) filterClusterResourceSnapshots(ctx *gin.Context, user model.User, items []kubeprovider.ResourceSnapshot, visibility projectservice.ListVisibility, projectID string) []kubeprovider.ResourceSnapshot {
	projectID = strings.TrimSpace(projectID)
	if visibility == projectservice.ListVisibilityAll && projectID == "" {
		return items
	}
	allowed := make(map[string]bool)
	if projectID != "" {
		allowed[projectID] = true
	} else {
		for _, visibleProjectID := range h.projectIDsForUser(ctx.Request.Context(), user.ID) {
			allowed[visibleProjectID] = true
		}
	}
	filtered := make([]kubeprovider.ResourceSnapshot, 0, len(items))
	for _, item := range items {
		if allowed[strings.TrimSpace(item.ProjectID)] {
			filtered = append(filtered, item)
		}
	}
	return filtered
}
