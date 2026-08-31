package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *Handlers) ListBuildVariableSets(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	visibility, ok := resolveListVisibility(ctx, user)
	if !ok {
		return
	}
	projectID := strings.TrimSpace(ctx.Query("projectId"))

	query := h.dbFor(ctx).Model(&model.BuildVariableSet{})
	var visible bool
	query, visible = h.applyScopedResourceListVisibility(ctx, query, scopedResourceBuildVariableSet, user, projectID, visibility)
	if !visible {
		return
	}
	query = applySearch(ctx, query, "name")

	var sets []model.BuildVariableSet
	pagination := paginationFromQueryWithSort(ctx, map[string]string{"name": "name", "scope": "scope", "createdAt": "created_at"}, "createdAt")
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if err := query.Order(orderByClause(pagination, map[string]string{
		"name":      "name",
		"scope":     "scope",
		"createdAt": "created_at",
	}, "created_at")).Limit(pagination.PageSize).Offset(pagination.Offset()).Find(&sets).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.attachBuildVariableSetProjects(sets, ctx.Request.Context())
	responses, err := h.buildVariableSetResponsesForUser(user, sets, ctx.Request.Context())
	if err != nil {
		writeProjectAuthorizationError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, paginatedResponse(responses, total, pagination))
}

func (h *Handlers) CreateBuildVariableSet(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var input buildVariableSetInput
	if !bindJSON(ctx, &input) {
		return
	}
	setID := id.New("bvs")
	set, ok := h.buildVariableSetFromInput(ctx, user, input, setID, nil)
	if !ok {
		return
	}
	set.CreatedBy = user.ID
	if err := h.saveBuildVariableSet(set, ctx.Request.Context()); err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	response, err := h.buildVariableSetResponseForUser(user, set, ctx.Request.Context())
	if err != nil {
		writeProjectAuthorizationError(ctx, err)
		return
	}
	ctx.JSON(http.StatusCreated, response)
}

func (h *Handlers) UpdateBuildVariableSet(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var existing model.BuildVariableSet
	if err := h.dbFor(ctx).First(&existing, "id = ?", ctx.Param("setId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "build variable set not found")
		return
	}
	if !h.canManageScopedResourceByID(ctx, user, existing.Scope, existing.OwnerRef, scopedResourceBuildVariableSet, existing.ID, "无权维护该变量和密钥") {
		return
	}
	var input buildVariableSetInput
	if !bindJSON(ctx, &input) {
		return
	}
	next, ok := h.buildVariableSetFromInput(ctx, user, input, existing.ID, decodeSecretRefs(existing.SecretRefs))
	if !ok {
		return
	}
	existing.Name = next.Name
	existing.Scope = next.Scope
	existing.OwnerRef = next.OwnerRef
	existing.ProjectIDs = next.ProjectIDs
	existing.Variables = next.Variables
	existing.SecretRefs = next.SecretRefs
	existing.Enabled = next.Enabled
	if err := h.saveBuildVariableSet(existing, ctx.Request.Context()); err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	response, err := h.buildVariableSetResponseForUser(user, existing, ctx.Request.Context())
	if err != nil {
		writeProjectAuthorizationError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, response)
}

func (h *Handlers) DeleteBuildVariableSet(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	var set model.BuildVariableSet
	if err := h.dbFor(ctx).First(&set, "id = ?", ctx.Param("setId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "build variable set not found")
		return
	}
	if !h.canManageScopedResourceByID(ctx, user, set.Scope, set.OwnerRef, scopedResourceBuildVariableSet, set.ID, "无权维护该变量和密钥") {
		return
	}
	if err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		var targets []model.DeploymentTarget
		if err := tx.Select("id", "build_variable_set_ids").Find(&targets).Error; err != nil {
			return err
		}
		for _, target := range targets {
			nextIDs := removeBuildVariableSetID(target.BuildVariableSetIDs, set.ID)
			if nextIDs != target.BuildVariableSetIDs {
				if err := tx.Model(&model.DeploymentTarget{}).Where("id = ?", target.ID).Update("build_variable_set_ids", nextIDs).Error; err != nil {
					return err
				}
			}
		}
		if err := tx.Where("resource_type = ? and resource_id = ?", scopedResourceBuildVariableSet, set.ID).Delete(&model.ScopedResourceProjectBinding{}).Error; err != nil {
			return err
		}
		return tx.Delete(&set).Error
	}); err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (h *Handlers) buildVariableSetFromInput(ctx *gin.Context, user model.User, input buildVariableSetInput, setID string, existingSecretRefs map[string]string) (model.BuildVariableSet, bool) {
	scope, ownerRef, projectIDs, ok := h.normalizeScopedOwnerWithProjects(ctx, user, input.Scope, input.OwnerRef, input.ProjectIDs, "只有平台管理员可以维护全局变量和密钥")
	if !ok {
		return model.BuildVariableSet{}, false
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		writeError(ctx, http.StatusBadRequest, "请输入变量和密钥名称")
		return model.BuildVariableSet{}, false
	}
	variables, ok := normalizeBuildVariables(ctx, input.Variables)
	if !ok {
		return model.BuildVariableSet{}, false
	}
	secretRefs, ok := h.buildVariableSecretRefsFromInput(ctx, user, setID, input.Secrets, existingSecretRefs)
	if !ok {
		return model.BuildVariableSet{}, false
	}
	if len(variables) == 0 && len(secretRefs) == 0 {
		writeError(ctx, http.StatusBadRequest, "请至少配置一个构建变量或密钥")
		return model.BuildVariableSet{}, false
	}
	content, err := json.Marshal(variables)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return model.BuildVariableSet{}, false
	}
	secretContent, err := json.Marshal(secretRefs)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return model.BuildVariableSet{}, false
	}
	return model.BuildVariableSet{
		ID:         setID,
		Name:       name,
		Scope:      scope,
		OwnerRef:   ownerRef,
		ProjectIDs: projectIDs,
		Variables:  string(content),
		SecretRefs: string(secretContent),
		Enabled:    input.Enabled,
	}, true
}

func (h *Handlers) saveBuildVariableSet(set model.BuildVariableSet, ctx context.Context) error {
	return h.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&set).Error; err != nil {
			return err
		}
		return h.replaceScopedResourceProjectBindings(tx, scopedResourceBuildVariableSet, set.ID, sortedProjectIDs(set.ProjectIDs), nil)
	})
}

func (h *Handlers) attachBuildVariableSetProjects(sets []model.BuildVariableSet, ctx context.Context) {
	projectMap := h.scopedResourceProjectIDMap(scopedResourceBuildVariableSet, buildVariableSetModelIDs(sets), ctx)
	for index := range sets {
		sets[index].ProjectIDs = projectMap[sets[index].ID]
	}
}

func buildVariableSetModelIDs(sets []model.BuildVariableSet) []string {
	ids := make([]string, 0, len(sets))
	for _, set := range sets {
		ids = append(ids, set.ID)
	}
	return ids
}

func (h *Handlers) buildVariableSecretRefsFromInput(ctx *gin.Context, user model.User, setID string, input map[string]string, existing map[string]string) (map[string]string, bool) {
	output := make(map[string]string)
	for key, value := range input {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" && value == "" {
			continue
		}
		if !isBuildEnvKey(key) {
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
		output[key] = h.secrets.StoreContext(ctx.Request.Context(), value, user.ID, "build_variable_set:"+setID+":"+key)
	}
	return output, true
}
