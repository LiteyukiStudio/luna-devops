package api

import (
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"github.com/gin-gonic/gin"
)

type deploymentTargetRuntimeSecretGeneration struct {
	Length   int    `json:"length"`
	Encoding string `json:"encoding"`
}

type deploymentTargetRuntimeSecretsInput struct {
	Values   map[string]string                                  `json:"values"`
	Generate map[string]deploymentTargetRuntimeSecretGeneration `json:"generate"`
	Clear    []string                                           `json:"clear"`
}

type deploymentTargetRuntimeSecretsResponse struct {
	ConfiguredKeys []string `json:"configuredKeys"`
	GeneratedKeys  []string `json:"generatedKeys"`
	ClearedKeys    []string `json:"clearedKeys"`
	SecretSet      bool     `json:"secretSet"`
}

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
	var input deploymentTargetRuntimeSecretsInput
	if !bindJSON(ctx, &input) {
		return
	}
	if !validateDeploymentTargetRuntimeSecretMutation(ctx, &input) {
		return
	}
	refs := decodeSecretRefs(target.SecretRefs)
	configuredKeys := make([]string, 0, len(input.Values))
	generatedKeys := make([]string, 0, len(input.Generate))
	clearedKeys := make([]string, 0, len(input.Clear))
	seenClear := map[string]struct{}{}

	for key, value := range input.Values {
		key = strings.TrimSpace(key)
		if !isBuildEnvKey(key) {
			writeErrorCode(ctx, http.StatusBadRequest, "deployment.secret_key_invalid", "密钥字段名无效")
			return
		}
		if _, exists := input.Generate[key]; exists {
			writeErrorCode(ctx, http.StatusBadRequest, "deployment.secret_update_conflict", "同一密钥不能同时填写和生成")
			return
		}
		if strings.TrimSpace(value) == "" {
			continue
		}
		ref := h.secrets.StoreContext(ctx.Request.Context(), value, user.ID, "deployment_target:"+target.ID+":runtime:"+key)
		if strings.TrimSpace(ref) == "" {
			writeErrorCode(ctx, http.StatusInternalServerError, "deployment.secret_store_unavailable", "密钥保存失败")
			return
		}
		refs[key] = ref
		configuredKeys = append(configuredKeys, key)
	}

	for key, generation := range input.Generate {
		key = strings.TrimSpace(key)
		if !isBuildEnvKey(key) {
			writeErrorCode(ctx, http.StatusBadRequest, "deployment.secret_key_invalid", "密钥字段名无效")
			return
		}
		if generation.Length == 0 {
			generation.Length = 32
		}
		if strings.TrimSpace(generation.Encoding) == "" {
			generation.Encoding = "base64"
		}
		value, err := secret.Generate(generation.Length, generation.Encoding)
		if err != nil {
			writeErrorCode(ctx, http.StatusBadRequest, "deployment.secret_generation_invalid", "密钥生成参数无效")
			return
		}
		ref := h.secrets.StoreContext(ctx.Request.Context(), value, user.ID, "deployment_target:"+target.ID+":runtime:"+key)
		if strings.TrimSpace(ref) == "" {
			writeErrorCode(ctx, http.StatusInternalServerError, "deployment.secret_store_unavailable", "密钥保存失败")
			return
		}
		refs[key] = ref
		generatedKeys = append(generatedKeys, key)
	}

	for _, rawKey := range input.Clear {
		key := strings.TrimSpace(rawKey)
		if !isBuildEnvKey(key) {
			writeErrorCode(ctx, http.StatusBadRequest, "deployment.secret_key_invalid", "密钥字段名无效")
			return
		}
		if _, duplicate := seenClear[key]; duplicate {
			continue
		}
		seenClear[key] = struct{}{}
		if _, exists := refs[key]; exists {
			delete(refs, key)
			clearedKeys = append(clearedKeys, key)
		}
	}

	sort.Strings(configuredKeys)
	sort.Strings(generatedKeys)
	sort.Strings(clearedKeys)
	encoded := encodeStringMap(refs)
	if err := h.dbWithContext(ctx.Request.Context()).Model(&model.DeploymentTarget{}).
		Where("id = ? and project_id = ? and application_id = ?", target.ID, project.ID, app.ID).
		Update("secret_refs", encoded).Error; err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "deployment.secret_store_unavailable", "密钥保存失败")
		return
	}
	h.auditWithContext(user.ID, "deployment_target.runtime_secrets.update", target.ID, true, "runtime secret state updated", ctx.Request.Context())
	ctx.JSON(http.StatusOK, deploymentTargetRuntimeSecretsResponse{
		ConfiguredKeys: configuredKeys,
		GeneratedKeys:  generatedKeys,
		ClearedKeys:    clearedKeys,
		SecretSet:      len(refs) > 0,
	})
}

