package service

import "github.com/LiteyukiStudio/devops/internal/model"

func CanUseRegistry(user model.User, registry model.ArtifactRegistry, userHasProject func(userID, projectID string) bool) bool {
	switch registry.Scope {
	case "global":
		return true
	case "user":
		return registry.OwnerRef == user.ID
	case "project":
		return user.Role == "platform_admin" || userHasProject(user.ID, registry.OwnerRef)
	default:
		return false
	}
}

func CanManageRegistry(user model.User, registry model.ArtifactRegistry, projectRoleAllows func(projectID string, roles ...string) bool) bool {
	switch registry.Scope {
	case "global":
		return user.Role == "platform_admin"
	case "user":
		return registry.OwnerRef == user.ID
	case "project":
		return user.Role == "platform_admin" || projectRoleAllows(registry.OwnerRef, "owner", "admin")
	default:
		return false
	}
}

func CanManageRegistryCredential(user model.User, registry model.ArtifactRegistry, credential model.RegistryCredential, projectRoleAllows func(projectID string, roles ...string) bool) bool {
	switch credential.Scope {
	case "global":
		return user.Role == "platform_admin"
	case "user":
		return credential.OwnerRef == user.ID
	case "project":
		if user.Role == "platform_admin" {
			return true
		}
		for _, projectID := range credential.ProjectIDs {
			if !projectRoleAllows(projectID, "owner", "admin") {
				return false
			}
		}
		return len(credential.ProjectIDs) > 0
	default:
		return false
	}
}
