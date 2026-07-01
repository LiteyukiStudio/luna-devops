package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/tasks"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *Handlers) ListProjectRuntimeConfigSets(ctx *gin.Context) {
	project, ok := h.findProjectForCurrentUser(ctx)
	if !ok {
		return
	}
	var sets []model.ProjectRuntimeConfigSet
	query := h.db.Model(&model.ProjectRuntimeConfigSet{}).Where("project_id = ?", project.ID)
	query = applySearch(ctx, query, "name", "env_vars", "config_files")
	if paginationRequested(ctx) {
		pagination := paginationFromQuery(ctx)
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
		return
	}
	if err := query.Order("created_at desc").Find(&sets).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, projectRuntimeConfigSetResponses(sets))
}

func (h *Handlers) CreateProjectRuntimeConfigSet(ctx *gin.Context) {
	project, ok := h.findProjectForCurrentUserWithRoles(ctx, "owner", "admin", "developer")
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
	set, ok := h.projectRuntimeConfigSetFromInput(ctx, user, project.ID, input, id.New("prcs"), nil, nil)
	if !ok {
		return
	}
	set.CreatedBy = user.ID
	if err := h.db.Create(&set).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	ctx.JSON(http.StatusCreated, projectRuntimeConfigSetResponseFor(set))
}

func (h *Handlers) UpdateProjectRuntimeConfigSet(ctx *gin.Context) {
	project, ok := h.findProjectForCurrentUserWithRoles(ctx, "owner", "admin", "developer")
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
	if err := h.db.First(&existing, "id = ? and project_id = ?", ctx.Param("setId"), project.ID).Error; err != nil {
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
	next, ok := h.projectRuntimeConfigSetFromInput(ctx, user, project.ID, input, existing.ID, decodeSecretRefs(existing.SecretRefs), decodeSecretRefs(existing.SecretFiles))
	if !ok {
		return
	}
	existing.Name = next.Name
	existing.EnvVars = next.EnvVars
	existing.ConfigFiles = next.ConfigFiles
	existing.SecretRefs = next.SecretRefs
	existing.SecretFiles = next.SecretFiles
	existing.Enabled = next.Enabled
	if err := h.db.Save(&existing).Error; err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	response := projectRuntimeConfigSetResponseFor(existing)
	response.AffectedDeploymentTargetCount = h.countRuntimeConfigSetDeploymentTargets(project.ID, existing.ID)
	ctx.JSON(http.StatusOK, response)
}

func (h *Handlers) DeleteProjectRuntimeConfigSet(ctx *gin.Context) {
	project, ok := h.findProjectForCurrentUserWithRoles(ctx, "owner", "admin", "developer")
	if !ok {
		return
	}
	if !h.ensureProjectCanMutate(ctx, project) {
		return
	}
	var set model.ProjectRuntimeConfigSet
	if err := h.db.First(&set, "id = ? and project_id = ?", ctx.Param("setId"), project.ID).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "运行配置集不存在")
		return
	}
	if !deleteStatusCanStart(set.DeleteStatus) {
		writeError(ctx, http.StatusConflict, "运行配置正在删除中，请等待资源清理完成")
		return
	}
	if err := markResourceDeleting(h.db, &model.ProjectRuntimeConfigSet{}, set.ID); err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if !h.enqueueResourceCleanup(ctx.Request.Context(), tasks.ResourceCleanupPayload{
		ResourceType: "runtime_config",
		ResourceID:   set.ID,
		ProjectID:    set.ProjectID,
		ActorID:      set.CreatedBy,
	}) {
		_ = markResourceDeleteFailed(h.db, &model.ProjectRuntimeConfigSet{}, set.ID, "资源清理任务投递失败，请稍后重试")
		writeError(ctx, http.StatusServiceUnavailable, "资源清理任务投递失败，请稍后重试")
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (h *Handlers) projectRuntimeConfigSetFromInput(ctx *gin.Context, user model.User, projectID string, input projectRuntimeConfigSetInput, setID string, existingSecretRefs map[string]string, existingSecretFiles map[string]string) (model.ProjectRuntimeConfigSet, bool) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		writeError(ctx, http.StatusBadRequest, "请输入运行配置集名称")
		return model.ProjectRuntimeConfigSet{}, false
	}
	configFiles, ok := normalizeRuntimeConfigFilesInput(ctx, input.ConfigFiles)
	if !ok {
		return model.ProjectRuntimeConfigSet{}, false
	}
	secretRefs, ok := h.runtimeSecretRefsFromInput(ctx, user, setID, input.SecretRefs, existingSecretRefs)
	if !ok {
		return model.ProjectRuntimeConfigSet{}, false
	}
	secretFiles, ok := h.runtimeSecretFilesFromInput(ctx, user, setID, input.SecretFiles, existingSecretFiles)
	if !ok {
		return model.ProjectRuntimeConfigSet{}, false
	}
	secretRefsContent, err := json.Marshal(secretRefs)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
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
		EnvVars:     strings.TrimSpace(input.EnvVars),
		ConfigFiles: configFiles,
		SecretRefs:  string(secretRefsContent),
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
		writeError(ctx, http.StatusBadRequest, "配置文件必须使用文件数组格式")
		return "", false
	}
	var raw []runtimeConfigFileInput
	if err := json.Unmarshal([]byte(normalized), &raw); err != nil {
		writeError(ctx, http.StatusBadRequest, "配置文件格式无效")
		return "", false
	}
	seenPaths := map[string]bool{}
	for _, item := range raw {
		filePath, ok := normalizeRuntimeConfigFilePathInput(ctx, item.Path)
		if !ok {
			return "", false
		}
		if seenPaths[filePath] {
			writeError(ctx, http.StatusBadRequest, "配置文件路径不能重复")
			return "", false
		}
		seenPaths[filePath] = true
	}
	return normalized, true
}

