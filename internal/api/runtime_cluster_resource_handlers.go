package api

import (
	"context"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func (h *Handlers) ListRuntimeClusterResources(ctx *gin.Context) {
	markLiveObservationResponse(ctx)
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var cluster model.RuntimeCluster
	if err := h.dbFor(ctx).First(&cluster, "id = ?", ctx.Param("clusterId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "runtime cluster not found")
		return
	}
	if !h.canUseScopedResourceByID(user, cluster.Scope, cluster.OwnerRef, scopedResourceRuntimeCluster, cluster.ID, ctx.Request.Context()) {
		writeErrorCode(ctx, http.StatusForbidden, "runtime_cluster.forbidden", "无权查看该集群资源")
		return
	}
	pagination := paginationFromQuery(ctx)
	if pagination.Page != 1 {
		writeErrorCode(ctx, http.StatusBadRequest, "pagination.cursor_required", "runtime cluster resources only support the first bounded page")
		return
	}
	kubeconfig := h.secrets.ResolveContext(ctx.Request.Context(), cluster.KubeconfigRef)
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
		Kind:          strings.TrimSpace(ctx.Query("resourceCategory")),
		Namespace:     strings.TrimSpace(ctx.Query("namespace")),
		ProjectID:     strings.TrimSpace(ctx.Query("projectId")),
		ApplicationID: strings.TrimSpace(ctx.Query("applicationId")),
		EnvironmentID: strings.TrimSpace(ctx.Query("environmentId")),
		Limit:         int64(pagination.PageSize),
	}
	if !validRuntimeResourceCategory(options.Kind) {
		writeRuntimeResourceArgumentError(ctx, "cluster.resource_category_invalid", "resourceCategory", runtimeResourceCategories)
		return
	}
	if options.ProjectID != "" && !h.canInspectClusterResourceProject(ctx, user, options.ProjectID) {
		return
	}
	requestCtx, cancel := context.WithTimeout(ctx.Request.Context(), 10*time.Second)
	defer cancel()
	page, err := client.ListManagedResourcesPage(requestCtx, options)
	if err != nil {
		writeError(ctx, http.StatusBadGateway, "集群资源读取失败，请检查集群连接和权限")
		return
	}
	items := h.filterClusterResourceSnapshots(ctx, user, page.Items)
	responses, err := h.clusterResourceResponses(items, ctx.Request.Context())
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if isWorkloadResourceKind(options.Kind) {
		responses = groupWorkloadPodResponses(responses)
	}
	pagination.SortBy = normalizeClusterResourceSortBy(pagination.SortBy)
	sortClusterResourceResponses(responses, pagination)
	pageItems := paginateSlice(responses, pagination)
	// A resource category fans out to several Kubernetes kinds, each with an
	// independent continue token. A numeric global page cannot preserve the
	// requested cross-kind sort without draining every kind. Keep this endpoint
	// to one bounded page and do not advertise inaccessible remaining pages.
	ctx.JSON(http.StatusOK, paginatedResponse(pageItems, int64(len(pageItems)), pagination))
}

func (h *Handlers) GetRuntimeClusterResourceYAML(ctx *gin.Context) {
	markLiveObservationResponse(ctx)
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var cluster model.RuntimeCluster
	if err := h.dbFor(ctx).First(&cluster, "id = ?", ctx.Param("clusterId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "runtime cluster not found")
		return
	}
	if !h.canUseScopedResourceByID(user, cluster.Scope, cluster.OwnerRef, scopedResourceRuntimeCluster, cluster.ID, ctx.Request.Context()) {
		writeErrorCode(ctx, http.StatusForbidden, "runtime_cluster.forbidden", "无权查看该集群资源")
		return
	}
	kubeconfig := h.secrets.ResolveContext(ctx.Request.Context(), cluster.KubeconfigRef)
	if strings.TrimSpace(kubeconfig) == "" {
		writeError(ctx, http.StatusBadRequest, "运行集群缺少 kubeconfig，无法读取资源 YAML")
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
	markLiveObservationResponse(ctx)
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var cluster model.RuntimeCluster
	if err := h.dbFor(ctx).First(&cluster, "id = ?", ctx.Param("clusterId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "runtime cluster not found")
		return
	}
	if !h.canUseScopedResourceByID(user, cluster.Scope, cluster.OwnerRef, scopedResourceRuntimeCluster, cluster.ID, ctx.Request.Context()) {
		writeErrorCode(ctx, http.StatusForbidden, "runtime_cluster.forbidden", "无权查看该集群资源")
		return
	}
	pagination := paginationFromQueryWithSort(ctx, map[string]string{"lastSeen": "last_seen"}, "lastSeen")
	if pagination.Page != 1 {
		writeErrorCode(ctx, http.StatusBadRequest, "pagination.cursor_required", "runtime cluster resource events only support the first bounded page")
		return
	}
	kubeconfig := h.secrets.ResolveContext(ctx.Request.Context(), cluster.KubeconfigRef)
	if strings.TrimSpace(kubeconfig) == "" {
		writeError(ctx, http.StatusBadRequest, "运行集群缺少 kubeconfig，无法读取资源事件")
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
	page, snapshot, err := client.ListManagedResourceEventsPage(requestCtx, kind, namespace, name, int64(pagination.PageSize))
	if err != nil {
		writeError(ctx, http.StatusBadGateway, "集群资源事件读取失败，请确认资源仍存在且归属平台管理")
		return
	}
	if !h.canInspectClusterResourceSnapshot(ctx, user, snapshot) {
		return
	}
	sort.Slice(page.Items, func(left, right int) bool {
		if pagination.SortOrder == "asc" {
			return page.Items[left].LastSeen.Before(page.Items[right].LastSeen)
		}
		return page.Items[left].LastSeen.After(page.Items[right].LastSeen)
	})
	// Kubernetes event continuation follows API-server key order, which is not
	// compatible with this endpoint's lastSeen ordering. Keep one bounded page
	// and avoid claiming that a globally sorted next page is accessible.
	ctx.JSON(http.StatusOK, paginatedResponse(page.Items, int64(len(page.Items)), pagination))
}

func (h *Handlers) StreamRuntimeClusterPodTerminal(ctx *gin.Context) {
	ticket := strings.TrimSpace(ctx.Query("ticket"))
	var (
		user          model.User
		authorization runtimeTerminalAuthorizationBinding
		ticketValue   runtimeTerminalTicketValue
		ok            bool
	)
	if ticket == "" {
		user, ok = h.currentUser(ctx)
		if !ok {
			return
		}
	} else {
		var err error
		ticketValue, ok, err = h.consumeRuntimeTerminalTicket(ctx.Request.Context(), ticket)
		if err != nil {
			writeErrorCode(ctx, http.StatusServiceUnavailable, "runtime_terminal.ticket_unavailable", "terminal authorization is temporarily unavailable")
			return
		}
		if !ok {
			writeErrorCode(ctx, http.StatusUnauthorized, "runtime_terminal.ticket_invalid", "terminal ticket is invalid, expired, or already consumed")
			return
		}
		if err := h.dbFor(ctx).First(&user, "id = ? and disabled = ?", ticketValue.UserID, false).Error; err != nil {
			writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.account.disabled")
			return
		}
		authorization = ticketValue.Authorization
	}
	if user.Role != authz.PlatformRoleAdmin {
		writeError(ctx, http.StatusForbidden, "只有平台管理员可以打开集群 Pod 终端")
		return
	}
	cluster, client, snapshot, ok := h.runtimeClusterPodTerminalTarget(ctx, user)
	if !ok {
		return
	}
	reference := runtimeClusterPodTerminalReference(cluster, snapshot)
	if ticket == "" {
		authorization, ok = h.requireRuntimeTerminalAuthorization(ctx, user)
		if !ok {
			return
		}
	} else {
		if !ticketValue.matches("runtime_pod", reference) ||
			!h.runtimeTerminalAuthorizationActive(ctx.Request.Context(), authorization, func(checkCtx context.Context, currentUser model.User) bool {
				return h.runtimeClusterPodTerminalAuthorizationAllowed(checkCtx, currentUser, client, reference)
			}) {
			writeErrorCode(ctx, http.StatusUnauthorized, "runtime_terminal.ticket_invalid", "terminal ticket is invalid, expired, revoked, or bound to another resource")
			return
		}
	}
	upgrader := websocket.Upgrader{
		CheckOrigin: func(request *http.Request) bool {
			origin := strings.TrimSpace(request.Header.Get("Origin"))
			if origin == "" {
				return true
			}
			return containsString(configuredAllowedOrigins(), origin)
		},
	}
	conn, err := upgrader.Upgrade(ctx.Writer, ctx.Request, nil)
	if err != nil {
		h.auditWithContext(user.ID, "runtime_cluster.pod_terminal", cluster.ID+":"+snapshot.Namespace+"/"+snapshot.Name, false, err.Error(), ctx.Request.Context())
		return
	}
	defer conn.Close()

	sessionCtx, cancel := context.WithDeadline(ctx.Request.Context(), authorization.Deadline)
	defer cancel()
	stdinReader, stdinWriter := io.Pipe()
	defer stdinReader.Close()
	defer stdinWriter.Close()
	sizeQueue := newRuntimeTerminalSizeQueue()
	wsWriter := &runtimeTerminalWebSocketWriter{conn: conn}
	authorizationRevoked := h.monitorRuntimeTerminalAuthorization(sessionCtx, authorization, func(checkCtx context.Context, currentUser model.User) bool {
		return h.runtimeClusterPodTerminalAuthorizationAllowed(checkCtx, currentUser, client, reference)
	}, cancel)
	activityTracker := h.newRuntimeTerminalActivityTracker(authorization)

	go h.readRuntimeTerminalMessages(sessionCtx, conn, stdinWriter, sizeQueue, activityTracker, cancel)
	err = client.PodTerminal(sessionCtx, kubeprovider.PodTerminalOptions{
		Namespace: snapshot.Namespace,
		PodName:   snapshot.Name,
		Container: strings.TrimSpace(ctx.Query("container")),
		Stdin:     stdinReader,
		Stdout:    wsWriter,
		SizeQueue: sizeQueue,
	})
	resourceID := cluster.ID + ":" + snapshot.Namespace + "/" + snapshot.Name
	if err != nil && sessionCtx.Err() == nil {
		_, _ = wsWriter.Write(terminalDisconnectedMessage(ctx, err.Error()))
		h.auditWithContext(user.ID, "runtime_cluster.pod_terminal", resourceID, false, err.Error(), ctx.Request.Context())
		return
	}
	select {
	case <-authorizationRevoked:
		h.auditWithContext(user.ID, "runtime_cluster.pod_terminal", resourceID, false, "authorization expired or was revoked", ctx.Request.Context())
		return
	default:
	}
	if sessionCtx.Err() == context.DeadlineExceeded {
		h.auditWithContext(user.ID, "runtime_cluster.pod_terminal", resourceID, false, "authorization deadline reached", ctx.Request.Context())
		return
	}
	h.auditWithContext(user.ID, "runtime_cluster.pod_terminal", resourceID, true, strings.TrimSpace(ctx.Query("container")), ctx.Request.Context())
}

func (h *Handlers) AuthorizeRuntimeClusterPodTerminal(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if user.Role != authz.PlatformRoleAdmin {
		writeError(ctx, http.StatusForbidden, "只有平台管理员可以打开集群 Pod 终端")
		return
	}
	cluster, _, snapshot, ok := h.runtimeClusterPodTerminalTarget(ctx, user)
	if !ok {
		return
	}
	authorization, ok := h.requireRuntimeTerminalAuthorization(ctx, user)
	if !ok {
		return
	}
	ticket, expiresAt, err := h.issueRuntimeTerminalTicket(
		ctx.Request.Context(),
		authorization,
		"runtime_pod",
		runtimeClusterPodTerminalReference(cluster, snapshot),
	)
	if err != nil {
		h.auditWithContext(user.ID, "runtime_cluster.pod_terminal_authorize", cluster.ID+":"+snapshot.Namespace+"/"+snapshot.Name, false, err.Error(), ctx.Request.Context())
		writeErrorCode(ctx, http.StatusServiceUnavailable, "runtime_terminal.ticket_unavailable", "terminal authorization is temporarily unavailable")
		return
	}
	ctx.JSON(http.StatusOK, runtimeTerminalTicketResponse{Ticket: ticket, ExpiresAt: expiresAt})
}

func (h *Handlers) runtimeClusterPodTerminalTarget(ctx *gin.Context, user model.User) (model.RuntimeCluster, *kubeprovider.Client, kubeprovider.ResourceSnapshot, bool) {
	namespace := strings.TrimSpace(ctx.Query("namespace"))
	name := strings.TrimSpace(ctx.Query("name"))
	if namespace == "" || name == "" {
		writeError(ctx, http.StatusBadRequest, "Pod 命名空间和名称不能为空")
		return model.RuntimeCluster{}, nil, kubeprovider.ResourceSnapshot{}, false
	}
	var cluster model.RuntimeCluster
	if err := h.dbFor(ctx).First(&cluster, "id = ? and type in ?", ctx.Param("clusterId"), []string{"kubernetes", "k3s"}).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "runtime cluster not found")
		return model.RuntimeCluster{}, nil, kubeprovider.ResourceSnapshot{}, false
	}
	kubeconfig := h.secrets.ResolveContext(ctx.Request.Context(), cluster.KubeconfigRef)
	if strings.TrimSpace(kubeconfig) == "" {
		writeError(ctx, http.StatusBadRequest, "运行集群缺少 kubeconfig，无法打开 Pod 终端")
		return model.RuntimeCluster{}, nil, kubeprovider.ResourceSnapshot{}, false
	}
	client, err := kubeprovider.NewClientFromKubeconfig(kubeconfig)
	if err != nil {
		writeError(ctx, http.StatusBadRequest, "运行集群 kubeconfig 无效")
		return model.RuntimeCluster{}, nil, kubeprovider.ResourceSnapshot{}, false
	}
	checkCtx, cancelCheck := context.WithTimeout(ctx.Request.Context(), 10*time.Second)
	defer cancelCheck()
	snapshot, err := client.GetManagedResource(checkCtx, "pod", namespace, name)
	if err != nil {
		writeError(ctx, http.StatusBadGateway, "集群 Pod 读取失败，请确认 Pod 仍存在且归属平台管理")
		return model.RuntimeCluster{}, nil, kubeprovider.ResourceSnapshot{}, false
	}
	if !h.canInspectClusterResourceSnapshot(ctx, user, snapshot) || !h.ensureRuntimeClusterPodWebConsoleEnabled(ctx, snapshot) {
		return model.RuntimeCluster{}, nil, kubeprovider.ResourceSnapshot{}, false
	}
	return cluster, client, snapshot, true
}
