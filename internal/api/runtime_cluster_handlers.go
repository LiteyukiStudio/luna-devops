package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *Handlers) ListRuntimeClusters(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	projectID := strings.TrimSpace(ctx.Query("projectId"))

	var clusters []model.RuntimeCluster
	query := h.db.Order("is_default desc, created_at desc")
	var visible bool
	query, visible = h.applyScopedResourceVisibility(ctx, query, scopedResourceRuntimeCluster, user, projectID)
	if !visible {
		return
	}
	if err := applySearch(ctx, query, "name", "endpoint").Find(&clusters).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	for index := range clusters {
		clusters[index] = h.runtimeClusterResponseForUser(user, clusters[index])
	}
	ctx.JSON(http.StatusOK, clusters)
}

func (h *Handlers) CreateRuntimeCluster(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var input runtimeClusterInput
	if !bindJSON(ctx, &input) {
		return
	}
	clusterID := id.New("clu")
	cluster, ok := h.runtimeClusterFromInput(ctx, user, input, clusterID)
	if !ok {
		return
	}
	if err := h.saveRuntimeClusterWithDefault(cluster); err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	ctx.JSON(http.StatusCreated, h.runtimeClusterResponseForUser(user, cluster))
}

func (h *Handlers) UpdateRuntimeCluster(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var existing model.RuntimeCluster
	if err := h.db.First(&existing, "id = ?", ctx.Param("clusterId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "runtime cluster not found")
		return
	}
	if !h.canManageScopedResourceByID(ctx, user, existing.Scope, existing.OwnerRef, scopedResourceRuntimeCluster, existing.ID, "无权维护该运行集群") {
		return
	}
	var input runtimeClusterInput
	if !bindJSON(ctx, &input) {
		return
	}
	if strings.TrimSpace(input.Kubeconfig) != "" && !h.canReplaceRuntimeClusterKubeconfig(user, existing) {
		writeError(ctx, http.StatusForbidden, "只有创建者或平台管理员可以替换 kubeconfig")
		return
	}
	next, ok := h.runtimeClusterFromInput(ctx, user, input, existing.ID)
	if !ok {
		return
	}
	existing.Name = next.Name
	existing.Type = next.Type
	existing.Endpoint = next.Endpoint
	existing.Scope = next.Scope
	existing.OwnerRef = next.OwnerRef
	existing.ProjectIDs = next.ProjectIDs
	if next.KubeconfigRef != "" {
		existing.KubeconfigRef = next.KubeconfigRef
	}
	existing.IsDefault = next.IsDefault
	existing.MaxConcurrentBuilds = next.MaxConcurrentBuilds
	existing.GatewayRootDomain = next.GatewayRootDomain
	existing.GatewayPublicScheme = next.GatewayPublicScheme
	existing.Status = next.Status
	if err := h.saveRuntimeClusterWithDefault(existing); err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, h.runtimeClusterResponseForUser(user, existing))
}