func normalizeRuntimeConfigFilePathInput(ctx *gin.Context, value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || !strings.HasPrefix(trimmed, "/") {
		writeError(ctx, http.StatusBadRequest, "配置文件路径必须使用绝对路径")
		return "", false
	}
	cleaned := path.Clean(trimmed)
	if cleaned == "/" {
		writeError(ctx, http.StatusBadRequest, "配置文件路径不能是根目录")
		return "", false
	}
	return cleaned, true
}

func (h *Handlers) runtimeSecretRefsFromInput(ctx *gin.Context, user model.User, ownerID string, value string, existing map[string]string) (map[string]string, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return copyStringMap(existing), true
	}
	if trimmed == "{}" {
		return map[string]string{}, true
	}
	parsed, ok := parseRuntimeKeyValueInput(ctx, trimmed, "密钥变量格式无效")
	if !ok {
		return nil, false
	}
	output := map[string]string{}
	for key, item := range parsed {
		if !isBuildEnvKey(key) {
			writeError(ctx, http.StatusBadRequest, "密钥变量名只能使用字母、数字和下划线，且不能以数字开头")
			return nil, false
		}
		if strings.TrimSpace(item) == "" {
			if existingRef := strings.TrimSpace(existing[key]); existingRef != "" {
				output[key] = existingRef
			}
			continue
		}
		output[key] = h.secrets.Store(item, user.ID, "runtime_config:"+ownerID+":secret:"+key)
	}
	return output, true
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
		output[filePath] = h.secrets.Store(content, user.ID, "runtime_config:"+ownerID+":file:"+filePath)
	}
	return output, true
}

func parseRuntimeKeyValueInput(ctx *gin.Context, value string, errorMessage string) (map[string]string, bool) {
	if strings.HasPrefix(value, "{") {
		var raw map[string]any
		if err := json.Unmarshal([]byte(value), &raw); err != nil {
			writeError(ctx, http.StatusBadRequest, errorMessage)
			return nil, false
		}
		output := map[string]string{}
		for key, item := range raw {
			output[strings.TrimSpace(key)] = fmt.Sprint(item)
		}
		return output, true
	}
	output := map[string]string{}
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, item, ok := strings.Cut(line, "=")
		if !ok {
			writeError(ctx, http.StatusBadRequest, errorMessage)
			return nil, false
		}
		output[strings.TrimSpace(key)] = strings.TrimSpace(item)
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
	Name        string `json:"name" binding:"required"`
	EnvVars     string `json:"envVars"`
	ConfigFiles string `json:"configFiles"`
	SecretRefs  string `json:"secretRefs"`
	SecretFiles string `json:"secretFiles"`
	Enabled     bool   `json:"enabled"`
}

type projectRuntimeConfigSetResponse struct {
	ID                            string    `json:"id"`
	ProjectID                     string    `json:"projectId"`
	Name                          string    `json:"name"`
	EnvVars                       string    `json:"envVars"`
	ConfigFiles                   string    `json:"configFiles"`
	SecretRefsSet                 bool      `json:"secretRefsSet"`
	SecretFilesSet                bool      `json:"secretFilesSet"`
	Enabled                       bool      `json:"enabled"`
	DeleteStatus                  string    `json:"deleteStatus"`
	DeleteMessage                 string    `json:"deleteMessage"`
	CreatedBy                     string    `json:"createdBy"`
	CreatedAt                     time.Time `json:"createdAt"`
	AffectedDeploymentTargetCount int       `json:"affectedDeploymentTargetCount,omitempty"`
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
		ID:             set.ID,
		ProjectID:      set.ProjectID,
		Name:           set.Name,
		EnvVars:        set.EnvVars,
		ConfigFiles:    set.ConfigFiles,
		SecretRefsSet:  strings.TrimSpace(set.SecretRefs) != "" && strings.TrimSpace(set.SecretRefs) != "{}",
		SecretFilesSet: strings.TrimSpace(set.SecretFiles) != "" && strings.TrimSpace(set.SecretFiles) != "{}",
		Enabled:        set.Enabled,
		DeleteStatus:   set.DeleteStatus,
		DeleteMessage:  set.DeleteMessage,
		CreatedBy:      set.CreatedBy,
		CreatedAt:      set.CreatedAt,
	}
}

func (h *Handlers) countRuntimeConfigSetDeploymentTargets(projectID string, setID string) int {
	var targets []model.DeploymentTarget
	if err := h.db.Select("runtime_config_set_ids", "runtime_config_refs").Where("project_id = ?", projectID).Find(&targets).Error; err != nil {
		return 0
	}
	count := 0
	for _, target := range targets {
		liveIDs := model.DeploymentRuntimeConfigLiveSetIDs(model.DecodeDeploymentRuntimeConfigRefs(target.RuntimeConfigRefs))
		if len(liveIDs) == 0 && strings.TrimSpace(target.RuntimeConfigRefs) == "" {
			liveIDs = buildVariableSetIDs(target.RuntimeConfigSetIDs)
		}
		for _, id := range liveIDs {
			if id == setID {
				count++
				break
			}
		}
	}
	return count
}
