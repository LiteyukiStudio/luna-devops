package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/buildenv"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type buildEnvironmentConfigInput struct {
	Variables map[string]string `json:"variables"`
	Secrets   map[string]string `json:"secrets"`
}

type buildEnvironmentConfigResponse struct {
	Scope     string            `json:"scope"`
	ScopeRef  string            `json:"scopeRef"`
	Variables map[string]string `json:"variables"`
	Secrets   map[string]bool   `json:"secrets"`
}

func (h *Handlers) GetBuildEnvironmentConfig(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	scope, scopeRef, _, ok := h.authorizeBuildEnvironmentConfig(ctx, user)
	if !ok {
		return
	}
	config, err := h.findBuildEnvironmentConfig(h.db, scope, scopeRef)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		config = model.BuildEnvironmentConfig{Scope: scope, ScopeRef: scopeRef, Variables: "{}", SecretRefs: "{}"}
	}
	ctx.JSON(http.StatusOK, buildEnvironmentConfigResponseFromModel(config))
}

func (h *Handlers) UpdateBuildEnvironmentConfig(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	scope, scopeRef, _, ok := h.authorizeBuildEnvironmentConfig(ctx, user)
	if !ok {
		return
	}
	if !h.requireStepUp(ctx, user, stepUpPurposeSecretUpdate) {
		return
	}
	var input buildEnvironmentConfigInput
	if !bindJSON(ctx, &input) {
		return
	}
	existing, err := h.findBuildEnvironmentConfig(h.db, scope, scopeRef)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	config, ok := h.buildEnvironmentConfigFromInput(ctx, user, scope, scopeRef, input, decodeSecretRefs(existing.SecretRefs))
	if !ok {
		return
	}
	if existing.ID != "" {
		config.ID = existing.ID
		config.CreatedAt = existing.CreatedAt
	}
	if err := h.db.Save(&config).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(user.ID, "build_environment.update", scope+":"+scopeRef, true, "")
	ctx.JSON(http.StatusOK, buildEnvironmentConfigResponseFromModel(config))
}

func (h *Handlers) authorizeBuildEnvironmentConfig(ctx *gin.Context, user model.User) (string, string, string, bool) {
	scope := strings.ToLower(strings.TrimSpace(ctx.Query("scope")))
	switch scope {
	case model.BuildEnvironmentScopeGlobal:
		if !authz.IsPlatformAdmin(user.Role) {
			writeError(ctx, http.StatusForbidden, "只有平台管理员可以维护全局构建变量和密钥")
			return "", "", "", false
		}
		return scope, model.BuildEnvironmentGlobalRef, "", true
	case model.BuildEnvironmentScopeApplication:
		projectID := strings.TrimSpace(ctx.Query("projectId"))
		applicationID := strings.TrimSpace(ctx.Query("applicationId"))
		if projectID == "" || applicationID == "" {
			writeError(ctx, http.StatusBadRequest, "项目空间和应用不能为空")
			return "", "", "", false
		}
		if !h.canManageBuildEnvironmentProject(ctx, user, projectID) {
			return "", "", "", false
		}
		var count int64
		if err := h.db.Model(&model.Application{}).Where("id = ? and project_id = ?", applicationID, projectID).Count(&count).Error; err != nil || count == 0 {
			writeError(ctx, http.StatusNotFound, "application not found")
			return "", "", "", false
		}
		return scope, applicationID, projectID, true
	case model.BuildEnvironmentScopeDeployment:
		projectID := strings.TrimSpace(ctx.Query("projectId"))
		applicationID := strings.TrimSpace(ctx.Query("applicationId"))
		targetID := strings.TrimSpace(ctx.Query("deploymentTargetId"))
		if projectID == "" || applicationID == "" || targetID == "" {
			writeError(ctx, http.StatusBadRequest, "项目空间、应用和部署配置不能为空")
			return "", "", "", false
		}
		if !h.canManageBuildEnvironmentProject(ctx, user, projectID) {
			return "", "", "", false
		}
		var count int64
		if err := h.db.Model(&model.DeploymentTarget{}).Where("id = ? and project_id = ? and application_id = ?", targetID, projectID, applicationID).Count(&count).Error; err != nil || count == 0 {
			writeError(ctx, http.StatusNotFound, "deployment target not found")
			return "", "", "", false
		}
		return scope, targetID, projectID, true
	default:
		writeError(ctx, http.StatusBadRequest, "构建环境作用域无效")
		return "", "", "", false
	}
}

