package api

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
)

func (h *Handlers) UpdateDeploymentTargetRuntimeSecrets(ctx *gin.Context) {
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
	if !h.ensureDeploymentTargetCanMutate(ctx, target) || !h.requireStepUp(ctx, user, stepUpPurposeSecretUpdate) {
		return
	}
	var input runtimeSecretMutationInput
	if !bindJSON(ctx, &input) {
		return
	}
	refs := decodeSecretRefs(target.SecretRefs)
	refs, response, ok := h.applyRuntimeSecretMutation(ctx, user, input, refs, "deployment_target:"+target.ID+":runtime")
	if !ok {
		return
	}
	encoded := encodeStringMap(refs)
	if err := h.dbWithContext(ctx.Request.Context()).Model(&model.DeploymentTarget{}).
		Where("id = ? and project_id = ? and application_id = ?", target.ID, project.ID, app.ID).
		Update("secret_refs", encoded).Error; err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "deployment.secret_store_unavailable", "密钥保存失败")
		return
	}
	h.auditWithContext(user.ID, "deployment_target.runtime_secrets.update", target.ID, true, "runtime secret state updated", ctx.Request.Context())
	ctx.JSON(http.StatusOK, response)
}

var runtimeSecretURLCredentials = regexp.MustCompile(`(?i)[a-z][a-z0-9+.-]*://[^\s/@:]+:[^\s/@]+@`)

func validateDeploymentTargetPublicEnvVars(ctx *gin.Context, values map[string]string) bool {
	for key, value := range values {
		if secretLikeEnvironmentKey(key) || runtimeSecretURLCredentials.MatchString(strings.TrimSpace(value)) {
			writeErrorCode(ctx, http.StatusBadRequest, "deployment.secret_must_use_secure_input", "敏感运行时配置必须通过安全密钥表单提交")
			return false
		}
	}
	return true
}

func secretLikeEnvironmentKey(key string) bool {
	upper := strings.ToUpper(strings.TrimSpace(key))
	for _, marker := range []string{"SECRET", "PASSWORD", "TOKEN", "API_KEY", "PRIVATE_KEY", "CLIENT_SECRET", "ACCESS_KEY", "REFRESH_TOKEN", "KUBECONFIG", "CREDENTIAL"} {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	return false
}
