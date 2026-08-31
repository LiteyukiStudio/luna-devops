package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/runtimeconfig"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (h *Handlers) ListProjectRuntimeConfigSets(ctx *gin.Context) {
	_, project, ok := h.authorizeProject(ctx, authz.ActionSecretReadSummary)
	if !ok {
		return
	}
	var sets []model.ProjectRuntimeConfigSet
	query := h.dbFor(ctx).Model(&model.ProjectRuntimeConfigSet{}).Where("project_id = ?", project.ID)
	query = applySearch(ctx, query, "name", "env_vars", "config_files")
	pagination := paginationFromQueryWithSort(ctx, map[string]string{"name": "name", "createdAt": "created_at"}, "createdAt")
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if err := query.Order(orderByClause(pagination, map[string]string{
		"name":      "name",
		"createdAt": "created_at",
	}, "created_at")).Limit(pagination.PageSize).Offset(pagination.Offset()).Find(&sets).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, paginatedResponse(projectRuntimeConfigSetResponses(sets), total, pagination))
}

func (h *Handlers) CreateProjectRuntimeConfigSet(ctx *gin.Context) {
	_, project, ok := h.authorizeProject(ctx, authz.ActionSecretUpdate)
	if !ok {
		return
	}
	if !h.ensureProjectCanMutate(ctx, project) {
		return
	}
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var input projectRuntimeConfigSetInput
	if !bindJSON(ctx, &input) {
		return
	}
	set, ok := h.projectRuntimeConfigSetFromInput(ctx, user, project.ID, input, id.New("prcs"), nil)
	if !ok {
		return
	}
	set.CreatedBy = user.ID
	if err := h.dbFor(ctx).Create(&set).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	ctx.JSON(http.StatusCreated, projectRuntimeConfigSetResponseFor(set))
}

func (h *Handlers) UpdateProjectRuntimeConfigSet(ctx *gin.Context) {
	_, project, ok := h.authorizeProject(ctx, authz.ActionSecretUpdate)
	if !ok {
		return
	}
	if !h.ensureProjectCanMutate(ctx, project) {
		return
	}
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var existing model.ProjectRuntimeConfigSet
	if err := h.dbFor(ctx).First(&existing, "id = ? and project_id = ?", ctx.Param("setId"), project.ID).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "运行配置集不存在")
		return
	}
	if !h.ensureRuntimeConfigSetCanMutate(ctx, existing) {
		return
	}
	var input projectRuntimeConfigSetInput
	if !bindJSON(ctx, &input) {
		return
	}
	next, ok := h.projectRuntimeConfigSetFromInput(ctx, user, project.ID, input, existing.ID, decodeSecretRefs(existing.SecretFiles))
	if !ok {
		return
	}
	existing.Name = next.Name
	existing.EnvVars = next.EnvVars
	existing.ConfigFiles = next.ConfigFiles
	existing.SecretFiles = next.SecretFiles
	existing.Enabled = next.Enabled
	if err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		var current model.ProjectRuntimeConfigSet
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id", "secret_refs").First(&current, "id = ? and project_id = ?", existing.ID, project.ID).Error; err != nil {
			return err
		}
		existing.SecretRefs = current.SecretRefs
		return tx.Save(&existing).Error
	}); err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	response := projectRuntimeConfigSetResponseFor(existing)
	response.AffectedDeploymentTargetCount = h.countRuntimeConfigSetDeploymentTargets(project.ID, existing.ID, ctx.Request.Context())
	ctx.JSON(http.StatusOK, response)
}

func (h *Handlers) DeleteProjectRuntimeConfigSet(ctx *gin.Context) {
	user, project, ok := h.authorizeProject(ctx, authz.ActionSecretUpdate)
	if !ok {
		return
	}
	if !h.ensureProjectCanMutate(ctx, project) {
		return
	}
	var set model.ProjectRuntimeConfigSet
	if err := h.dbFor(ctx).First(&set, "id = ? and project_id = ?", ctx.Param("setId"), project.ID).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "运行配置集不存在")
		return
	}
	if !deleteStatusCanStart(set.DeleteStatus) {
		writeError(ctx, http.StatusConflict, "运行配置正在删除中，请等待资源清理完成")
		return
	}
	if err := markResourceDeleting(h.dbFor(ctx), &model.ProjectRuntimeConfigSet{}, set.ID); err != nil {
		if errors.Is(err, errResourceDeleteAlreadyStarted) {
			writeErrorCode(ctx, http.StatusConflict, "runtime_config.delete_in_progress", "运行配置正在删除中，请等待资源清理完成")
			return
		}
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if !h.enqueueResourceCleanup(ctx.Request.Context(), "runtime_config", set.ID, set.ProjectID, user.ID) {
		_ = markResourceDeleteFailed(h.dbFor(ctx), &model.ProjectRuntimeConfigSet{}, set.ID, "资源清理任务投递失败，请稍后重试")
		h.auditWithContext(user.ID, "runtime_config.delete", set.ID, false, "cleanup_enqueue_failed", ctx.Request.Context())
		writeError(ctx, http.StatusServiceUnavailable, "资源清理任务投递失败，请稍后重试")
		return
	}
	h.auditWithContext(user.ID, "runtime_config.delete", set.ID, true, "cleanup_queued", ctx.Request.Context())
	ctx.Status(http.StatusNoContent)
}

