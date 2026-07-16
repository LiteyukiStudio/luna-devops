package api

import (
	"net/http"
	"sort"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	scopedResourceGitProvider        = "git_provider"
	scopedResourceGitAccount         = "git_account"
	scopedResourceArtifactRegistry   = "artifact_registry"
	scopedResourceRegistryCredential = "registry_credential"
	scopedResourceBuildVariableSet   = "build_variable_set"
	scopedResourceRuntimeCluster     = "runtime_cluster"
)

func (h *Handlers) normalizeScopedOwnerWithProjects(ctx *gin.Context, user model.User, rawScope, rawOwnerRef string, rawProjectIDs []string, globalError string) (string, string, []string, bool) {
	scope := normalizeOwnerScope(rawScope)
	ownerRef := strings.TrimSpace(rawOwnerRef)
	switch scope {
	case "global":
		if !authz.IsPlatformAdmin(user.Role) {
			writeError(ctx, http.StatusForbidden, globalError)
			return "", "", nil, false
		}
		return scope, "", nil, true
	case "user":
		return scope, user.ID, nil, true
	case "project":
		projectIDs := normalizeStringList(rawProjectIDs)
		if len(projectIDs) == 0 {
			writeError(ctx, http.StatusBadRequest, "请选择项目空间")
			return "", "", nil, false
		}
		if !h.canManageAllScopedProjects(ctx, user, projectIDs) {
			return "", "", nil, false
		}
		return scope, "", projectIDs, true
	default:
		return scope, ownerRef, nil, true
	}
}

func (h *Handlers) normalizeCredentialScopeWithinParent(ctx *gin.Context, user model.User, rawScope string, rawProjectIDs []string, parentScope string, parentProjectIDs []string, globalError string) (string, string, []string, bool) {
	scope, ownerRef, projectIDs, ok := h.normalizeScopedOwnerWithProjects(ctx, user, rawScope, "", rawProjectIDs, globalError)
	if !ok {
		return "", "", nil, false
	}
	parentScope = normalizeOwnerScope(parentScope)
	if parentScope == "user" && scope != "user" {
		writeError(ctx, http.StatusBadRequest, "个人级资源下的凭据只能设为个人使用")
		return "", "", nil, false
	}
	if parentScope == "project" {
		if scope == "global" {
			writeError(ctx, http.StatusBadRequest, "项目级资源下的凭据不能设为全局使用")
			return "", "", nil, false
		}
		if scope == "project" {
			allowed := make(map[string]struct{}, len(parentProjectIDs))
			for _, projectID := range normalizeStringList(parentProjectIDs) {
				allowed[projectID] = struct{}{}
			}
			for _, projectID := range projectIDs {
				if _, exists := allowed[projectID]; !exists {
					writeError(ctx, http.StatusBadRequest, "凭据不能共享给父级资源范围之外的项目空间")
					return "", "", nil, false
				}
			}
		}
	}
	return scope, ownerRef, projectIDs, true
}

func (h *Handlers) canManageScopedResourceByID(ctx *gin.Context, user model.User, scope, ownerRef, resourceType, resourceID, errorMessage string) bool {
	switch normalizeOwnerScope(scope) {
	case "global":
		if authz.IsPlatformAdmin(user.Role) {
			return true
		}
	case "user":
		if ownerRef == user.ID {
			return true
		}
	case "project":
		if authz.IsPlatformAdmin(user.Role) {
			return true
		}
		if h.canManageAllScopedProjects(ctx, user, h.scopedResourceProjectIDs(resourceType, resourceID)) {
			return true
		}
	}
	writeError(ctx, http.StatusForbidden, errorMessage)
	return false
}

