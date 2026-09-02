package deploymentapi

import (
	"net/http"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
)

func (h *Handlers) UpdateDeploymentTargetRuntimeSecrets(ctx *gin.Context) {
	setRuntimeSecretNoStoreHeaders(ctx)
	user, project, ok := h.authorizeProject(ctx, authz.ActionDeploymentUpdate)
	if !ok || !h.ensureProjectCanMutate(ctx, project) {
		return
	}
	app, ok := h.findApplication(ctx)
	if !ok || !applicationCanMutate(app) {
		return
	}
	var target model.DeploymentTarget
	if err := h.dbFor(ctx).First(&target, "id = ? and project_id = ? and application_id = ?", ctx.Param("targetId"), project.ID, app.ID).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "deployment target not found")
		return
	}
	if !h.ensureDeploymentTargetCanMutate(ctx, target) {
		return
	}
	h.host.MutateDeploymentTargetRuntimeSecrets(ctx, user, project, app, target)
}