func (h *Handlers) projectRuntimeConfigSetFromInput(ctx *gin.Context, user model.User, projectID string, input projectRuntimeConfigSetInput, setID string, existingSecretFiles map[string]string) (model.ProjectRuntimeConfigSet, bool) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		writeError(ctx, http.StatusBadRequest, "请输入运行配置集名称")
		return model.ProjectRuntimeConfigSet{}, false
	}
	publicEnvironment, ok := normalizePublicEnvironmentVariables(ctx, input.EnvironmentVariables)
	if !ok {
		return model.ProjectRuntimeConfigSet{}, false
	}
	envVars, err := runtimeconfig.EncodeKeyValue(publicEnvironment)
	if err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "deployment.runtime_config_invalid", "运行时环境变量格式无效")
		return model.ProjectRuntimeConfigSet{}, false
	}
	configFiles, ok := normalizeRuntimeConfigFilesInput(ctx, input.ConfigFiles)
	if !ok {
		return model.ProjectRuntimeConfigSet{}, false
	}
	secretFiles, ok := h.runtimeSecretFilesFromInput(ctx, user, setID, input.SecretFiles, existingSecretFiles)
	if !ok {
		return model.ProjectRuntimeConfigSet{}, false
	}
	secretFilesContent, err := json.Marshal(secretFiles)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return model.ProjectRuntimeConfigSet{}, false
	}
	return model.ProjectRuntimeConfigSet{
		ID:          setID,
		ProjectID:   projectID,
		Name:        name,
		EnvVars:     envVars,
		ConfigFiles: configFiles,
		SecretFiles: string(secretFilesContent),
		Enabled:     input.Enabled,
	}, true
}

func normalizeRuntimeConfigFilesInput(ctx *gin.Context, value string) (string, bool) {
	normalized := strings.TrimSpace(value)
	if normalized == "" || normalized == "[]" {
		return "", true
	}
	if !strings.HasPrefix(normalized, "[") {
		writeErrorCode(ctx, http.StatusBadRequest, "deployment.runtime_config_files_invalid", "runtime configuration files are invalid")
		return "", false
	}
	var raw []runtimeConfigFileInput
	if err := json.Unmarshal([]byte(normalized), &raw); err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "deployment.runtime_config_files_invalid", "runtime configuration files are invalid")
		return "", false
	}
	seenPaths := map[string]bool{}
	for _, item := range raw {
		filePath, ok := normalizeRuntimeConfigFilePathInput(ctx, item.Path)
		if !ok {
			return "", false
		}
		if seenPaths[filePath] {
			writeErrorCode(ctx, http.StatusBadRequest, "deployment.runtime_config_files_invalid", "runtime configuration files are invalid")
			return "", false
		}
		seenPaths[filePath] = true
	}
	return normalized, true
}

func normalizeRuntimeConfigFilePathInput(ctx *gin.Context, value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || !strings.HasPrefix(trimmed, "/") {
		writeErrorCode(ctx, http.StatusBadRequest, "deployment.runtime_config_path_invalid", "runtime configuration path is invalid")
		return "", false
	}
	cleaned := path.Clean(trimmed)
	if cleaned == "/" {
		writeErrorCode(ctx, http.StatusBadRequest, "deployment.runtime_config_path_invalid", "runtime configuration path is invalid")
		return "", false
	}
	return cleaned, true
}

func (h *Handlers) runtimeSecretFilesFromInput(ctx *gin.Context, user model.User, ownerID string, value string, existing map[string]string) (map[string]string, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return copyStringMap(existing), true
	}
	if trimmed == "[]" {
		return map[string]string{}, true
	}
	parsed, ok := parseRuntimeFileContentInput(ctx, trimmed, "密钥文件格式无效")
	if !ok {
		return nil, false
	}
	output := map[string]string{}
	for itemPath, content := range parsed {
		filePath, ok := normalizeRuntimeConfigFilePathInput(ctx, itemPath)
		if !ok {
			return nil, false
		}
		if strings.TrimSpace(content) == "" {
			if existingRef := strings.TrimSpace(existing[filePath]); existingRef != "" {
				output[filePath] = existingRef
			}
			continue
		}
		output[filePath] = h.secrets.StoreContext(ctx.Request.Context(), content, user.ID, "runtime_config:"+ownerID+":file:"+filePath)
	}
	return output, true
}