func (h *Handlers) canInspectScopedResourceConfigByID(user model.User, scope, ownerRef, resourceType, resourceID string) bool {
	switch normalizeOwnerScope(scope) {
	case "global":
		return authz.IsPlatformAdmin(user.Role)
	case "user":
		return ownerRef == user.ID
	case "project":
		if authz.IsPlatformAdmin(user.Role) {
			return true
		}
		for _, projectID := range h.scopedResourceProjectIDs(resourceType, resourceID) {
			var member model.ProjectMember
			err := h.db.First(&member, "project_id = ? and user_id = ? and role in ?", projectID, user.ID, []string{"owner", "admin"}).Error
			if err == nil {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func (h *Handlers) canUseScopedResourceByID(user model.User, scope, ownerRef, resourceType, resourceID string) bool {
	switch normalizeOwnerScope(scope) {
	case "global":
		return true
	case "user":
		return ownerRef == user.ID
	case "project":
		if authz.IsPlatformAdmin(user.Role) {
			return true
		}
		for _, projectID := range h.scopedResourceProjectIDs(resourceType, resourceID) {
			if h.projects.UserHasProject(user.ID, projectID) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func normalizeOwnerScope(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "project", "user":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "global"
	}
}

func (h *Handlers) applyScopedResourceVisibility(ctx *gin.Context, query *gorm.DB, resourceType string, user model.User, projectID string) (*gorm.DB, bool) {
	projectID = strings.TrimSpace(projectID)
	if projectID != "" {
		if _, ok := h.findProjectForCurrentUserByID(ctx, projectID); !ok {
			return query, false
		}
	}

	conditions := []string{"scope = 'global'", "(scope = 'user' and owner_ref = ?)"}
	args := []any{user.ID}
	projectSubquery := h.db.Model(&model.ScopedResourceProjectBinding{}).
		Select("resource_id").
		Where("resource_type = ?", resourceType)
	if projectID != "" {
		projectSubquery = projectSubquery.Where("project_id = ?", projectID)
		conditions = append(conditions, "(scope = 'project' and id in (?))")
		args = append(args, projectSubquery)
	} else if authz.IsPlatformAdmin(user.Role) {
		conditions = append(conditions, "(scope = 'project' and id in (?))")
		args = append(args, projectSubquery)
	} else {
		projectIDs := h.projectIDsForUser(user.ID)
		if len(projectIDs) > 0 {
			projectSubquery = projectSubquery.Where("project_id in ?", projectIDs)
			conditions = append(conditions, "(scope = 'project' and id in (?))")
			args = append(args, projectSubquery)
		}
	}
	return query.Where(strings.Join(conditions, " or "), args...), true
}

func (h *Handlers) applyScopedResourceVisibilityForUser(query *gorm.DB, resourceType string, user model.User) *gorm.DB {
	conditions := []string{"scope = 'global'", "(scope = 'user' and owner_ref = ?)"}
	args := []any{user.ID}
	projectSubquery := h.db.Model(&model.ScopedResourceProjectBinding{}).
		Select("resource_id").
		Where("resource_type = ?", resourceType)
	if authz.IsPlatformAdmin(user.Role) {
		conditions = append(conditions, "(scope = 'project' and id in (?))")
		args = append(args, projectSubquery)
	} else if projectIDs := h.projectIDsForUser(user.ID); len(projectIDs) > 0 {
		conditions = append(conditions, "(scope = 'project' and id in (?))")
		args = append(args, projectSubquery.Where("project_id in ?", projectIDs))
	}
	return query.Where(strings.Join(conditions, " or "), args...)
}

func (h *Handlers) applyScopedResourceVisibilityForProject(query *gorm.DB, resourceType string, user model.User, projectID string) *gorm.DB {
	projectSubquery := h.db.Model(&model.ScopedResourceProjectBinding{}).
		Select("resource_id").
		Where("resource_type = ? and project_id = ?", resourceType, strings.TrimSpace(projectID))
	return query.Where(
		"scope = 'global' or (scope = 'user' and owner_ref = ?) or (scope = 'project' and id in (?))",
		user.ID,
		projectSubquery,
	)
}

func (h *Handlers) replaceScopedResourceProjectBindings(tx *gorm.DB, resourceType, resourceID string, projectIDs []string, defaultProjectIDs []string) error {
	if err := tx.Where("resource_type = ? and resource_id = ?", resourceType, resourceID).Delete(&model.ScopedResourceProjectBinding{}).Error; err != nil {
		return err
	}
	defaults := map[string]bool{}
	for _, projectID := range normalizeStringList(defaultProjectIDs) {
		defaults[projectID] = true
	}
	for _, projectID := range normalizeStringList(projectIDs) {
		binding := model.ScopedResourceProjectBinding{
			ID:           id.New("srpb"),
			ResourceType: resourceType,
			ResourceID:   resourceID,
			ProjectID:    projectID,
			IsDefault:    defaults[projectID],
		}
		if err := tx.Create(&binding).Error; err != nil {
			return err
		}
	}
	return nil
}

func (h *Handlers) scopedResourceProjectIDs(resourceType, resourceID string) []string {
	var bindings []model.ScopedResourceProjectBinding
	if err := h.db.Where("resource_type = ? and resource_id = ?", resourceType, resourceID).Order("project_id asc").Find(&bindings).Error; err != nil {
		return nil
	}
	result := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		result = append(result, binding.ProjectID)
	}
	return result
}

func (h *Handlers) scopedResourceProjectIDMap(resourceType string, resourceIDs []string) map[string][]string {
	result := map[string][]string{}
	resourceIDs = normalizeStringList(resourceIDs)
	if len(resourceIDs) == 0 {
		return result
	}
	var bindings []model.ScopedResourceProjectBinding
	if err := h.db.Where("resource_type = ? and resource_id in ?", resourceType, resourceIDs).Order("project_id asc").Find(&bindings).Error; err != nil {
		return result
	}
	for _, binding := range bindings {
		result[binding.ResourceID] = append(result[binding.ResourceID], binding.ProjectID)
	}
	return result
}

func (h *Handlers) scopedResourceDefaultProjectIDMap(resourceType string, resourceIDs []string) map[string][]string {
	result := map[string][]string{}
	resourceIDs = normalizeStringList(resourceIDs)
	if len(resourceIDs) == 0 {
		return result
	}
	var bindings []model.ScopedResourceProjectBinding
	if err := h.db.Where("resource_type = ? and resource_id in ? and is_default = ?", resourceType, resourceIDs, true).Order("project_id asc").Find(&bindings).Error; err != nil {
		return result
	}
	for _, binding := range bindings {
		result[binding.ResourceID] = append(result[binding.ResourceID], binding.ProjectID)
	}
	return result
}

func (h *Handlers) canManageAllScopedProjects(ctx *gin.Context, user model.User, projectIDs []string) bool {
	projectIDs = normalizeStringList(projectIDs)
	if len(projectIDs) == 0 {
		return false
	}
	for _, projectID := range projectIDs {
		if authz.IsPlatformAdmin(user.Role) {
			var project model.Project
			if err := h.db.First(&project, "id = ?", projectID).Error; err != nil {
				writeError(ctx, http.StatusNotFound, "project not found")
				return false
			}
			continue
		}
		if _, ok := h.findProjectForCurrentUserWithRolesByID(ctx, projectID, "owner", "admin"); !ok {
			return false
		}
	}
	return true
}

func sortedProjectIDs(projectIDs []string) []string {
	result := normalizeStringList(projectIDs)
	sort.Strings(result)
	return result
}
