package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/gin-gonic/gin"
)

func (h *Handlers) ListRuntimeClusterResources(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var cluster model.RuntimeCluster
	if err := h.db.First(&cluster, "id = ?", ctx.Param("clusterId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "runtime cluster not found")
		return
	}
	if !h.canManageScopedResourceByID(ctx, user, cluster.Scope, cluster.OwnerRef, scopedResourceRuntimeCluster, cluster.ID, "无权查看该集群资源") {
		return
	}
	kubeconfig := h.secrets.Resolve(cluster.KubeconfigRef)
	if strings.TrimSpace(kubeconfig) == "" {
		writeError(ctx, http.StatusBadRequest, "运行集群缺少 kubeconfig，无法读取资源")
		return
	}
	client, err := kubeprovider.NewClientFromKubeconfig(kubeconfig)
	if err != nil {
		writeError(ctx, http.StatusBadRequest, "运行集群 kubeconfig 无效")
		return
	}
	options := kubeprovider.ResourceListOptions{
		Kind:          strings.TrimSpace(ctx.Query("kind")),
		Namespace:     strings.TrimSpace(ctx.Query("namespace")),
		ProjectID:     strings.TrimSpace(ctx.Query("projectId")),
		ApplicationID: strings.TrimSpace(ctx.Query("applicationId")),
		EnvironmentID: strings.TrimSpace(ctx.Query("environmentId")),
	}
	if options.ProjectID != "" && !h.canInspectClusterResourceProject(ctx, user, options.ProjectID) {
		return
	}
	requestCtx, cancel := context.WithTimeout(ctx.Request.Context(), 10*time.Second)
	defer cancel()
	items, err := client.ListManagedResources(requestCtx, options)
	if err != nil {
		writeError(ctx, http.StatusBadGateway, "集群资源读取失败，请检查集群连接和权限")
		return
	}
	items = h.filterClusterResourceSnapshots(ctx, user, items)
	responses, err := h.clusterResourceResponses(items)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if paginationRequested(ctx) {
		pagination := paginationFromQuery(ctx)
		pagination.SortBy = normalizeClusterResourceSortBy(pagination.SortBy)
		sortClusterResourceResponses(responses, pagination)
		ctx.JSON(http.StatusOK, paginatedResponse(paginateSlice(responses, pagination), int64(len(responses)), pagination))
		return
	}
	ctx.JSON(http.StatusOK, responses)
}

func (h *Handlers) GetRuntimeClusterResourceYAML(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var cluster model.RuntimeCluster
	if err := h.db.First(&cluster, "id = ?", ctx.Param("clusterId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "runtime cluster not found")
		return
	}
	if !h.canManageScopedResourceByID(ctx, user, cluster.Scope, cluster.OwnerRef, scopedResourceRuntimeCluster, cluster.ID, "无权查看该集群资源") {
		return
	}
	kubeconfig := h.secrets.Resolve(cluster.KubeconfigRef)
	if strings.TrimSpace(kubeconfig) == "" {
		writeError(ctx, http.StatusBadRequest, "运行集群缺少 kubeconfig，无法读取资源 YAML")
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
	content, snapshot, err := client.GetManagedResourceYAML(requestCtx, kind, namespace, name)
	if err != nil {
		writeError(ctx, http.StatusBadGateway, "集群资源 YAML 读取失败，请确认资源仍存在且归属平台管理")
		return
	}
	if !h.canInspectClusterResourceSnapshot(ctx, user, snapshot) {
		return
	}
	ctx.JSON(http.StatusOK, clusterResourceYAMLResponse{YAML: content})
}

func (h *Handlers) ListRuntimeClusterResourceEvents(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var cluster model.RuntimeCluster
	if err := h.db.First(&cluster, "id = ?", ctx.Param("clusterId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "runtime cluster not found")
		return
	}
	if !h.canManageScopedResourceByID(ctx, user, cluster.Scope, cluster.OwnerRef, scopedResourceRuntimeCluster, cluster.ID, "无权查看该集群资源") {
		return
	}
	kubeconfig := h.secrets.Resolve(cluster.KubeconfigRef)
	if strings.TrimSpace(kubeconfig) == "" {
		writeError(ctx, http.StatusBadRequest, "运行集群缺少 kubeconfig，无法读取资源事件")
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
	events, snapshot, err := client.ListManagedResourceEvents(requestCtx, kind, namespace, name)
	if err != nil {
		writeError(ctx, http.StatusBadGateway, "集群资源事件读取失败，请确认资源仍存在且归属平台管理")
		return
	}
	if !h.canInspectClusterResourceSnapshot(ctx, user, snapshot) {
		return
	}
	ctx.JSON(http.StatusOK, events)
}
