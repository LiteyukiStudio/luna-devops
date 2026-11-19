package buildapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (h *Handlers) ListBuildVariableSets(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	visibility, ok := h.resolveListVisibility(ctx, user)
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
		h.writeProjectAuthorizationError(ctx, err)
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
	set, secretInputs, ok := h.buildVariableSetDraftFromInput(ctx, user, input, setID, nil)
	if !ok {
		return
	}
	set.CreatedBy = user.ID
	err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		secretRefs := make(map[string]string, len(secretInputs))
		for key, value := range secretInputs {
			if h.secretAvailable() {
				ref, storeErr := h.storeSecretWithDB(ctx.Request.Context(), tx, value, user.ID, "build_variable_set:"+set.ID+":"+key)
				if storeErr != nil {
					return storeErr
				}
				secretRefs[key] = ref
			} else {
				secretRefs[key] = ""
			}
		}
		secretContent, marshalErr := json.Marshal(secretRefs)
		if marshalErr != nil {
			return marshalErr
		}
		set.SecretRefs = string(secretContent)
		if err := tx.Save(&set).Error; err != nil {
			return err
		}
		return h.replaceScopedResourceProjectBindings(tx, scopedResourceBuildVariableSet, set.ID, sortedProjectIDs(set.ProjectIDs), nil)
	})
	if err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	response, err := h.buildVariableSetResponseForUser(user, set, ctx.Request.Context())
	if err != nil {
		h.writeProjectAuthorizationError(ctx, err)
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
	next, secretInputs, ok := h.buildVariableSetDraftFromInput(ctx, user, input, existing.ID, decodeSecretRefs(existing.SecretRefs))
	if !ok {
		return
	}
	var updated model.BuildVariableSet
	err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		var locked model.BuildVariableSet
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "id = ?", existing.ID).Error; err != nil {
			return err
		}
		oldRefs := decodeSecretRefs(locked.SecretRefs)
		nextRefs := make(map[string]string, len(secretInputs))
		for key, value := range secretInputs {
			if value == "" {
				if existingRef := strings.TrimSpace(oldRefs[key]); existingRef != "" {
					nextRefs[key] = existingRef
				}
				continue
			}
			if h.secretAvailable() {
				resource := "build_variable_set:" + locked.ID + ":" + key
				newRef, storeErr := h.storeSecretWithDB(ctx.Request.Context(), tx, value, user.ID, resource)
				if storeErr != nil {
					return storeErr
				}
				nextRefs[key] = newRef
			} else {
				nextRefs[key] = ""
			}
		}
		for key, oldRef := range oldRefs {
			if strings.TrimSpace(oldRef) == "" {
				continue
			}
			if nextRefs[key] == oldRef {
				continue
			}
			if err := h.deleteSecretRefWithDB(ctx.Request.Context(), tx, oldRef, "build_variable_set:"+locked.ID+":"+key); err != nil {
				return err
			}
		}
		secretContent, marshalErr := json.Marshal(nextRefs)
		if marshalErr != nil {
			return marshalErr
		}
		locked.Name = next.Name
		locked.Scope = next.Scope
		locked.OwnerRef = next.OwnerRef
		locked.ProjectIDs = next.ProjectIDs
		locked.Variables = next.Variables
		locked.SecretRefs = string(secretContent)
		locked.Enabled = next.Enabled
		if err := tx.Save(&locked).Error; err != nil {
			return err
		}
		projectIDs := sortedProjectIDs(locked.ProjectIDs)
		if err := h.replaceScopedResourceProjectBindings(tx, scopedResourceBuildVariableSet, locked.ID, projectIDs, nil); err != nil {
			return err
		}
		if err := pruneInaccessibleBuildVariableSetTargets(tx, locked, projectIDs); err != nil {
			return err
		}
		updated = locked
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(ctx, http.StatusNotFound, "build variable set not found")
		return
	}
	if err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	response, err := h.buildVariableSetResponseForUser(user, updated, ctx.Request.Context())
	if err != nil {
		h.writeProjectAuthorizationError(ctx, err)
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
		var locked model.BuildVariableSet
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "id = ?", set.ID).Error; err != nil {
			return err
		}
		var targets []model.DeploymentTarget
		if err := tx.Select("id", "build_variable_set_ids").Find(&targets).Error; err != nil {
			return err
		}
		for _, target := range targets {
			nextIDs := removeBuildVariableSetID(target.BuildVariableSetIDs, locked.ID)
			if nextIDs != target.BuildVariableSetIDs {
				if err := tx.Model(&model.DeploymentTarget{}).Where("id = ?", target.ID).Update("build_variable_set_ids", nextIDs).Error; err != nil {
					return err
				}
			}
		}
		for key, ref := range decodeSecretRefs(locked.SecretRefs) {
			if strings.TrimSpace(ref) == "" {
				continue
			}
			if err := h.deleteSecretRefWithDB(ctx.Request.Context(), tx, ref, "build_variable_set:"+locked.ID+":"+key); err != nil {
				return err
			}
		}
		if err := tx.Where("resource_type = ? and resource_id = ?", scopedResourceBuildVariableSet, locked.ID).Delete(&model.ScopedResourceProjectBinding{}).Error; err != nil {
			return err
		}
		return tx.Delete(&locked).Error
	}); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(ctx, http.StatusNotFound, "build variable set not found")
			return
		}
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (h *Handlers) buildVariableSetDraftFromInput(ctx *gin.Context, user model.User, input buildVariableSetInput, setID string, existingSecretRefs map[string]string) (model.BuildVariableSet, map[string]string, bool) {
	scope, ownerRef, projectIDs, ok := h.normalizeScopedOwnerWithProjects(ctx, user, input.Scope, input.OwnerRef, input.ProjectIDs, "只有平台管理员可以维护全局变量和密钥")
	if !ok {
		return model.BuildVariableSet{}, nil, false
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		writeError(ctx, http.StatusBadRequest, "请输入变量和密钥名称")
		return model.BuildVariableSet{}, nil, false
	}
	variables, ok := normalizeBuildVariables(ctx, input.Variables)
	if !ok {
		return model.BuildVariableSet{}, nil, false
	}
	secretInputs, ok := normalizeBuildVariableSecretInputs(ctx, input.Secrets, existingSecretRefs)
	if !ok {
		return model.BuildVariableSet{}, nil, false
	}
	if len(variables) == 0 && len(secretInputs) == 0 {
		writeError(ctx, http.StatusBadRequest, "请至少配置一个构建变量或密钥")
		return model.BuildVariableSet{}, nil, false
	}
	content, err := json.Marshal(variables)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return model.BuildVariableSet{}, nil, false
	}
	return model.BuildVariableSet{
		ID:         setID,
		Name:       name,
		Scope:      scope,
		OwnerRef:   ownerRef,
		ProjectIDs: projectIDs,
		Variables:  string(content),
		Enabled:    input.Enabled,
	}, secretInputs, true
}

func pruneInaccessibleBuildVariableSetTargets(tx *gorm.DB, set model.BuildVariableSet, projectIDs []string) error {
	if set.Scope == "global" {
		return nil
	}
	query := tx.Select("id", "build_variable_set_ids")
	switch set.Scope {
	case "project":
		if len(projectIDs) > 0 {
			query = query.Where("project_id not in ?", projectIDs)
		}
	case "user":
		memberProjects := tx.Model(&model.ProjectMember{}).Select("project_id").Where("user_id = ?", set.OwnerRef)
		query = query.Where("project_id not in (?) or created_by <> ?", memberProjects, set.OwnerRef)
	}
	var targets []model.DeploymentTarget
	if err := query.Find(&targets).Error; err != nil {
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
	return nil
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

func normalizeBuildVariableSecretInputs(ctx *gin.Context, input map[string]string, existing map[string]string) (map[string]string, bool) {
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
			if strings.TrimSpace(existing[key]) != "" {
				output[key] = ""
			}
			continue
		}
		if len(value) > 8192 {
			writeError(ctx, http.StatusBadRequest, "构建密钥值过长")
			return nil, false
		}
		output[key] = value
	}
	return output, true
}
