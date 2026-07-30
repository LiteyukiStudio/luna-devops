package service

import (
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
)

func CanUseRegistry(user model.User, registry model.ArtifactRegistry, userHasProject func(userID, projectID string) bool) bool {
	switch registry.Scope {
	case "global":
		return true
	case "user":
		return registry.OwnerRef == user.ID
	case "project":
		return user.Role == authz.PlatformRoleAdmin || userHasProject(user.ID, registry.OwnerRef)
	default:
		return false
	}
}

func CanManageRegistry(user model.User, registry model.ArtifactRegistry, projectRoleAllows func(projectID string, roles ...string) bool) bool {
	switch registry.Scope {
	case "global":
		return user.Role == authz.PlatformRoleAdmin
	case "user":
		return registry.OwnerRef == user.ID
	case "project":
		return user.Role == authz.PlatformRoleAdmin || projectRoleAllows(registry.OwnerRef, authz.ProjectRoleOwner, authz.ProjectRoleAdmin)
	default:
		return false
	}
}

func CanManageRegistryCredential(user model.User, registry model.ArtifactRegistry, credential model.RegistryCredential, projectRoleAllows func(projectID string, roles ...string) bool) bool {
	switch credential.Scope {
	case "global":
		return user.Role == authz.PlatformRoleAdmin
	case "user":
		return credential.OwnerRef == user.ID
	case "project":
		if user.Role == authz.PlatformRoleAdmin {
			return true
		}
		for _, projectID := range credential.ProjectIDs {
			if !projectRoleAllows(projectID, authz.ProjectRoleOwner, authz.ProjectRoleAdmin) {
				return false
			}
		}
		return len(credential.ProjectIDs) > 0
	default:
		return false
	}
}