func (h *Handlers) canManageBuildEnvironmentProject(ctx *gin.Context, user model.User, projectID string) bool {
	if authz.IsPlatformAdmin(user.Role) {
		return true
	}
	var member model.ProjectMember
	if err := h.db.First(&member, "project_id = ? and user_id = ?", projectID, user.ID).Error; err == nil && projectRoleAllowed(member.Role, []string{"owner", "admin"}) {
		return true
	}
	writeError(ctx, http.StatusForbidden, "只有项目空间所有者或管理员可以维护构建变量和密钥")
	return false
}

func (h *Handlers) buildEnvironmentConfigFromInput(ctx *gin.Context, user model.User, scope, scopeRef string, input buildEnvironmentConfigInput, existingSecretRefs map[string]string) (model.BuildEnvironmentConfig, bool) {
	variables, ok := normalizeBuildVariables(ctx, input.Variables)
	if !ok {
		return model.BuildEnvironmentConfig{}, false
	}
	secretRefs, ok := h.buildEnvironmentSecretRefsFromInput(ctx, user, scope, scopeRef, input.Secrets, existingSecretRefs)
	if !ok {
		return model.BuildEnvironmentConfig{}, false
	}
	return model.BuildEnvironmentConfig{
		ID:         id.New("bec"),
		Scope:      scope,
		ScopeRef:   scopeRef,
		Variables:  buildenv.Encode(variables),
		SecretRefs: buildenv.Encode(secretRefs),
		UpdatedBy:  user.ID,
	}, true
}

func (h *Handlers) deploymentBuildEnvironmentFromInput(ctx *gin.Context, user model.User, projectID, targetID string, input deploymentTargetInput, existing *model.BuildEnvironmentConfig) (*model.BuildEnvironmentConfig, bool) {
	if input.BuildVariables == nil && input.BuildSecrets == nil {
		return nil, true
	}
	if !h.canManageBuildEnvironmentProject(ctx, user, projectID) {
		return nil, false
	}
	if !h.requireStepUp(ctx, user, stepUpPurposeSecretUpdate) {
		return nil, false
	}
	values := buildEnvironmentConfigInput{Variables: map[string]string{}, Secrets: map[string]string{}}
	existingRefs := map[string]string{}
	if existing != nil && existing.ID != "" {
		values.Variables = buildenv.Decode(existing.Variables)
		existingRefs = buildenv.Decode(existing.SecretRefs)
		for key := range existingRefs {
			values.Secrets[key] = ""
		}
	}
	if input.BuildVariables != nil {
		values.Variables = *input.BuildVariables
	}
	if input.BuildSecrets != nil {
		values.Secrets = *input.BuildSecrets
	}
	config, ok := h.buildEnvironmentConfigFromInput(ctx, user, model.BuildEnvironmentScopeDeployment, targetID, values, existingRefs)
	if !ok {
		return nil, false
	}
	if existing != nil && existing.ID != "" {
		config.ID = existing.ID
		config.CreatedAt = existing.CreatedAt
	}
	return &config, true
}

func (h *Handlers) buildEnvironmentSecretRefsFromInput(ctx *gin.Context, user model.User, scope, scopeRef string, input, existing map[string]string) (map[string]string, bool) {
	output := make(map[string]string)
	for key, value := range input {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" && value == "" {
			continue
		}
		if !buildenv.IsKey(key) {
			writeError(ctx, http.StatusBadRequest, "构建密钥名只能使用字母、数字和下划线，且不能以数字开头")
			return nil, false
		}
		if value == "" {
			if existingRef := strings.TrimSpace(existing[key]); existingRef != "" {
				output[key] = existingRef
			}
			continue
		}
		if len(value) > 8192 {
			writeError(ctx, http.StatusBadRequest, "构建密钥值过长")
			return nil, false
		}
		ref := h.secrets.Store(value, user.ID, "build_environment:"+scope+":"+scopeRef+":"+key)
		if strings.TrimSpace(ref) == "" {
			writeError(ctx, http.StatusInternalServerError, "构建密钥保存失败")
			return nil, false
		}
		output[key] = ref
	}
	return output, true
}

func (h *Handlers) findBuildEnvironmentConfig(db *gorm.DB, scope, scopeRef string) (model.BuildEnvironmentConfig, error) {
	var config model.BuildEnvironmentConfig
	err := db.First(&config, "scope = ? and scope_ref = ?", scope, scopeRef).Error
	return config, err
}

func buildEnvironmentConfigResponseFromModel(config model.BuildEnvironmentConfig) buildEnvironmentConfigResponse {
	secrets := map[string]bool{}
	for key, ref := range buildenv.Decode(config.SecretRefs) {
		if buildenv.IsKey(key) && strings.TrimSpace(ref) != "" {
			secrets[key] = true
		}
	}
	return buildEnvironmentConfigResponse{
		Scope:     config.Scope,
		ScopeRef:  config.ScopeRef,
		Variables: buildenv.Decode(config.Variables),
		Secrets:   secrets,
	}
}
