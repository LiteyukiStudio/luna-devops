package registryapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	registryprovider "github.com/LiteyukiStudio/devops/internal/provider/registry"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (h *Handler) ListArtifactRegistries(ctx *gin.Context) {
	markLiveObservationResponse(ctx)
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	visibility, ok := h.resolveListVisibility(ctx, user)
	if !ok {
		return
	}

	projectID := strings.TrimSpace(ctx.Query("projectId"))

	var registries []model.ArtifactRegistry
	query := h.dbFor(ctx).Model(&model.ArtifactRegistry{})
	var visible bool
	query, visible = h.applyScopedResourceListVisibility(ctx, query, scopedResourceArtifactRegistry, user, projectID, visibility)
	if !visible {
		return
	}

	query = applySearch(ctx, query, "name", "endpoint")
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
	}, "created_at")).Limit(pagination.PageSize).Offset(pagination.Offset()).Find(&registries).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.observeArtifactRegistries(ctx.Request.Context(), user, registries)
	responses, err := h.registryResponsesForUser(user, registries, ctx.Request.Context())
	if err != nil {
		h.writeProjectAuthorizationError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, paginatedResponse(responses, total, pagination))
}

func (h *Handler) CreateArtifactRegistry(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}

	var input artifactRegistryInput
	if !bindJSON(ctx, &input) {
		return
	}

	registry, ok := h.registryFromInput(ctx, user, input, "")
	if !ok {
		return
	}
	registry.ID = id.New("reg")
	registry.CreatedBy = user.ID

	if err := h.saveRegistryWithDefault(&registry, ctx.Request.Context()); err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	h.auditWithContext(user.ID, "registry.create", registry.ID, true, registry.Scope, ctx.Request.Context())
	h.observeArtifactRegistry(ctx.Request.Context(), user, &registry)
	ctx.JSON(http.StatusCreated, registryResponseWithManage(registry, true))
}

func (h *Handler) UpdateArtifactRegistry(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}

	var existing model.ArtifactRegistry
	if err := h.dbFor(ctx).First(&existing, "id = ?", ctx.Param("registryId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "artifact registry not found")
		return
	}
	if !h.canManageRegistry(ctx, user, existing) {
		return
	}

	var input artifactRegistryInput
	if !bindJSON(ctx, &input) {
		return
	}

	next, ok := h.registryFromInput(ctx, user, input, existing.ID)
	if !ok {
		return
	}
	var updated model.ArtifactRegistry
	err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		var locked model.ArtifactRegistry
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "id = ?", existing.ID).Error; err != nil {
			return err
		}
		locked.Name = next.Name
		locked.Provider = next.Provider
		locked.Endpoint = next.Endpoint
		locked.Namespace = next.Namespace
		locked.Scope = next.Scope
		locked.OwnerRef = next.OwnerRef
		locked.ProjectIDs = next.ProjectIDs
		locked.IsDefault = next.IsDefault
		locked.Capabilities = next.Capabilities
		if compatErr := ensureRegistryScopeCompatibleTx(tx, locked); compatErr != nil {
			return compatErr
		}
		if saveErr := h.saveRegistryWithDefaultTx(tx, &locked); saveErr != nil {
			return saveErr
		}
		updated = locked
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(ctx, http.StatusNotFound, "artifact registry not found")
		return
	}
	if errors.Is(err, errRegistryCredentialScopeConflict) {
		writeErrorCode(ctx, http.StatusConflict, "registry.credential_scope_conflict", "child registry credentials exceed the target registry scope")
		return
	}
	if errors.Is(err, errArtifactRegistryInUse) {
		writeErrorCode(ctx, http.StatusConflict, "registry.in_use", "artifact registry is referenced by deployment targets outside the target scope")
		return
	}
	if err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	h.auditWithContext(user.ID, "registry.update", updated.ID, true, updated.Scope, ctx.Request.Context())
	h.observeArtifactRegistry(ctx.Request.Context(), user, &updated)
	ctx.JSON(http.StatusOK, registryResponseWithManage(updated, true))
}

