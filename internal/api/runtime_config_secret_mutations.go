package api

import (
	"encoding/json"
	"net/http"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (h *Handlers) UpdateProjectRuntimeConfigSetRuntimeSecrets(ctx *gin.Context) {
	setRuntimeSecretNoStoreHeaders(ctx)
	user, project, ok := h.projectAndCurrentUserWithRoles(ctx, authz.ProjectRoleOwner, authz.ProjectRoleAdmin, authz.ProjectRoleDeveloper)
	if !ok || !h.ensureProjectCanMutate(ctx, project) {
		return
	}
	var set model.ProjectRuntimeConfigSet
	if err := h.dbFor(ctx).First(&set, "id = ? and project_id = ?", ctx.Param("setId"), project.ID).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "运行配置集不存在")
		return
	}
	if !h.ensureRuntimeConfigSetCanMutate(ctx, set) || !h.requireStepUp(ctx, user, stepUpPurposeSecretUpdate) {
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
		writeRuntimeSecretMutationError(ctx, "runtime_config_set", err)
		return
	}
	response, err := h.mutateRuntimeSecrets(ctx.Request.Context(), user, prepared, projectRuntimeConfigSetSecretMutationOwner(set.ID, project.ID))
	if err != nil {
		writeRuntimeSecretMutationError(ctx, "runtime_config_set", err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}

func projectRuntimeConfigSetSecretMutationOwner(setID, projectID string) runtimeSecretMutationOwner {
	return runtimeSecretMutationOwner{
		ResourceID:     setID,
		ResourcePrefix: "runtime_config:" + setID + ":runtime",
		AuditAction:    "runtime_config_set.runtime_secrets.update",
		LoadRefs: func(tx *gorm.DB) (string, error) {
			var current model.ProjectRuntimeConfigSet
			err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Select("id", "secret_refs").
				First(&current, "id = ? and project_id = ? and delete_status in ?", setID, projectID, []string{"", "active", "delete_failed"}).Error
			return current.SecretRefs, err
		},
		SaveRefs: func(tx *gorm.DB, encoded string) error {
			return tx.Model(&model.ProjectRuntimeConfigSet{}).
				Where("id = ? and project_id = ?", setID, projectID).
				Update("secret_refs", encoded).Error
		},
		EncodeRefs: func(refs map[string]string) string {
			encoded, _ := json.Marshal(refs)
			return string(encoded)
		},
	}
}
