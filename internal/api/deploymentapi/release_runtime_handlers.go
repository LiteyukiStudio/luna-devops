package deploymentapi

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	transportapi "github.com/LiteyukiStudio/devops/internal/api/transport"
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/runtimeaccess"
	"github.com/gin-gonic/gin"
)

func (h *Handlers) GetReleaseRuntimeLogs(ctx *gin.Context) {
	markLiveObservationResponse(ctx)
	if _, _, ok := h.authorizeProject(ctx, authz.ActionDeploymentRead); !ok {
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
	user, project, ok := h.authorizeProject(ctx, authz.ActionDeploymentExec)
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
		h.auditWithContext(user.ID, "release_runtime.exec", release.ID, false, err.Error(), ctx.Request.Context())
		writeError(ctx, http.StatusBadGateway, err.Error())
		return
	}
	h.auditWithContext(user.ID, "release_runtime.exec", release.ID, result.ExitCode == 0, runtimeExecAuditMessage(command, result), ctx.Request.Context())
	ctx.JSON(http.StatusOK, result)
}

func (h *Handlers) StreamReleaseRuntimeTerminal(ctx *gin.Context) {
	if !transportapi.RuntimeTerminalSubprotocolRequested(ctx.Request) {
		writeErrorCode(ctx, http.StatusBadRequest, "runtime_terminal.protocol_required", "terminal WebSocket requires the luna.devops.terminal.v1 subprotocol")
		return
	}
	ticket := strings.TrimSpace(ctx.Query("ticket"))
	if !requireRuntimeTerminalTicketForBearer(ctx, ticket) {
		return
	}
	var (
		user          model.User
		project       model.Project
		authorization runtimeTerminalAuthorizationBinding
		ticketValue   runtimeTerminalTicketValue
		ok            bool
	)
	if ticket == "" {
		user, project, ok = h.authorizeProject(ctx, authz.ActionDeploymentExec)
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
		project, ok = h.releaseRuntimeTerminalProjectForUser(ctx, user)
		if !ok {
			return
		}
		authorization = ticketValue.Authorization
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
	reference := releaseRuntimeTerminalAuthorizationReference{
		ProjectID:          project.ID,
		ApplicationID:      release.ApplicationID,
		ReleaseID:          release.ID,
		DeploymentTargetID: target.ID,
		ClusterID:          cluster.ID,
		ClusterKubeconfig:  cluster.KubeconfigRef,
		Namespace:          namespace,
	}
	if ticket == "" {
		authorization, ok = h.requireRuntimeTerminalAuthorization(ctx, user)
		if !ok {
			return
		}
	} else {
		if !ticketValue.Matches("release", reference) ||
			!h.continuousAuthorizationActive(ctx.Request.Context(), authorization, func(checkCtx context.Context, currentUser model.User) bool {
				return h.releaseRuntimeTerminalAuthorizationAllowed(checkCtx, currentUser, reference)
			}) {
			writeErrorCode(ctx, http.StatusUnauthorized, "runtime_terminal.ticket_invalid", "terminal ticket is invalid, expired, revoked, or bound to another resource")
			return
		}
	}
	upgrader := transportapi.RuntimeTerminalUpgrader(h.host.AllowedOrigin)
	conn, err := upgrader.Upgrade(ctx.Writer, ctx.Request, nil)
	if err != nil {
		h.auditWithContext(user.ID, "release_runtime.terminal", release.ID, false, err.Error(), ctx.Request.Context())
		return
	}
	defer conn.Close()

	sessionCtx, cancel := context.WithDeadline(ctx.Request.Context(), authorization.Deadline)
	defer cancel()
	stdinReader, stdinWriter := io.Pipe()
	defer stdinReader.Close()
	defer stdinWriter.Close()
	sizeQueue := transportapi.NewRuntimeTerminalSizeQueue()
	defer sizeQueue.Close()
	terminalSocket := transportapi.NewRuntimeTerminalWebSocket(conn)
	var authorizationRevoked atomic.Bool
	_, authorizationActive := h.monitorContinuousAuthorization(sessionCtx, authorization, func(checkCtx context.Context, currentUser model.User) bool {
		return h.releaseRuntimeTerminalAuthorizationAllowed(checkCtx, currentUser, reference)
	}, func() {
		authorizationRevoked.Store(true)
		cancel()
	})
	if !authorizationActive {
		_ = terminalSocket.CloseAuthorizationRevoked()
		h.auditWithContext(user.ID, "release_runtime.terminal", release.ID, false, "authorization expired or was revoked", ctx.Request.Context())
		return
	}

	inputDone := terminalSocket.PumpInput(stdinWriter, sizeQueue, cancel)
	result, streamErr := client.RuntimeTerminal(sessionCtx, kubeprovider.RuntimeTerminalOptions{
		Namespace:          namespace,
		DeploymentTargetID: target.ID,
		Container:          strings.TrimSpace(ctx.Query("container")),
		Stdin:              stdinReader,
		Stdout:             terminalSocket,
		SizeQueue:          sizeQueue,
	})
	end := transportapi.FinishRuntimeTerminal(terminalSocket, result.ExitCode, streamErr, sessionCtx.Err(), authorizationRevoked.Load(), inputDone)
	switch end {
	case transportapi.RuntimeTerminalEndAuthorizationLost:
		h.auditWithContext(user.ID, "release_runtime.terminal", release.ID, false, "authorization expired or was revoked", ctx.Request.Context())
		return
	case transportapi.RuntimeTerminalEndAuthorizationExpiry:
		h.auditWithContext(user.ID, "release_runtime.terminal", release.ID, false, "authorization deadline reached", ctx.Request.Context())
		return
	case transportapi.RuntimeTerminalEndInternalError:
		h.auditWithContext(user.ID, "release_runtime.terminal", release.ID, false, streamErr.Error(), ctx.Request.Context())
		return
	case transportapi.RuntimeTerminalEndProtocolError:
		h.auditWithContext(user.ID, "release_runtime.terminal", release.ID, false, "terminal protocol error", ctx.Request.Context())
		return
	case transportapi.RuntimeTerminalEndClientDisconnected:
		h.auditWithContext(user.ID, "release_runtime.terminal", release.ID, false, "terminal client disconnected", ctx.Request.Context())
		return
	}
	h.auditWithContext(user.ID, "release_runtime.terminal", release.ID, true, strings.TrimSpace(ctx.Query("container")), ctx.Request.Context())
}

func (h *Handlers) AuthorizeReleaseRuntimeTerminal(ctx *gin.Context) {
	user, project, ok := h.authorizeProject(ctx, authz.ActionDeploymentExec)
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
	_, namespace, cluster, ok := h.runtimeClientForDeploymentTarget(ctx, project, target)
	if !ok {
		return
	}
	authorization, ok := h.requireRuntimeTerminalAuthorization(ctx, user)
	if !ok {
		return
	}
	reference := releaseRuntimeTerminalAuthorizationReference{
		ProjectID:          project.ID,
		ApplicationID:      release.ApplicationID,
		ReleaseID:          release.ID,
		DeploymentTargetID: target.ID,
		ClusterID:          cluster.ID,
		ClusterKubeconfig:  cluster.KubeconfigRef,
		Namespace:          namespace,
	}
	ticket, expiresAt, err := h.issueRuntimeTerminalTicket(
		ctx.Request.Context(),
		authorization,
		"release",
		reference,
	)
	if err != nil {
		h.auditWithContext(user.ID, "release_runtime.terminal_authorize", release.ID, false, err.Error(), ctx.Request.Context())
		writeErrorCode(ctx, http.StatusServiceUnavailable, "runtime_terminal.ticket_unavailable", "terminal authorization is temporarily unavailable")
		return
	}
	ctx.JSON(http.StatusOK, runtimeTerminalTicketResponse{Ticket: ticket, ExpiresAt: expiresAt})
}

func (h *Handlers) releaseRuntimeTerminalProjectForUser(ctx *gin.Context, user model.User) (model.Project, bool) {
	project, ok := h.findProject(ctx)
	if !ok {
		return model.Project{}, false
	}
	allowed, err := h.projectRoleActionAllowed(ctx.Request.Context(), user, project.ID, authz.ActionDeploymentExec)
	if err != nil {
		writeProjectAuthorizationError(ctx, err)
		return model.Project{}, false
	}
	if !allowed {
		writeError(ctx, http.StatusForbidden, "你没有执行该项目操作的权限")
		return model.Project{}, false
	}
	return project, true
}

func (h *Handlers) releaseRuntimeClient(ctx *gin.Context, release model.Release) (*kubeprovider.Client, string, model.DeploymentTarget, bool) {
	var project model.Project
	if err := h.dbFor(ctx).First(&project, "id = ?", release.ProjectID).Error; err != nil {
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
	if err := h.dbFor(ctx).First(&target, "id = ? and project_id = ? and application_id = ?", release.DeploymentTargetID, release.ProjectID, release.ApplicationID).Error; err != nil {
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
	return runtimeaccess.Enabled(project.WebConsoleEnabled, target.WebConsoleEnabled)
}

func normalizeWebConsoleOverride(value *bool) *bool {
	return runtimeaccess.NormalizeOverride(value)
}

func (h *Handlers) runtimeClientForDeploymentTarget(ctx *gin.Context, project model.Project, target model.DeploymentTarget) (*kubeprovider.Client, string, model.RuntimeCluster, bool) {
	cluster, ok := h.runtimeClusterForDeploymentTarget(ctx, target)
	if !ok {
		return nil, "", model.RuntimeCluster{}, false
	}
	kubeconfig := h.secrets.ResolveContext(ctx.Request.Context(), cluster.KubeconfigRef)
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