func (h *Handler) DeleteArtifactRegistry(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}

	var registry model.ArtifactRegistry
	if err := h.dbFor(ctx).First(&registry, "id = ?", ctx.Param("registryId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "artifact registry not found")
		return
	}
	if !h.canManageRegistry(ctx, user, registry) {
		return
	}
	err := h.dbFor(ctx).Transaction(func(tx *gorm.DB) error {
		var locked model.ArtifactRegistry
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "id = ?", registry.ID).Error; err != nil {
			return err
		}
		var targetCount int64
		if err := tx.Model(&model.DeploymentTarget{}).
			Joins("join projects on projects.id = deployment_targets.project_id and projects.deleted_at is null").
			Where("deployment_targets.target_registry_id = ?", locked.ID).
			Count(&targetCount).Error; err != nil {
			return err
		}
		if targetCount > 0 {
			return errArtifactRegistryInUse
		}
		var credentials []model.RegistryCredential
		if err := tx.Where("registry_id = ?", locked.ID).Find(&credentials).Error; err != nil {
			return err
		}
		credentialIDs := make([]string, 0, len(credentials))
		for _, cred := range credentials {
			credentialIDs = append(credentialIDs, cred.ID)
			if err := h.deleteSecretRefWithDB(ctx.Request.Context(), tx, cred.PasswordRef, "registry_credential:"+cred.RegistryID+":password"); err != nil {
				return err
			}
			if err := h.deleteSecretRefWithDB(ctx.Request.Context(), tx, cred.TokenRef, "registry_credential:"+cred.RegistryID+":token"); err != nil {
				return err
			}
		}
		if len(credentialIDs) > 0 {
			if err := tx.Where("resource_type = ? and resource_id in ?", scopedResourceRegistryCredential, credentialIDs).Delete(&model.ScopedResourceProjectBinding{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("registry_id = ?", locked.ID).Delete(&model.RegistryCredential{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&locked).Error; err != nil {
			return err
		}
		return tx.Where("resource_type = ? and resource_id = ?", scopedResourceArtifactRegistry, locked.ID).Delete(&model.ScopedResourceProjectBinding{}).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeError(ctx, http.StatusNotFound, "artifact registry not found")
		return
	}
	if errors.Is(err, errArtifactRegistryInUse) {
		writeErrorCode(ctx, http.StatusConflict, "registry.in_use", "artifact registry is referenced by deployment targets")
		return
	}
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.auditWithContext(user.ID, "registry.delete", registry.ID, true, registry.Scope, ctx.Request.Context())
	ctx.Status(http.StatusNoContent)
}

var (
	errArtifactRegistryInUse           = errors.New("artifact registry is referenced by deployment targets")
	errRegistryCredentialScopeConflict = errors.New("child registry credentials exceed the target registry scope")
)

func ensureRegistryScopeCompatibleTx(tx *gorm.DB, registry model.ArtifactRegistry) error {
	if registry.Scope == "global" {
		return nil
	}
	var credentials []model.RegistryCredential
	if err := tx.Where("registry_id = ?", registry.ID).Find(&credentials).Error; err != nil {
		return err
	}
	allowedProjects := make(map[string]bool, len(registry.ProjectIDs))
	for _, projectID := range registry.ProjectIDs {
		allowedProjects[strings.TrimSpace(projectID)] = true
	}
	credentialIDs := make([]string, 0, len(credentials))
	for _, cred := range credentials {
		credentialIDs = append(credentialIDs, cred.ID)
	}
	credProjectMap := make(map[string][]string, len(credentialIDs))
	if len(credentialIDs) > 0 {
		var bindings []model.ScopedResourceProjectBinding
		if err := tx.Where("resource_type = ? and resource_id in ?", scopedResourceRegistryCredential, credentialIDs).Find(&bindings).Error; err != nil {
			return err
		}
		for _, binding := range bindings {
			credProjectMap[binding.ResourceID] = append(credProjectMap[binding.ResourceID], binding.ProjectID)
		}
	}
	for _, cred := range credentials {
		switch registry.Scope {
		case "user":
			if cred.Scope != "user" || strings.TrimSpace(cred.OwnerRef) != strings.TrimSpace(registry.OwnerRef) {
				return errRegistryCredentialScopeConflict
			}
		case "project":
			if cred.Scope == "global" {
				return errRegistryCredentialScopeConflict
			}
			if cred.Scope == "project" {
				for _, boundProjectID := range credProjectMap[cred.ID] {
					if !allowedProjects[boundProjectID] {
						return errRegistryCredentialScopeConflict
					}
				}
			}
		}
	}

	targetQuery := tx.Model(&model.DeploymentTarget{}).
		Joins("join projects on projects.id = deployment_targets.project_id and projects.deleted_at is null").
		Where("deployment_targets.target_registry_id = ?", registry.ID)
	switch registry.Scope {
	case "project":
		if len(registry.ProjectIDs) > 0 {
			targetQuery = targetQuery.Where("deployment_targets.project_id not in ?", registry.ProjectIDs)
		}
	case "user":
		memberProjects := tx.Model(&model.ProjectMember{}).Select("project_id").Where("user_id = ?", registry.OwnerRef)
		targetQuery = targetQuery.Where("deployment_targets.project_id not in (?)", memberProjects)
	}
	var conflictTargets int64
	if err := targetQuery.Count(&conflictTargets).Error; err != nil {
		return err
	}
	if conflictTargets > 0 {
		return errArtifactRegistryInUse
	}
	return nil
}

func (h *Handler) GetDefaultArtifactRegistry(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	projectID := strings.TrimSpace(ctx.Param("projectId"))
	if _, ok := h.findProjectForCurrentUserByID(ctx, projectID); !ok {
		return
	}

	registry, ok := h.defaultRegistryFor(user.ID, projectID, ctx.Request.Context())
	if !ok {
		writeError(ctx, http.StatusNotFound, "default artifact registry not found")
		return
	}
	h.observeArtifactRegistry(ctx.Request.Context(), user, &registry)
	responses, err := h.registryResponsesForUser(user, []model.ArtifactRegistry{registry}, ctx.Request.Context())
	if err != nil {
		h.writeProjectAuthorizationError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, responses[0])
}

func (h *Handler) TestArtifactRegistry(ctx *gin.Context) {
	user, registry, ok := h.registryForCurrentUser(ctx)
	if !ok {
		return
	}

	result := h.pingRegistry(ctx.Request.Context(), user, registry)
	canInspect, err := h.canInspectScopedResourceConfigByID(user, registry.Scope, registry.OwnerRef, scopedResourceArtifactRegistry, registry.ID, ctx.Request.Context())
	if err != nil {
		h.writeProjectAuthorizationError(ctx, err)
		return
	}
	if !canInspect {
		result.Endpoint = ""
	}
	ctx.JSON(http.StatusOK, result)
}

func (h *Handler) registryFromInput(ctx *gin.Context, user model.User, input artifactRegistryInput, registryID string) (model.ArtifactRegistry, bool) {
	scope, ownerRef, projectIDs, ok := h.normalizeScopedOwnerWithProjects(ctx, user, input.Scope, input.OwnerRef, input.ProjectIDs, "只有平台管理员可以维护全局镜像站")
	if !ok {
		return model.ArtifactRegistry{}, false
	}

	endpoint := strings.TrimRight(strings.TrimSpace(input.Endpoint), "/")
	if _, err := registryprovider.ParseEndpoint(endpoint); err != nil {
		writeError(ctx, http.StatusBadRequest, "请输入有效镜像站地址")
		return model.ArtifactRegistry{}, false
	}

	registry := model.ArtifactRegistry{
		ID:           registryID,
		Name:         strings.TrimSpace(input.Name),
		Provider:     normalizeRegistryProvider(input.Provider),
		Endpoint:     endpoint,
		Namespace:    "",
		Scope:        scope,
		OwnerRef:     ownerRef,
		ProjectIDs:   projectIDs,
		IsDefault:    input.IsDefault,
		Capabilities: strings.Join(normalizeList(input.Capabilities, false), ","),
		CreatedBy:    user.ID,
	}
	if registry.Name == "" {
		writeError(ctx, http.StatusBadRequest, "请输入镜像站名称")
		return model.ArtifactRegistry{}, false
	}
	return registry, true
}

func (h *Handler) saveRegistryWithDefault(registry *model.ArtifactRegistry, ctx context.Context) error {
	return h.dbWithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return h.saveRegistryWithDefaultTx(tx, registry)
	})
}

func (h *Handler) saveRegistryWithDefaultTx(tx *gorm.DB, registry *model.ArtifactRegistry) error {
	projectIDs := sortedProjectIDs(registry.ProjectIDs)
	registry.ProjectIDs = projectIDs
	defaultProjectIDs := []string{}
	if registry.Scope == "project" {
		if registry.IsDefault {
			defaultProjectIDs = projectIDs
			if len(projectIDs) > 0 {
				if err := tx.Model(&model.ScopedResourceProjectBinding{}).
					Where("resource_type = ? and project_id in ? and resource_id <> ?", scopedResourceArtifactRegistry, projectIDs, registry.ID).
					Update("is_default", false).Error; err != nil {
					return err
				}
			}
		}
		registry.IsDefault = false
		registry.DefaultProjectIDs = defaultProjectIDs
	} else {
		registry.DefaultProjectIDs = []string{}
		if registry.IsDefault {
			if err := tx.Model(&model.ArtifactRegistry{}).
				Where("scope = ? and owner_ref = ? and id <> ?", registry.Scope, registry.OwnerRef, registry.ID).
				Update("is_default", false).Error; err != nil {
				return err
			}
		}
	}
	if err := tx.Save(registry).Error; err != nil {
		return err
	}
	return h.replaceScopedResourceProjectBindings(tx, scopedResourceArtifactRegistry, registry.ID, projectIDs, defaultProjectIDs)
}
