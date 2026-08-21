package api

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	kubeprovider "github.com/LiteyukiStudio/devops/internal/provider/kubernetes"
	"github.com/LiteyukiStudio/devops/internal/runtimecommand"
	"github.com/gin-gonic/gin"
)

type releaseRuntimeCommandSessionInput struct {
	Container string `json:"container"`
}

type releaseRuntimeCommandSessionExecuteInput struct {
	Command string `json:"command"`
}

func (h *Handlers) CreateReleaseRuntimeCommandSession(ctx *gin.Context) {
	if h.runtimeCommands == nil {
		writeErrorCode(ctx, http.StatusServiceUnavailable, "runtime.command_session_unavailable", "runtime command session broker is unavailable")
		return
	}
	user, project, release, target, client, namespace, subjectID, runID, ok := h.releaseRuntimeCommandSessionContext(ctx, true)
	if !ok {
		return
	}
	var input releaseRuntimeCommandSessionInput
	if !bindJSON(ctx, &input) {
		return
	}
	resolveCtx, cancel := context.WithTimeout(ctx.Request.Context(), 10*time.Second)
	identity, err := client.ResolveRuntimePod(resolveCtx, namespace, target.ID, strings.TrimSpace(input.Container))
	cancel()
	if err != nil {
		h.auditWithContext(user.ID, "release_runtime.command_session.create", release.ID, false, err.Error(), ctx.Request.Context())
		writeErrorCode(ctx, http.StatusBadGateway, "runtime.command_session_unavailable", "runtime shell target is unavailable")
		return
	}
	binding := runtimecommand.Binding{
		UserID: user.ID, SubjectID: subjectID, AgentRunID: runID,
		ProjectID: project.ID, ApplicationID: release.ApplicationID, ReleaseID: release.ID,
		DeploymentTargetID: target.ID, Container: identity.Container,
	}
	snapshot, err := h.runtimeCommands.Create(ctx.Request.Context(), binding, func(shellCtx context.Context, stdinReader io.Reader, stdoutWriter io.Writer) error {
		return client.RuntimeShell(shellCtx, kubeprovider.RuntimeShellOptions{
			Namespace: namespace, DeploymentTargetID: target.ID, Container: identity.Container,
			Stdin: stdinReader, Stdout: stdoutWriter,
		})
	})
	if err != nil {
		h.auditWithContext(user.ID, "release_runtime.command_session.create", release.ID, false, err.Error(), ctx.Request.Context())
		writeRuntimeCommandSessionError(ctx, err)
		return
	}
	h.auditWithContext(user.ID, "release_runtime.command_session.create", release.ID, true,
		fmt.Sprintf("session=%s pod=%s container=%s", snapshot.ID, identity.Pod, identity.Container), ctx.Request.Context())
	ctx.JSON(http.StatusCreated, snapshot)
}

func (h *Handlers) ExecuteReleaseRuntimeCommandSession(ctx *gin.Context) {
	if h.runtimeCommands == nil {
		writeErrorCode(ctx, http.StatusServiceUnavailable, "runtime.command_session_unavailable", "runtime command session broker is unavailable")
		return
	}
	user, project, release, target, _, _, subjectID, runID, ok := h.releaseRuntimeCommandSessionContext(ctx, false)
	if !ok {
		return
	}
	var input releaseRuntimeCommandSessionExecuteInput
	if !bindJSON(ctx, &input) {
		return
	}
	binding := runtimecommand.Binding{
		UserID: user.ID, SubjectID: subjectID, AgentRunID: runID,
		ProjectID: project.ID, ApplicationID: release.ApplicationID, ReleaseID: release.ID,
		DeploymentTargetID: target.ID,
	}
	result, err := h.runtimeCommands.Execute(ctx.Request.Context(), ctx.Param("sessionId"), binding, input.Command)
	if err != nil {
		h.auditWithContext(user.ID, "release_runtime.command_session.execute", release.ID, false,
			runtimeCommandSessionAudit(input.Command, ctx.Param("sessionId"), -1), ctx.Request.Context())
		writeRuntimeCommandSessionError(ctx, err)
		return
	}
	h.auditWithContext(user.ID, "release_runtime.command_session.execute", release.ID, result.ExitCode == 0,
		runtimeCommandSessionAudit(input.Command, ctx.Param("sessionId"), result.ExitCode), ctx.Request.Context())
	ctx.JSON(http.StatusOK, result)
}

