package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func (h *Handlers) GetReleaseRuntimeLogs(ctx *gin.Context) {
	if _, ok := h.findProjectForCurrentUser(ctx); !ok {
		return
	}
	release, ok := h.findRelease(ctx)
	if !ok {
		return
	}
	client, namespace, target, ok := h.releaseRuntimeClient(ctx, release)
	if !ok {
		return
	}
	tailLines := int64(500)
	if value := strings.TrimSpace(ctx.Query("tailLines")); value != "" {
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil && parsed > 0 && parsed <= 5000 {
			tailLines = parsed
		}
	}
	requestCtx, cancel := context.WithTimeout(ctx.Request.Context(), 12*time.Second)
	defer cancel()
	result, err := client.RuntimePodLogs(requestCtx, kubeprovider.RuntimePodLogsOptions{
		Namespace:          namespace,
		DeploymentTargetID: target.ID,
		Container:          strings.TrimSpace(ctx.Query("container")),
		TailLines:          tailLines,
	})
	if err != nil {
		writeError(ctx, http.StatusBadGateway, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, result)
}

func (h *Handlers) ExecReleaseRuntimeCommand(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, "owner", "admin", "developer")
	if !ok {
		return
	}
	if !h.ensureProjectCanMutate(ctx, project) {
		return
	}
	release, ok := h.findRelease(ctx)
	if !ok {
		return
	}
	target, ok := h.releaseRuntimeTarget(ctx, release)
	if !ok || !ensureRuntimeWebConsoleEnabled(ctx, project, target) {
		return
	}
	if !h.requireStepUp(ctx, user, stepUpPurposeRuntimeExec) {
		return
	}
	var input releaseRuntimeExecInput
	if !bindJSON(ctx, &input) {
		return
	}
	command := strings.TrimSpace(input.Command)
	if command == "" {
		writeError(ctx, http.StatusBadRequest, "command is required")
		return
	}
	if len(command) > 2000 {
		writeError(ctx, http.StatusBadRequest, "command is too long")
		return
	}
	client, namespace, _, ok := h.runtimeClientForDeploymentTarget(ctx, project, target)
	if !ok {
		return
	}
	if !h.ensureDeploymentTargetCanMutate(ctx, target) {
		return
	}
	requestCtx, cancel := context.WithTimeout(ctx.Request.Context(), 30*time.Second)
	defer cancel()
	result, err := client.RuntimeExec(requestCtx, kubeprovider.RuntimeExecOptions{
		Namespace:          namespace,
		DeploymentTargetID: target.ID,
		Container:          strings.TrimSpace(input.Container),
		Command:            command,
	})
	if err != nil {
		h.audit(user.ID, "release_runtime.exec", release.ID, false, err.Error())
		writeError(ctx, http.StatusBadGateway, err.Error())
		return
	}
	h.audit(user.ID, "release_runtime.exec", release.ID, result.ExitCode == 0, runtimeExecAuditMessage(command, input.Container, result))
	ctx.JSON(http.StatusOK, result)
}

func (h *Handlers) StreamReleaseRuntimeTerminal(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, "owner", "admin", "developer")
	if !ok {
		return
	}
	if !h.ensureProjectCanMutate(ctx, project) {
		return
	}
	release, ok := h.findRelease(ctx)
	if !ok {
		return
	}
	target, ok := h.releaseRuntimeTarget(ctx, release)
	if !ok || !ensureRuntimeWebConsoleEnabled(ctx, project, target) {
		return
	}
	client, namespace, cluster, ok := h.runtimeClientForDeploymentTarget(ctx, project, target)
	if !ok {
		return
	}
	if !h.ensureDeploymentTargetCanMutate(ctx, target) {
		return
	}
	authorization, ok := h.requireRuntimeTerminalAuthorization(ctx, user)
	if !ok {
		return
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
		h.audit(user.ID, "release_runtime.terminal", release.ID, false, err.Error())
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
	reference := releaseRuntimeTerminalAuthorizationReference{
		ProjectID:          project.ID,
		ApplicationID:      release.ApplicationID,
		ReleaseID:          release.ID,
		DeploymentTargetID: target.ID,
		ClusterID:          cluster.ID,
		ClusterKubeconfig:  cluster.KubeconfigRef,
		Namespace:          namespace,
	}
	authorizationRevoked := h.monitorRuntimeTerminalAuthorization(sessionCtx, authorization, func(checkCtx context.Context, currentUser model.User) bool {
		return h.releaseRuntimeTerminalAuthorizationAllowed(checkCtx, currentUser, reference)
	}, cancel)

	go h.readRuntimeTerminalMessages(sessionCtx, conn, stdinWriter, sizeQueue, cancel)
	err = client.RuntimeTerminal(sessionCtx, kubeprovider.RuntimeTerminalOptions{
		Namespace:          namespace,
		DeploymentTargetID: target.ID,
		Container:          strings.TrimSpace(ctx.Query("container")),
		Stdin:              stdinReader,
		Stdout:             wsWriter,
		SizeQueue:          sizeQueue,
	})
	if err != nil && sessionCtx.Err() == nil {
		_, _ = wsWriter.Write([]byte("\r\nterminal disconnected: " + err.Error() + "\r\n"))
		h.audit(user.ID, "release_runtime.terminal", release.ID, false, err.Error())
		return
	}
	select {
	case <-authorizationRevoked:
		h.audit(user.ID, "release_runtime.terminal", release.ID, false, "authorization expired or was revoked")
		return
	default:
	}
	if sessionCtx.Err() == context.DeadlineExceeded {
		h.audit(user.ID, "release_runtime.terminal", release.ID, false, "authorization deadline reached")
		return
	}
	h.audit(user.ID, "release_runtime.terminal", release.ID, true, strings.TrimSpace(ctx.Query("container")))
}

