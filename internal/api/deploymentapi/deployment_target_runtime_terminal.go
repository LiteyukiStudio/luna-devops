package deploymentapi

import (
	"context"
	"net/http"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
)

func (h *Handlers) StreamDeploymentTargetRuntimeTerminal(ctx *gin.Context) {
	h.streamRuntimeTerminal(ctx, false, h.deploymentTargetRuntimeTerminalEndpoint)
}

func (h *Handlers) AuthorizeDeploymentTargetRuntimeTerminal(ctx *gin.Context) {
	h.authorizeRuntimeTerminal(ctx, h.deploymentTargetRuntimeTerminalEndpoint)
}

func (h *Handlers) deploymentTargetRuntimeTerminalEndpoint(ctx *gin.Context, project model.Project) (runtimeTerminalEndpoint, bool) {
	app, ok := h.findApplication(ctx)
	if !ok {
		return runtimeTerminalEndpoint{}, false
	}
	if !applicationCanMutate(app) {
		writeErrorCode(ctx, http.StatusConflict, "application.delete_in_progress", "应用正在删除中，不能打开运行终端")
		return runtimeTerminalEndpoint{}, false
	}
	var target model.DeploymentTarget
	if err := h.dbFor(ctx).First(
		&target,
		"id = ? and project_id = ? and application_id = ?",
		ctx.Param("targetId"),
		app.ProjectID,
		app.ID,
	).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "deployment target not found")
		return runtimeTerminalEndpoint{}, false
	}
	if !ensureRuntimeWebConsoleEnabled(ctx, project, target) || !h.ensureDeploymentTargetCanMutate(ctx, target) {
		return runtimeTerminalEndpoint{}, false
	}
	client, namespace, cluster, ok := h.runtimeClientForDeploymentTarget(ctx, project, target)
	if !ok {
		return runtimeTerminalEndpoint{}, false
	}
	reference := deploymentTargetRuntimeTerminalAuthorizationReference{
		ProjectID:          project.ID,
		ApplicationID:      app.ID,
		DeploymentTargetID: target.ID,
		ClusterID:          cluster.ID,
		ClusterKubeconfig:  cluster.KubeconfigRef,
		Namespace:          namespace,
	}
	return runtimeTerminalEndpoint{
		resourceKind:         "deployment_target",
		reference:            reference,
		client:               client,
		namespace:            namespace,
		target:               target,
		auditAction:          "deployment_target_runtime.terminal",
		auditAuthorizeAction: "deployment_target_runtime.terminal_authorize",
		auditResourceID:      target.ID,
		authorizationAllowed: func(checkCtx context.Context, currentUser model.User) bool {
			return h.deploymentTargetRuntimeTerminalAuthorizationAllowed(checkCtx, currentUser, reference)
		},
	}, true
}
