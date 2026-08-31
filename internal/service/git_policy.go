package service

import (
	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/model"
)

func CanUseGitAccount(user model.User, account model.GitAccount, projectActionAllows func(projectID string, action authz.Action) bool) bool {
	switch account.Scope {
	case "global":
		return true
	case "user":
		return account.OwnerRef == user.ID
	case "project":
		return user.Role == authz.PlatformRoleAdmin || projectActionAllows(account.OwnerRef, authz.ActionGitRead)
	default:
		return false
	}
}

func CanUseGitProvider(user model.User, provider model.GitProvider, projectActionAllows func(projectID string, action authz.Action) bool) bool {
	switch provider.Scope {
	case "global":
		return true
	case "user":
		return provider.OwnerRef == user.ID
	case "project":
		return user.Role == authz.PlatformRoleAdmin || projectActionAllows(provider.OwnerRef, authz.ActionGitRead)
	default:
		return false
	}
}