func (h *Handlers) AuthorizeReleaseRuntimeTerminal(ctx *gin.Context) {
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, "owner", "admin", "developer")
	if !ok || !h.ensureProjectCanMutate(ctx, project) {
		return
	}
	release, ok := h.findRelease(ctx)
	if !ok {
		return
	}
	target, ok := h.releaseRuntimeTarget(ctx, release)
	if !ok || !ensureRuntimeWebConsoleEnabled(ctx, project, target) || !h.ensureDeploymentTargetCanMutate(ctx, target) {
		return
	}
	if _, ok := h.requireRuntimeTerminalAuthorization(ctx, user); !ok {
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (h *Handlers) readRuntimeTerminalMessages(ctx context.Context, conn *websocket.Conn, stdin *io.PipeWriter, sizeQueue *runtimeTerminalSizeQueue, cancel context.CancelFunc) {
	defer cancel()
	defer stdin.Close()
	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
			continue
		}
		if messageType == websocket.TextMessage {
			var message runtimeTerminalClientMessage
			if err := json.Unmarshal(data, &message); err == nil && message.Type == "resize" {
				sizeQueue.Push(message.Cols, message.Rows)
				continue
			}
		}
		if _, err := stdin.Write(data); err != nil {
			return
		}
	}
}

func (h *Handlers) releaseRuntimeClient(ctx *gin.Context, release model.Release) (*kubeprovider.Client, string, model.DeploymentTarget, bool) {
	var project model.Project
	if err := h.db.First(&project, "id = ?", release.ProjectID).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "project not found")
		return nil, "", model.DeploymentTarget{}, false
	}
	target, ok := h.releaseRuntimeTarget(ctx, release)
	if !ok {
		return nil, "", model.DeploymentTarget{}, false
	}
	client, namespace, _, ok := h.runtimeClientForDeploymentTarget(ctx, project, target)
	return client, namespace, target, ok
}

func (h *Handlers) releaseRuntimeTarget(ctx *gin.Context, release model.Release) (model.DeploymentTarget, bool) {
	var target model.DeploymentTarget
	if err := h.db.First(&target, "id = ? and project_id = ? and application_id = ?", release.DeploymentTargetID, release.ProjectID, release.ApplicationID).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "deployment target not found")
		return model.DeploymentTarget{}, false
	}
	return target, true
}

func ensureRuntimeWebConsoleEnabled(ctx *gin.Context, project model.Project, target model.DeploymentTarget) bool {
	if runtimeWebConsoleEnabled(project, target) {
		return true
	}
	writeErrorCode(ctx, http.StatusForbidden, "runtime.web_console_disabled", "web console is disabled for this deployment target")
	return false
}

func runtimeWebConsoleEnabled(project model.Project, target model.DeploymentTarget) bool {
	return project.WebConsoleEnabled && (target.WebConsoleEnabled == nil || *target.WebConsoleEnabled)
}

func normalizeWebConsoleOverride(value *bool) *bool {
	if value == nil || *value {
		return nil
	}
	return value
}

func (h *Handlers) runtimeClientForDeploymentTarget(ctx *gin.Context, project model.Project, target model.DeploymentTarget) (*kubeprovider.Client, string, model.RuntimeCluster, bool) {
	cluster, ok := h.runtimeClusterForDeploymentTarget(ctx, target)
	if !ok {
		return nil, "", model.RuntimeCluster{}, false
	}
	kubeconfig := h.secrets.Resolve(cluster.KubeconfigRef)
	if strings.TrimSpace(kubeconfig) == "" {
		writeError(ctx, http.StatusBadRequest, "运行集群缺少 kubeconfig，无法读取运行时")
		return nil, "", model.RuntimeCluster{}, false
	}
	client, err := kubeprovider.NewClientFromKubeconfig(kubeconfig)
	if err != nil {
		writeError(ctx, http.StatusBadRequest, "运行集群 kubeconfig 无效")
		return nil, "", model.RuntimeCluster{}, false
	}
	return client, deploymentTargetNamespace(project, target), cluster, true
}