var runtimeSecretURLCredentials = regexp.MustCompile(`(?i)[a-z][a-z0-9+.-]*://[^\s/@:]+:[^\s/@]+@`)

func validateDeploymentTargetRuntimeSecretMutation(ctx *gin.Context, input *deploymentTargetRuntimeSecretsInput) bool {
	for key := range input.Values {
		if !isBuildEnvKey(key) {
			writeErrorCode(ctx, http.StatusBadRequest, "deployment.secret_key_invalid", "密钥字段名无效")
			return false
		}
		if _, exists := input.Generate[key]; exists {
			writeErrorCode(ctx, http.StatusBadRequest, "deployment.secret_update_conflict", "同一密钥不能同时填写和生成")
			return false
		}
	}
	for key, generation := range input.Generate {
		if !isBuildEnvKey(key) {
			writeErrorCode(ctx, http.StatusBadRequest, "deployment.secret_key_invalid", "密钥字段名无效")
			return false
		}
		if generation.Length == 0 {
			generation.Length = 32
			input.Generate[key] = generation
		}
		if strings.TrimSpace(generation.Encoding) == "" {
			generation.Encoding = "base64"
			input.Generate[key] = generation
		}
		if err := secret.ValidateGeneration(generation.Length, generation.Encoding); err != nil {
			writeErrorCode(ctx, http.StatusBadRequest, "deployment.secret_generation_invalid", "密钥生成参数无效")
			return false
		}
	}
	for _, rawKey := range input.Clear {
		key := strings.TrimSpace(rawKey)
		if !isBuildEnvKey(key) {
			writeErrorCode(ctx, http.StatusBadRequest, "deployment.secret_key_invalid", "密钥字段名无效")
			return false
		}
		if _, exists := input.Values[key]; exists {
			writeErrorCode(ctx, http.StatusBadRequest, "deployment.secret_update_conflict", "同一密钥不能同时设置和清除")
			return false
		}
		if _, exists := input.Generate[key]; exists {
			writeErrorCode(ctx, http.StatusBadRequest, "deployment.secret_update_conflict", "同一密钥不能同时生成和清除")
			return false
		}
	}
	return true
}

func validateDeploymentTargetPublicEnvVars(ctx *gin.Context, raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "{}" {
		return true
	}
	values, ok := parseRuntimeKeyValueInput(ctx, trimmed, "运行时环境变量格式无效")
	if !ok {
		return false
	}
	for key, value := range values {
		if secretLikeEnvironmentKey(key) || runtimeSecretURLCredentials.MatchString(strings.TrimSpace(value)) {
			writeErrorCode(ctx, http.StatusBadRequest, "deployment.secret_must_use_secure_input", "敏感运行时配置必须通过安全密钥表单提交")
			return false
		}
	}
	return true
}

func validateDeploymentTargetSecretRefs(ctx *gin.Context, raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "{}" {
		return true
	}
	var refs map[string]string
	if err := json.Unmarshal([]byte(trimmed), &refs); err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "deployment.secret_must_use_secure_input", "运行时密钥只能提交 Secret 引用")
		return false
	}
	for key, ref := range refs {
		if !isBuildEnvKey(key) || !(strings.HasPrefix(strings.TrimSpace(ref), "secret-id:") || strings.HasPrefix(strings.TrimSpace(ref), "secret:v1:")) {
			writeErrorCode(ctx, http.StatusBadRequest, "deployment.secret_must_use_secure_input", "运行时密钥只能提交 Secret 引用")
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
