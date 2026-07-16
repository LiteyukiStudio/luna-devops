package api

import (
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/id"
	"github.com/LiteyukiStudio/devops/internal/model"
	registryprovider "github.com/LiteyukiStudio/devops/internal/provider/registry"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *Handlers) ListArtifactRegistries(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}

	projectID := strings.TrimSpace(ctx.Query("projectId"))

	var registries []model.ArtifactRegistry
	query := h.db.Model(&model.ArtifactRegistry{})
	var visible bool
	query, visible = h.applyScopedResourceVisibility(ctx, query, scopedResourceArtifactRegistry, user, projectID)
	if !visible {
		return
	}

	query = applySearch(ctx, query, "name", "endpoint")
	if paginationRequested(ctx) {
		pagination := paginationFromQuery(ctx)
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
		ctx.JSON(http.StatusOK, paginatedResponse(h.registryResponsesForUser(user, registries), total, pagination))
		return
	}
	if err := query.Order("is_default desc, created_at desc").Find(&registries).Error; err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, h.registryResponsesForUser(user, registries))
}

func (h *Handlers) CreateArtifactRegistry(ctx *gin.Context) {
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

	if err := h.saveRegistryWithDefault(registry); err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	h.audit(user.ID, "registry.create", registry.ID, true, registry.Scope)
	ctx.JSON(http.StatusCreated, registryResponse(registry))
}

func (h *Handlers) UpdateArtifactRegistry(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}

	var existing model.ArtifactRegistry
	if err := h.db.First(&existing, "id = ?", ctx.Param("registryId")).Error; err != nil {
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
	existing.Name = next.Name
	existing.Provider = next.Provider
	existing.Endpoint = next.Endpoint
	existing.Namespace = next.Namespace
	existing.Scope = next.Scope
	existing.OwnerRef = next.OwnerRef
	existing.ProjectIDs = next.ProjectIDs
	existing.IsDefault = next.IsDefault
	existing.Capabilities = next.Capabilities

	if err := h.saveRegistryWithDefault(existing); err != nil {
		writeError(ctx, http.StatusBadRequest, err.Error())
		return
	}
	h.audit(user.ID, "registry.update", existing.ID, true, existing.Scope)
	ctx.JSON(http.StatusOK, registryResponse(existing))
}

func (h *Handlers) DeleteArtifactRegistry(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}

	var registry model.ArtifactRegistry
	if err := h.db.First(&registry, "id = ?", ctx.Param("registryId")).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "artifact registry not found")
		return
	}
	if !h.canManageRegistry(ctx, user, registry) {
		return
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		var credentialIDs []string
		if err := tx.Model(&model.RegistryCredential{}).Where("registry_id = ?", registry.ID).Pluck("id", &credentialIDs).Error; err != nil {
			return err
		}
		if len(credentialIDs) > 0 {
			if err := tx.Where("resource_type = ? and resource_id in ?", scopedResourceRegistryCredential, credentialIDs).Delete(&model.ScopedResourceProjectBinding{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("registry_id = ?", registry.ID).Delete(&model.RegistryCredential{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&registry).Error; err != nil {
			return err
		}
		return tx.Where("resource_type = ? and resource_id = ?", scopedResourceArtifactRegistry, registry.ID).Delete(&model.ScopedResourceProjectBinding{}).Error
	}); err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(user.ID, "registry.delete", registry.ID, true, registry.Scope)
	ctx.Status(http.StatusNoContent)
}

func (h *Handlers) GetDefaultArtifactRegistry(ctx *gin.Context) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return
	}
	projectID := strings.TrimSpace(ctx.Param("projectId"))
	if _, ok := h.findProjectForCurrentUserByID(ctx, projectID); !ok {
		return
	}

	registry, ok := h.defaultRegistryFor(user.ID, projectID)
	if !ok {
		writeError(ctx, http.StatusNotFound, "default artifact registry not found")
		return
	}
	ctx.JSON(http.StatusOK, registryResponse(registry))
}

func (h *Handlers) TestArtifactRegistry(ctx *gin.Context) {
	user, registry, ok := h.registryForCurrentUser(ctx)
	if !ok {
		return
	}

	result := h.pingRegistry(ctx.Request.Context(), user, registry)
	ctx.JSON(http.StatusOK, result)
}

func (h *Handlers) registryFromInput(ctx *gin.Context, user model.User, input artifactRegistryInput, registryID string) (model.ArtifactRegistry, bool) {
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

func (h *Handlers) saveRegistryWithDefault(registry model.ArtifactRegistry) error {
	return h.db.Transaction(func(tx *gorm.DB) error {
		projectIDs := sortedProjectIDs(registry.ProjectIDs)
		defaultProjectIDs := []string{}
		if registry.Scope == "project" {
			if registry.IsDefault {
				defaultProjectIDs = projectIDs
				if err := tx.Model(&model.ScopedResourceProjectBinding{}).
					Where("resource_type = ? and project_id in ? and resource_id <> ?", scopedResourceArtifactRegistry, projectIDs, registry.ID).
					Update("is_default", false).Error; err != nil {
					return err
				}
			}
			registry.IsDefault = false
		} else if registry.IsDefault {
			if err := tx.Model(&model.ArtifactRegistry{}).
				Where("scope = ? and owner_ref = ? and id <> ?", registry.Scope, registry.OwnerRef, registry.ID).
				Update("is_default", false).Error; err != nil {
				return err
			}
		}
		if err := tx.Save(&registry).Error; err != nil {
			return err
		}
		return h.replaceScopedResourceProjectBindings(tx, scopedResourceArtifactRegistry, registry.ID, projectIDs, defaultProjectIDs)
	})
}
