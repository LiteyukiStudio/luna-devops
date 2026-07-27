package api

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
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
		if isWorkloadResourceKind(options.Kind) {
			responses = groupWorkloadPodResponses(responses)
		}
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
		if err := h.db.First(&user, "id = ? and disabled = ?", ticketValue.UserID, false).Error; err != nil {
			writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.account.disabled")
			return
		}
		authorization = ticketValue.Authorization
	}
	if user.Role != "platform_admin" {
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
		h.audit(user.ID, "runtime_cluster.pod_terminal", cluster.ID+":"+snapshot.Namespace+"/"+snapshot.Name, false, err.Error())
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
		_, _ = wsWriter.Write([]byte("\r\nterminal disconnected: " + err.Error() + "\r\n"))
		h.audit(user.ID, "runtime_cluster.pod_terminal", resourceID, false, err.Error())
		return
	}
	select {
	case <-authorizationRevoked:
		h.audit(user.ID, "runtime_cluster.pod_terminal", resourceID, false, "authorization expired or was revoked")
		return
	default:
	}
	if sessionCtx.Err() == context.DeadlineExceeded {
		h.audit(user.ID, "runtime_cluster.pod_terminal", resourceID, false, "authorization deadline reached")
		return
	}
	h.audit(user.ID, "runtime_cluster.pod_terminal", resourceID, true, strings.TrimSpace(ctx.Query("container")))
}

func (h *Handlers) AuthorizeRuntimeClusterPodTerminal(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	if user.Role != "platform_admin" {
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
		h.audit(user.ID, "runtime_cluster.pod_terminal_authorize", cluster.ID+":"+snapshot.Namespace+"/"+snapshot.Name, false, err.Error())
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
	if err := h.db.First(&cluster, "id = ? and type in ?", ctx.Param("clusterId"), []string{"kubernetes", "k3s"}).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "runtime cluster not found")
		return model.RuntimeCluster{}, nil, kubeprovider.ResourceSnapshot{}, false
	}
	kubeconfig := h.secrets.Resolve(cluster.KubeconfigRef)
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
