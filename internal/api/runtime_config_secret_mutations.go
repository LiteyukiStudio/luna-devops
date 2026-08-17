package api

import (
	"encoding/json"
	"net/http"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
)

func (h *Handlers) UpdateProjectRuntimeConfigSetRuntimeSecrets(ctx *gin.Context) {
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
	var input runtimeSecretMutationInput
	if !bindJSON(ctx, &input) {
		return
	}
	refs := decodeSecretRefs(set.SecretRefs)
	refs, response, ok := h.applyRuntimeSecretMutation(ctx, user, input, refs, "runtime_config:"+set.ID+":runtime")
	if !ok {
		return
	}
	encoded, err := json.Marshal(refs)
	if err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "deployment.secret_store_unavailable", "密钥保存失败")
		return
	}
	if err := h.dbWithContext(ctx.Request.Context()).Model(&model.ProjectRuntimeConfigSet{}).
		Where("id = ? and project_id = ?", set.ID, project.ID).
		Update("secret_refs", string(encoded)).Error; err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "deployment.secret_store_unavailable", "密钥保存失败")
		return
	}
	h.auditWithContext(user.ID, "runtime_config_set.runtime_secrets.update", set.ID, true, "runtime secret state updated", ctx.Request.Context())
	ctx.JSON(http.StatusOK, response)
}
