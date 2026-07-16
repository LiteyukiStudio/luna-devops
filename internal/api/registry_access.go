package api

import (
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
)

func (h *Handlers) registryForCurrentUser(ctx *gin.Context) (model.User, model.ArtifactRegistry, bool) {
	user, ok := h.currentUser(ctx)
	if !ok {
		return model.User{}, model.ArtifactRegistry{}, false
	}
	registry, ok := h.findAccessibleRegistry(ctx, user, ctx.Param("registryId"))
	return user, registry, ok
}

func (h *Handlers) findAccessibleRegistry(ctx *gin.Context, user model.User, registryID string) (model.ArtifactRegistry, bool) {
	var registry model.ArtifactRegistry
	if err := h.db.First(&registry, "id = ?", strings.TrimSpace(registryID)).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "artifact registry not found")
		return registry, false
	}
	if !h.canUseRegistry(ctx, user, registry) {
		return registry, false
	}
	registry.ProjectIDs = h.scopedResourceProjectIDs(scopedResourceArtifactRegistry, registry.ID)
	return registry, true
}

func (h *Handlers) canUseRegistry(ctx *gin.Context, user model.User, registry model.ArtifactRegistry) bool {
	if h.canUseScopedResourceByID(user, registry.Scope, registry.OwnerRef, scopedResourceArtifactRegistry, registry.ID) {
		return true
	}
	writeError(ctx, http.StatusForbidden, "无权访问该镜像站")
	return false
}

func (h *Handlers) canManageRegistry(ctx *gin.Context, user model.User, registry model.ArtifactRegistry) bool {
	if h.canManageScopedResourceByID(ctx, user, registry.Scope, registry.OwnerRef, scopedResourceArtifactRegistry, registry.ID, "无权维护该镜像站") {
		return true
	}
	return false
}

func (h *Handlers) canManageRegistryCredential(ctx *gin.Context, user model.User, registry model.ArtifactRegistry, credential model.RegistryCredential) bool {
	return h.canManageScopedResourceByID(ctx, user, credential.Scope, credential.OwnerRef, scopedResourceRegistryCredential, credential.ID, "无权维护该镜像凭据")
}

func (h *Handlers) defaultRegistryFor(userID, projectID string) (model.ArtifactRegistry, bool) {
	var projectRegistry model.ArtifactRegistry
	projectDefault := h.db.
		Joins("join scoped_resource_project_bindings srpb on srpb.resource_id = artifact_registries.id and srpb.resource_type = ? and srpb.project_id = ? and srpb.is_default = ?", scopedResourceArtifactRegistry, projectID, true).
		Where("artifact_registries.scope = ? and artifact_registries.deleted_at is null", "project").
		First(&projectRegistry)
	if projectDefault.Error == nil {
		projectRegistry.ProjectIDs = h.scopedResourceProjectIDs(scopedResourceArtifactRegistry, projectRegistry.ID)
		projectRegistry.DefaultProjectIDs = h.scopedResourceDefaultProjectIDMap(scopedResourceArtifactRegistry, []string{projectRegistry.ID})[projectRegistry.ID]
		return projectRegistry, true
	}
	candidates := []struct {
		scope string
		owner string
	}{
		{scope: "user", owner: userID},
		{scope: "global", owner: ""},
	}
	for _, candidate := range candidates {
		var registry model.ArtifactRegistry
		err := h.db.First(&registry, "scope = ? and owner_ref = ? and is_default = ?", candidate.scope, candidate.owner, true).Error
		if err == nil {
			return registry, true
		}
	}
	return model.ArtifactRegistry{}, false
}