func parseRuntimeFileContentInput(ctx *gin.Context, value string, errorMessage string) (map[string]string, bool) {
	if !strings.HasPrefix(value, "[") {
		writeError(ctx, http.StatusBadRequest, errorMessage)
		return nil, false
	}
	var raw []runtimeConfigFileInput
	if err := json.Unmarshal([]byte(value), &raw); err != nil {
		writeError(ctx, http.StatusBadRequest, errorMessage)
		return nil, false
	}
	output := map[string]string{}
	seenPaths := map[string]bool{}
	for _, item := range raw {
		filePath, ok := normalizeRuntimeConfigFilePathInput(ctx, item.Path)
		if !ok {
			return nil, false
		}
		if seenPaths[filePath] {
			writeError(ctx, http.StatusBadRequest, "密钥文件路径不能重复")
			return nil, false
		}
		seenPaths[filePath] = true
		output[filePath] = item.Content
	}
	return output, true
}

func copyStringMap(values map[string]string) map[string]string {
	output := make(map[string]string, len(values))
	for key, value := range values {
		output[key] = value
	}
	return output
}

type runtimeConfigFileInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type projectRuntimeConfigSetInput struct {
	Name                 string                            `json:"name" binding:"required"`
	EnvironmentVariables []runtimeEnvironmentVariableInput `json:"environmentVariables"`
	ConfigFiles          string                            `json:"configFiles"`
	SecretFiles          string                            `json:"secretFiles"`
	Enabled              bool                              `json:"enabled"`
}

type projectRuntimeConfigSetResponse struct {
	ID                            string                               `json:"id"`
	ProjectID                     string                               `json:"projectId"`
	Name                          string                               `json:"name"`
	EnvironmentVariables          []runtimeEnvironmentVariableResponse `json:"environmentVariables"`
	ConfigFiles                   string                               `json:"configFiles"`
	SecretFilesSet                bool                                 `json:"secretFilesSet"`
	Enabled                       bool                                 `json:"enabled"`
	DeleteStatus                  string                               `json:"deleteStatus"`
	DeleteMessage                 string                               `json:"deleteMessage"`
	CreatedBy                     string                               `json:"createdBy"`
	CreatedAt                     time.Time                            `json:"createdAt"`
	AffectedDeploymentTargetCount int                                  `json:"affectedDeploymentTargetCount,omitempty"`
}

func projectRuntimeConfigSetResponses(sets []model.ProjectRuntimeConfigSet) []projectRuntimeConfigSetResponse {
	output := make([]projectRuntimeConfigSetResponse, 0, len(sets))
	for _, set := range sets {
		output = append(output, projectRuntimeConfigSetResponseFor(set))
	}
	return output
}

func projectRuntimeConfigSetResponseFor(set model.ProjectRuntimeConfigSet) projectRuntimeConfigSetResponse {
	return projectRuntimeConfigSetResponse{
		ID:                   set.ID,
		ProjectID:            set.ProjectID,
		Name:                 set.Name,
		EnvironmentVariables: runtimeEnvironmentVariables(set.EnvVars, set.SecretRefs),
		ConfigFiles:          set.ConfigFiles,
		SecretFilesSet:       strings.TrimSpace(set.SecretFiles) != "" && strings.TrimSpace(set.SecretFiles) != "{}",
		Enabled:              set.Enabled,
		DeleteStatus:         set.DeleteStatus,
		DeleteMessage:        set.DeleteMessage,
		CreatedBy:            set.CreatedBy,
		CreatedAt:            set.CreatedAt,
	}
}

func (h *Handlers) countRuntimeConfigSetDeploymentTargets(projectID string, setID string, ctx context.Context) int {
	var targets []model.DeploymentTarget
	if err := h.dbWithContext(ctx).Select("runtime_config_refs").Where("project_id = ?", projectID).Find(&targets).Error; err != nil {
		return 0
	}
	count := 0
	for _, target := range targets {
		liveIDs := model.DeploymentRuntimeConfigLiveSetIDs(model.DecodeDeploymentRuntimeConfigRefs(target.RuntimeConfigRefs))
		for _, id := range liveIDs {
			if id == setID {
				count++
				break
			}
		}
	}
	return count
}
