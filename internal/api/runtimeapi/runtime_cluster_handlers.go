package runtimeapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/observation"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/runtimecluster"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *Handlers) ListRuntimeClusters(ctx *gin.Context) {
	markLiveObservationResponse(ctx)
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	visibility, ok := resolveListVisibility(ctx, user)
	if !ok {
		return
	}
	projectID := strings.TrimSpace(ctx.Query("projectId"))

	var clusters []model.RuntimeCluster
	query := h.dbFor(ctx).Model(&model.RuntimeCluster{})
	var visible bool
	query, visible = h.applyScopedResourceListVisibility(ctx, query, scopedResourceRuntimeCluster, user, projectID, visibility)
	if !visible {
		return
	}
	query = applyRuntimeClusterSearch(ctx, query, user)
	pagination := paginationFromQueryWithSort(ctx, map[string]string{
		"name": "name", "type": "type", "scope": "scope", "createdAt": "created_at",
	}, "createdAt")
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if err := query.Order(orderByClause(pagination, map[string]string{
		"name":      "name",
		"type":      "type",
		"scope":     "scope",
		"createdAt": "created_at",
	}, "created_at")).Limit(pagination.PageSize).Offset(pagination.Offset()).Find(&clusters).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	for index := range clusters {
		response, err := h.runtimeClusterResponseForUser(user, clusters[index], ctx.Request.Context())
		if err != nil {
			writeProjectAuthorizationError(ctx, err)
			return
		}
		clusters[index] = response
	}
	h.observeRuntimeClusters(ctx.Request.Context(), clusters)
	ctx.JSON(http.StatusOK, paginatedResponse(clusters, total, pagination))
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
	auditMetadata := runtimeClusterSafeAuditMetadata(cluster, strings.TrimSpace(input.Kubeconfig) != "")
	if err := h.saveRuntimeClusterWithDefault(cluster, ctx.Request.Context()); err != nil {
		auditWithSafeMetadata(h, user.ID, "runtime_cluster.create", cluster.ID, false, runtimeClusterPersistenceAuditMessage(err), auditMetadata, ctx.Request.Context())
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	auditWithSafeMetadata(h, user.ID, "runtime_cluster.create", cluster.ID, true, "", auditMetadata, ctx.Request.Context())
	cluster, err := h.runtimeClusterResponseForUser(user, cluster, ctx.Request.Context())
	if err != nil {
		writeProjectAuthorizationError(ctx, err)
		return
	}
	ctx.JSON(http.StatusCreated, h.observeRuntimeCluster(ctx.Request.Context(), cluster))
}

func (h *Handlers) UpdateRuntimeCluster(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var existing model.RuntimeCluster
	if err := h.dbFor(ctx).First(&existing, "id = ?", ctx.Param("clusterId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "runtime cluster not found")
		return
	}
	if !runtimecluster.IsActive(existing) {
		writeErrorCode(ctx, http.StatusConflict, "runtime_cluster.not_active", "运行集群不是活动状态，不能修改")
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
	existing.CPURequestPercent = next.CPURequestPercent
	existing.MemoryRequestPercent = next.MemoryRequestPercent
	existing.CPULimitPercent = next.CPULimitPercent
	existing.MemoryLimitPercent = next.MemoryLimitPercent
	existing.GatewayProvider = next.GatewayProvider
	existing.GatewayRootDomain = next.GatewayRootDomain
	existing.GatewayDomainSuffixesRaw = next.GatewayDomainSuffixesRaw
	existing.GatewayDomainSuffixes = next.GatewayDomainSuffixes
	existing.GatewayPublicScheme = next.GatewayPublicScheme
	existing.GatewayPublicPort = next.GatewayPublicPort
	existing.GatewayControllerType = next.GatewayControllerType
	existing.GatewayClassName = next.GatewayClassName
	existing.GatewayName = next.GatewayName
	existing.GatewayNamespace = next.GatewayNamespace
	existing.GatewayHTTPListenerName = next.GatewayHTTPListenerName
	existing.GatewayHTTPListenerPort = next.GatewayHTTPListenerPort
	existing.GatewayHTTPSListenerName = next.GatewayHTTPSListenerName
	existing.GatewayHTTPSListenerPort = next.GatewayHTTPSListenerPort
	existing.GatewayTLSSecretName = next.GatewayTLSSecretName
	existing.GatewayTLSSecretNamespace = next.GatewayTLSSecretNamespace
	existing.GatewayCertIssuerKind = next.GatewayCertIssuerKind
	existing.GatewayCertIssuerName = next.GatewayCertIssuerName
	existing.GatewayCertificateNamespace = next.GatewayCertificateNamespace
	existing.GatewayWildcardCertEnabled = next.GatewayWildcardCertEnabled
	existing.GatewayWildcardCertDomain = next.GatewayWildcardCertDomain
	existing.GatewayWildcardCertSecretName = next.GatewayWildcardCertSecretName
	existing.GatewayExternalTLSMode = next.GatewayExternalTLSMode
	existing.GatewayForwardedHeadersMode = next.GatewayForwardedHeadersMode
	existing.GatewayTrustedProxyCIDRs = next.GatewayTrustedProxyCIDRs
	existing.GatewayDefaultRequestHeaders = next.GatewayDefaultRequestHeaders
	existing.GatewayDefaultResponseHeaders = next.GatewayDefaultResponseHeaders
	auditMetadata := runtimeClusterSafeAuditMetadata(existing, strings.TrimSpace(input.Kubeconfig) != "")
	if err := h.saveRuntimeClusterWithDefault(existing, ctx.Request.Context()); err != nil {
		auditWithSafeMetadata(h, user.ID, "runtime_cluster.update", existing.ID, false, runtimeClusterPersistenceAuditMessage(err), auditMetadata, ctx.Request.Context())
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	auditWithSafeMetadata(h, user.ID, "runtime_cluster.update", existing.ID, true, "", auditMetadata, ctx.Request.Context())
	existing, err := h.runtimeClusterResponseForUser(user, existing, ctx.Request.Context())
	if err != nil {
		writeProjectAuthorizationError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, h.observeRuntimeCluster(ctx.Request.Context(), existing))
}

func applyRuntimeClusterSearch(ctx *gin.Context, query *gorm.DB, user model.User) *gorm.DB {
	keyword := strings.TrimSpace(ctx.Query("search"))
	if keyword == "" {
		return query
	}
	escaped := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(keyword)
	pattern := "%" + escaped + "%"
	nameCondition := "LOWER(runtime_clusters.name) LIKE LOWER(?) ESCAPE '\\'"
	endpointCondition := "LOWER(runtime_clusters.endpoint) LIKE LOWER(?) ESCAPE '\\'"
	if authz.IsPlatformAdmin(user.Role) {
		return query.Where("("+nameCondition+" OR "+endpointCondition+")", pattern, pattern)
	}

	managePolicy, ok := authz.ProjectPolicyForAction(authz.ActionProjectManage)
	if !ok {
		return query.Where(nameCondition, pattern)
	}
	inspectableCondition := `(runtime_clusters.scope = 'user' AND runtime_clusters.owner_ref = ?) OR (` +
		`runtime_clusters.scope = 'project' AND EXISTS (` +
		`SELECT 1 FROM scoped_resource_project_bindings AS runtime_cluster_bindings ` +
		`JOIN project_members AS runtime_cluster_members ON runtime_cluster_members.project_id = runtime_cluster_bindings.project_id ` +
		`WHERE runtime_cluster_bindings.resource_type = ? AND runtime_cluster_bindings.resource_id = runtime_clusters.id ` +
		`AND runtime_cluster_members.user_id = ? AND runtime_cluster_members.role IN ?))`
	return query.Where("("+nameCondition+" OR (("+inspectableCondition+") AND "+endpointCondition+"))",
		pattern, user.ID, scopedResourceRuntimeCluster, user.ID, managePolicy.AllowedRoles, pattern)
}

func runtimeClusterSafeAuditMetadata(cluster model.RuntimeCluster, kubeconfigUpdated bool) runtimeClusterAuditMetadata {
	return runtimeClusterAuditMetadata{
		Type: cluster.Type, Scope: cluster.Scope, IsDefault: cluster.IsDefault,
		ProjectCount: len(normalizeStringList(cluster.ProjectIDs)), KubeconfigUpdated: kubeconfigUpdated,
	}
}

func runtimeClusterPersistenceAuditMessage(_ error) string {
	return "persist_failed"
}

func (h *Handlers) DeleteRuntimeCluster(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var cluster model.RuntimeCluster
	if err := h.dbFor(ctx).First(&cluster, "id = ?", ctx.Param("clusterId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "runtime cluster not found")
		return
	}
	if !h.canManageScopedResourceByID(ctx, user, cluster.Scope, cluster.OwnerRef, scopedResourceRuntimeCluster, cluster.ID, "无权维护该运行集群") {
		return
	}
	var targetCount int64
	if err := h.dbFor(ctx).Model(&model.DeploymentTarget{}).Where("cluster_id = ?", cluster.ID).Count(&targetCount).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if targetCount > 0 {
		writeError(ctx, http.StatusConflict, "运行集群仍被部署配置引用，请先迁移或删除相关部署配置")
		return
	}
	if !h.deleteStatusCanStart(cluster.DeleteStatus) {
		writeErrorCode(ctx, http.StatusConflict, "runtime_cluster.delete_in_progress", "运行集群正在删除中，请等待资源清理完成")
		return
	}
	if err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		return h.markResourceDeleting(tx, &model.RuntimeCluster{}, cluster.ID)
	}); err != nil {
		if h.host.ResourceDeleteAlreadyStarted(err) {
			writeErrorCode(ctx, http.StatusConflict, "runtime_cluster.delete_in_progress", "运行集群正在删除中，请等待资源清理完成")
			return
		}
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if !h.enqueueResourceCleanup(ctx.Request.Context(), "runtime_cluster", cluster.ID, "", user.ID) {
		_ = h.markResourceDeleteFailed(h.dbFor(ctx), &model.RuntimeCluster{}, cluster.ID, "资源清理任务投递失败，请稍后重试")
		h.auditWithContext(user.ID, "runtime_cluster.delete", cluster.ID, false, "cleanup_enqueue_failed", ctx.Request.Context())
		writeErrorCode(ctx, http.StatusServiceUnavailable, "runtime_cluster.cleanup_enqueue_failed", "运行集群清理任务投递失败，请稍后重试")
		return
	}
	h.auditWithContext(user.ID, "runtime_cluster.delete", cluster.ID, true, "cleanup_queued", ctx.Request.Context())
	ctx.Status(http.StatusNoContent)
}

func (h *Handlers) TestRuntimeCluster(ctx *gin.Context) {
	markLiveObservationResponse(ctx)
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var cluster model.RuntimeCluster
	if err := runtimecluster.ActiveScope(h.dbFor(ctx)).First(&cluster, "id = ?", ctx.Param("clusterId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "runtime cluster not found")
		return
	}
	if !h.canUseScopedResourceByID(user, cluster.Scope, cluster.OwnerRef, scopedResourceRuntimeCluster, cluster.ID, ctx.Request.Context()) {
		writeErrorCode(ctx, http.StatusForbidden, "runtime_cluster.forbidden", "无权测试该运行集群")
		return
	}
	cluster, err := h.runtimeClusterResponseForUser(user, cluster, ctx.Request.Context())
	if err != nil {
		writeProjectAuthorizationError(ctx, err)
		return
	}
	cluster = h.observeRuntimeCluster(ctx.Request.Context(), cluster)
	switch cluster.Status {
	case observation.StatusNotConfigured:
		writeErrorCode(ctx, http.StatusBadRequest, cluster.ObservationCode, "运行集群缺少可用的 kubeconfig")
	case observation.StatusUnavailable:
		writeErrorCode(ctx, http.StatusBadGateway, cluster.ObservationCode, "无法连接运行集群")
	default:
		ctx.JSON(http.StatusOK, cluster)
	}
}

func (h *Handlers) DeleteRuntimeClusterResource(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var cluster model.RuntimeCluster
	if err := runtimecluster.ActiveScope(h.dbFor(ctx)).First(&cluster, "id = ?", ctx.Param("clusterId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "runtime cluster not found")
		return
	}
	if !h.canManageScopedResourceByID(ctx, user, cluster.Scope, cluster.OwnerRef, scopedResourceRuntimeCluster, cluster.ID, "无权维护该集群资源") {
		return
	}
	kubeconfig := h.secrets.ResolveContext(ctx.Request.Context(), cluster.KubeconfigRef)
	if strings.TrimSpace(kubeconfig) == "" {
		writeError(ctx, http.StatusBadRequest, "运行集群缺少 kubeconfig，无法维护资源")
		return
	}
	client, err := kubeprovider.NewClientFromKubeconfig(kubeconfig)
	if err != nil {
		writeError(ctx, http.StatusBadRequest, "运行集群 kubeconfig 无效")
		return
	}
	kind := strings.TrimSpace(ctx.Query("resourceKind"))
	namespace := strings.TrimSpace(ctx.Query("namespace"))
	name := strings.TrimSpace(ctx.Query("name"))
	if !validRuntimeResourceKind(kind) {
		writeRuntimeResourceArgumentError(ctx, "cluster.resource_kind_invalid", "resourceKind", runtimeResourceKinds)
		return
	}
	if name == "" {
		writeErrorCode(ctx, http.StatusBadRequest, "cluster.resource_name_required", "resource name is required")
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
	h.auditWithContext(user.ID, "runtime_cluster_resource.delete", cluster.ID, true, strings.Join([]string{kind, namespace, name}, "/"), ctx.Request.Context())
	ctx.Status(http.StatusNoContent)
}
