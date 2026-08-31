package service

import (
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
)

func CanUseRegistry(user model.User, registry model.ArtifactRegistry, projectActionAllows func(projectID string, action authz.Action) bool) bool {
	switch registry.Scope {
	case "global":
		return true
	case "user":
		return registry.OwnerRef == user.ID
	case "project":
		return user.Role == authz.PlatformRoleAdmin || projectActionAllows(registry.OwnerRef, authz.ActionRegistryUse)
	default:
		return false
	}
}

func CanManageRegistry(user model.User, registry model.ArtifactRegistry, projectActionAllows func(projectID string, action authz.Action) bool) bool {
	switch registry.Scope {
	case "global":
		return user.Role == authz.PlatformRoleAdmin
	case "user":
		return registry.OwnerRef == user.ID
	case "project":
		return user.Role == authz.PlatformRoleAdmin || projectActionAllows(registry.OwnerRef, authz.ActionProjectManage)
	default:
		return false
	}
}

func CanManageRegistryCredential(user model.User, registry model.ArtifactRegistry, credential model.RegistryCredential, projectActionAllows func(projectID string, action authz.Action) bool) bool {
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
			if !projectActionAllows(projectID, authz.ActionProjectManage) {
				return false
			}
		}
		return len(credential.ProjectIDs) > 0
	default:
		return false
	}
}
