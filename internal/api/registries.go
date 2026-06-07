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
	if projectID != "" {
		if _, ok := h.findProjectForCurrentUserByID(ctx, projectID); !ok {
			return
		}
	}

	var registries []model.ArtifactRegistry
	query := h.db.Order("is_default desc, created_at desc")
	conditions := []string{"scope = 'global'", "(scope = 'user' and owner_ref = ?)"}
	args := []any{user.ID}
	if projectID != "" {
		conditions = append(conditions, "(scope = 'project' and owner_ref = ?)")
		args = append(args, projectID)
	} else if user.Role == "platform_admin" {
		conditions = append(conditions, "scope = 'project'")
	} else {
		projectIDs := h.projectIDsForUser(user.ID)
		if len(projectIDs) > 0 {
			conditions = append(conditions, "(scope = 'project' and owner_ref in ?)")
			args = append(args, projectIDs)
		}
	}
	query = query.Where(strings.Join(conditions, " or "), args...)

	query = applySearch(ctx, query, "name", "endpoint")
	if err := query.Find(&registries).Error; err != nil {
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
	if err := h.db.Delete(&registry).Error; err != nil {
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
	scope := normalizeRegistryScope(input.Scope)
	ownerRef := strings.TrimSpace(input.OwnerRef)
	switch scope {
	case "global":
		if user.Role != "platform_admin" {
			writeError(ctx, http.StatusForbidden, "只有平台管理员可以维护全局镜像站")
			return model.ArtifactRegistry{}, false
		}
		ownerRef = ""
	case "project":
		if ownerRef == "" {
			writeError(ctx, http.StatusBadRequest, "项目镜像站需要选择项目")
			return model.ArtifactRegistry{}, false
		}
		if _, ok := h.findProjectForCurrentUserWithRolesByID(ctx, ownerRef, "owner", "admin"); !ok {
			return model.ArtifactRegistry{}, false
		}
	case "user":
		ownerRef = user.ID
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
		if registry.IsDefault {
			if err := tx.Model(&model.ArtifactRegistry{}).
				Where("scope = ? and owner_ref = ? and id <> ?", registry.Scope, registry.OwnerRef, registry.ID).
				Update("is_default", false).Error; err != nil {
				return err
			}
		}
		return tx.Save(&registry).Error
	})
}
