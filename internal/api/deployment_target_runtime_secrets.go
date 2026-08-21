package api

import (
	"net/http"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (h *Handlers) UpdateDeploymentTargetRuntimeSecrets(ctx *gin.Context) {
	setRuntimeSecretNoStoreHeaders(ctx)
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, authz.ProjectRoleOwner, authz.ProjectRoleAdmin, authz.ProjectRoleDeveloper)
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
	var input runtimeSecretMutationRequest
	if !bindJSON(ctx, &input) {
		return
	}
	mutationInput, ok := runtimeSecretMutationInputFromRequest(ctx, input)
	if !ok || !validateRuntimeSecretMutation(ctx, &mutationInput) {
		return
	}
	prepared, err := prepareRuntimeSecretMutation(mutationInput)
	if err != nil {
		writeRuntimeSecretMutationError(ctx, "deployment_target", err)
		return
	}
	response, err := h.mutateRuntimeSecrets(ctx.Request.Context(), user, prepared, deploymentTargetRuntimeSecretMutationOwner(target.ID, project.ID, app.ID))
	if err != nil {
		writeRuntimeSecretMutationError(ctx, "deployment_target", err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}

func deploymentTargetRuntimeSecretMutationOwner(targetID, projectID, applicationID string) runtimeSecretMutationOwner {
	return runtimeSecretMutationOwner{
		ResourceID:     targetID,
		ResourcePrefix: "deployment_target:" + targetID + ":runtime",
		AuditAction:    "deployment_target.runtime_secrets.update",
		LoadRefs: func(tx *gorm.DB) (string, error) {
			var current model.DeploymentTarget
			err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Select("id", "secret_refs").
				First(&current, "id = ? and project_id = ? and application_id = ? and delete_status in ?", targetID, projectID, applicationID, []string{"", "active", "delete_failed"}).Error
			return current.SecretRefs, err
		},
		SaveRefs: func(tx *gorm.DB, encoded string) error {
			return tx.Model(&model.DeploymentTarget{}).
				Where("id = ? and project_id = ? and application_id = ?", targetID, projectID, applicationID).
				Update("secret_refs", encoded).Error
		},
		EncodeRefs: encodeStringMap,
	}
}