func (h *Handlers) CloseReleaseRuntimeCommandSession(ctx *gin.Context) {
	if h.runtimeCommands == nil {
		writeErrorCode(ctx, http.StatusServiceUnavailable, "runtime.command_session_unavailable", "runtime command session broker is unavailable")
		return
	}
	user, project, release, target, _, _, subjectID, runID, ok := h.releaseRuntimeCommandSessionContext(ctx, false)
	if !ok {
		return
	}
	binding := runtimecommand.Binding{
		UserID: user.ID, SubjectID: subjectID, AgentRunID: runID,
		ProjectID: project.ID, ApplicationID: release.ApplicationID, ReleaseID: release.ID,
		DeploymentTargetID: target.ID,
	}
	if err := h.runtimeCommands.Close(ctx.Request.Context(), ctx.Param("sessionId"), binding); err != nil {
		writeRuntimeCommandSessionError(ctx, err)
		return
	}
	h.auditWithContext(user.ID, "release_runtime.command_session.close", release.ID, true,
		"session="+ctx.Param("sessionId"), ctx.Request.Context())
	ctx.Status(http.StatusNoContent)
}

func (h *Handlers) releaseRuntimeCommandSessionContext(ctx *gin.Context, connect bool) (
	user model.User, project model.Project, release model.Release, target model.DeploymentTarget,
	client *kubeprovider.Client, namespace, subjectID, runID string, ok bool,
) {
	user, project, ok = h.projectAndCurrentUserWithRoles(ctx, authz.ProjectRoleOwner, authz.ProjectRoleAdmin, authz.ProjectRoleDeveloper)
	if !ok || !h.ensureProjectCanMutate(ctx, project) {
		return
	}
	release, ok = h.findRelease(ctx)
	if !ok {
		return
	}
	target, ok = h.releaseRuntimeTarget(ctx, release)
	if !ok || !ensureRuntimeWebConsoleEnabled(ctx, project, target) || !h.ensureDeploymentTargetCanMutate(ctx, target) {
		ok = false
		return
	}
	session, sessionOK := h.currentSessionFromCookie(ctx)
	if !sessionOK || session.UserID != user.ID {
		writeErrorKey(ctx, http.StatusUnauthorized, requestLanguage(ctx), "auth.session.expired")
		ok = false
		return
	}
	subjectID = session.ID
	runID = strings.TrimSpace(ctx.GetHeader("X-Luna-AI-Run-ID"))
	if connect {
		client, namespace, _, ok = h.runtimeClientForDeploymentTarget(ctx, project, target)
	}
	return
}

func writeRuntimeCommandSessionError(ctx *gin.Context, err error) {
	switch {
	case errors.Is(err, runtimecommand.ErrOwnerMismatch):
		writeErrorCode(ctx, http.StatusConflict, "runtime.command_session.owner_mismatch", "runtime command session belongs to another API instance")
	case errors.Is(err, runtimecommand.ErrBindingMismatch):
		writeErrorCode(ctx, http.StatusForbidden, "runtime.command_session_binding_changed", "runtime command session binding no longer matches the actor or resource")
	case errors.Is(err, runtimecommand.ErrExpired):
		writeErrorCode(ctx, http.StatusGone, "runtime.command_session_expired", "runtime command session expired")
	case errors.Is(err, runtimecommand.ErrNotFound), errors.Is(err, runtimecommand.ErrClosed):
		writeErrorCode(ctx, http.StatusNotFound, "runtime.command_session_not_found", "runtime command session is unavailable")
	case errors.Is(err, runtimecommand.ErrCommandTooLong):
		writeErrorCode(ctx, http.StatusBadRequest, "request.invalid", "runtime command is empty or too long")
	case errors.Is(err, context.DeadlineExceeded):
		writeErrorCode(ctx, http.StatusGatewayTimeout, "runtime.command_timeout", "runtime command timed out")
	default:
		writeErrorCode(ctx, http.StatusBadGateway, "runtime.command_session_failed", "runtime command session failed")
	}
}

func runtimeCommandSessionAudit(command, sessionID string, exitCode int) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(command)))
	return fmt.Sprintf("session=%s exitCode=%d commandBytes=%d commandSha256=%x", sessionID, exitCode, len([]byte(command)), digest)
}
