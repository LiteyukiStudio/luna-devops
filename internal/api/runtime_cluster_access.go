package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *Handlers) runtimeClusterForEnvironment(ctx *gin.Context, environment model.Environment) (model.RuntimeCluster, bool) {
	var cluster model.RuntimeCluster
	if clusterID := strings.TrimSpace(environment.ClusterID); clusterID != "" {
		err := h.db.First(&cluster, "id = ? and type in ?", clusterID, []string{"kubernetes", "k3s"}).Error
		if err != nil {
			writeError(ctx, http.StatusNotFound, "runtime cluster not found")
			return cluster, false
		}
		return cluster, true
	}
	err := h.db.Where("scope = ? and is_default = ? and type in ?", "global", true, []string{"kubernetes", "k3s"}).First(&cluster).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = h.db.Where("scope = ? and type in ?", "global", []string{"kubernetes", "k3s"}).Order("created_at asc").First(&cluster).Error
	}
	if err != nil {
		writeError(ctx, http.StatusNotFound, "runtime cluster not found")
		return cluster, false
	}
	return cluster, true
}

func (h *Handlers) runtimeClusterForDeploymentTarget(ctx *gin.Context, target model.DeploymentTarget) (model.RuntimeCluster, bool) {
	var cluster model.RuntimeCluster
	if clusterID := strings.TrimSpace(target.ClusterID); clusterID != "" {
		err := h.db.First(&cluster, "id = ? and type in ?", clusterID, []string{"kubernetes", "k3s"}).Error
		if err != nil {
			writeError(ctx, http.StatusNotFound, "runtime cluster not found")
			return cluster, false
		}
		return cluster, true
	}
	err := h.db.Where("scope = ? and is_default = ? and type in ?", "global", true, []string{"kubernetes", "k3s"}).First(&cluster).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = h.db.Where("scope = ? and type in ?", "global", []string{"kubernetes", "k3s"}).Order("created_at asc").First(&cluster).Error
	}
	if err != nil {
		writeError(ctx, http.StatusNotFound, "runtime cluster not found")
		return cluster, false
	}
	return cluster, true
}

func deploymentTargetNamespace(project model.Project, target model.DeploymentTarget) string {
	if namespace := strings.TrimSpace(target.Namespace); namespace != "" {
		return namespace
	}
	return runtimeProjectNamespace(project)
}

func (h *Handlers) runtimeClusterResponseForUser(user model.User, cluster model.RuntimeCluster) model.RuntimeCluster {
	cluster.ProjectIDs = h.scopedResourceProjectIDs(scopedResourceRuntimeCluster, cluster.ID)
	cluster.KubeconfigSet = cluster.KubeconfigRef != ""
	cluster.Kubeconfig = ""
	if !h.canInspectScopedResourceConfigByID(user, cluster.Scope, cluster.OwnerRef, scopedResourceRuntimeCluster, cluster.ID) {
		cluster.Endpoint = ""
	}
	return cluster
}

func (h *Handlers) canReplaceRuntimeClusterKubeconfig(user model.User, cluster model.RuntimeCluster) bool {
	return user.Role == "platform_admin" || cluster.CreatedBy == user.ID
}

func (h *Handlers) canInspectClusterResourceProject(ctx *gin.Context, user model.User, projectID string) bool {
	if user.Role == "platform_admin" {
		return true
	}
	if _, ok := h.findProjectForCurrentUserWithRolesByID(ctx, projectID, "owner", "admin"); ok {
		return true
	}
	return false
}

func (h *Handlers) canInspectClusterResourceSnapshot(ctx *gin.Context, user model.User, item kubeprovider.ResourceSnapshot) bool {
	if user.Role == "platform_admin" {
		return true
	}
	if strings.TrimSpace(item.ProjectID) == "" {
		writeError(ctx, http.StatusForbidden, "无权查看无项目空间归属的集群资源")
		return false
	}
	return h.canInspectClusterResourceProject(ctx, user, item.ProjectID)
}

func (h *Handlers) canManageClusterResourceSnapshot(ctx *gin.Context, user model.User, item kubeprovider.ResourceSnapshot) bool {
	if user.Role == "platform_admin" {
		return true
	}
	if strings.TrimSpace(item.ProjectID) == "" {
		writeError(ctx, http.StatusForbidden, "无权维护无项目空间归属的集群资源")
		return false
	}
	if _, ok := h.findProjectForCurrentUserWithRolesByID(ctx, item.ProjectID, "owner", "admin"); ok {
		return true
	}
	return false
}

func (h *Handlers) filterClusterResourceSnapshots(ctx *gin.Context, user model.User, items []kubeprovider.ResourceSnapshot) []kubeprovider.ResourceSnapshot {
	if user.Role == "platform_admin" {
		return items
	}
	allowed := make(map[string]bool)
	filtered := make([]kubeprovider.ResourceSnapshot, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.ProjectID) == "" {
			continue
		}
		allowedProject, ok := allowed[item.ProjectID]
		if !ok {
			_, projectOK := h.findProjectForCurrentUserWithRolesByID(ctx, item.ProjectID, "owner", "admin")
			allowedProject = projectOK
			allowed[item.ProjectID] = projectOK
		}
		if allowedProject {
			filtered = append(filtered, item)
		}
	}
	return filtered
}