func (h *Handlers) DeleteRuntimeCluster(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var cluster model.RuntimeCluster
	if err := h.db.First(&cluster, "id = ?", ctx.Param("clusterId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "runtime cluster not found")
		return
	}
	if !h.canManageScopedResourceByID(ctx, user, cluster.Scope, cluster.OwnerRef, scopedResourceRuntimeCluster, cluster.ID, "无权维护该运行集群") {
		return
	}
	var targetCount int64
	if err := h.db.Model(&model.DeploymentTarget{}).Where("cluster_id = ?", cluster.ID).Count(&targetCount).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if targetCount > 0 {
		writeError(ctx, http.StatusConflict, "运行集群仍被部署配置引用，请先迁移或删除相关部署配置")
		return
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("resource_type = ? and resource_id = ?", scopedResourceRuntimeCluster, cluster.ID).Delete(&model.ScopedResourceProjectBinding{}).Error; err != nil {
			return err
		}
		return tx.Delete(&cluster).Error
	}); err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (h *Handlers) TestRuntimeCluster(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var cluster model.RuntimeCluster
	if err := h.db.First(&cluster, "id = ?", ctx.Param("clusterId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "runtime cluster not found")
		return
	}
	if !h.canManageScopedResourceByID(ctx, user, cluster.Scope, cluster.OwnerRef, scopedResourceRuntimeCluster, cluster.ID, "无权测试该运行集群") {
		return
	}
	now := time.Now()
	cluster.LastCheckedAt = &now
	if cluster.KubeconfigRef == "" {
		cluster.Status = "missing-credential"
		if err := h.db.Save(&cluster).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		}
		writeError(ctx, http.StatusBadRequest, "运行集群缺少 kubeconfig，无法测试连接")
		return
	}
	kubeconfig := h.secrets.Resolve(cluster.KubeconfigRef)
	if strings.TrimSpace(kubeconfig) == "" {
		cluster.Status = "missing-credential"
		if err := h.db.Save(&cluster).Error; err != nil {
			writeError(ctx, http.StatusInternalServerError, err.Error())
			return
		}
		writeError(ctx, http.StatusBadRequest, "运行集群缺少 kubeconfig，无法测试连接")
		return
	}
	client, err := kubeprovider.NewClientFromKubeconfig(kubeconfig)
	if err != nil {
		cluster.Status = "unhealthy"
		if saveErr := h.db.Save(&cluster).Error; saveErr != nil {
			writeError(ctx, http.StatusInternalServerError, saveErr.Error())
			return
		}
		writeError(ctx, http.StatusBadRequest, "运行集群 kubeconfig 无效")
		return
	}
	pingCtx, cancel := context.WithTimeout(ctx.Request.Context(), 8*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx); err != nil {
		cluster.Status = "unhealthy"
		if saveErr := h.db.Save(&cluster).Error; saveErr != nil {
			writeError(ctx, http.StatusInternalServerError, saveErr.Error())
			return
		}
		writeError(ctx, http.StatusBadGateway, "运行集群连接测试失败，请检查 kubeconfig、集群地址和网络连通性")
		return
	}
	cluster.Status = "ready"
	if err := h.db.Save(&cluster).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, h.runtimeClusterResponseForUser(user, cluster))
}

func (h *Handlers) DeleteRuntimeClusterResource(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var cluster model.RuntimeCluster
	if err := h.db.First(&cluster, "id = ?", ctx.Param("clusterId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "runtime cluster not found")
		return
	}
	if !h.canManageScopedResourceByID(ctx, user, cluster.Scope, cluster.OwnerRef, scopedResourceRuntimeCluster, cluster.ID, "无权维护该集群资源") {
		return
	}
	kubeconfig := h.secrets.Resolve(cluster.KubeconfigRef)
	if strings.TrimSpace(kubeconfig) == "" {
		writeError(ctx, http.StatusBadRequest, "运行集群缺少 kubeconfig，无法维护资源")
		return
	}
	client, err := kubeprovider.NewClientFromKubeconfig(kubeconfig)
	if err != nil {
		writeError(ctx, http.StatusBadRequest, "运行集群 kubeconfig 无效")
		return
	}
	kind := strings.TrimSpace(ctx.Query("kind"))
	namespace := strings.TrimSpace(ctx.Query("namespace"))
	name := strings.TrimSpace(ctx.Query("name"))
	if kind == "" || name == "" {
		writeError(ctx, http.StatusBadRequest, "资源类型和名称不能为空")
		return
	}
	requestCtx, cancel := context.WithTimeout(ctx.Request.Context(), 10*time.Second)
	defer cancel()
	snapshot, err := client.GetManagedResource(requestCtx, kind, namespace, name)
	if err != nil {
		writeError(ctx, http.StatusBadGateway, "集群资源读取失败，请确认资源仍存在且归属平台管理")
		return
	}
	if !h.canManageClusterResourceSnapshot(ctx, user, snapshot) {
		return
	}
	if err := client.DeleteManagedResource(requestCtx, kind, namespace, name); err != nil {
		writeError(ctx, http.StatusBadGateway, "集群资源删除失败，请确认资源仍存在且归属平台管理")
		return
	}
	h.audit(user.ID, "runtime_cluster_resource.delete", cluster.ID, true, strings.Join([]string{kind, namespace, name}, "/"))
	ctx.Status(http.StatusNoContent)
}
