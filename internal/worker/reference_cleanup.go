package worker

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/notification"
	"github.com/LiteyukiStudio/devops/internal/runtimecluster"
	"github.com/LiteyukiStudio/devops/internal/secret"
	"gorm.io/gorm"
)

func cleanupProjectReferences(ctx context.Context, tx *gorm.DB, secrets secret.Store, projectID string) error {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil
	}
	var appIDs []string
	if err := tx.Model(&model.Application{}).Where("project_id = ?", projectID).Pluck("id", &appIDs).Error; err != nil {
		return err
	}
	var targetIDs []string
	if err := tx.Model(&model.DeploymentTarget{}).Where("project_id = ?", projectID).Pluck("id", &targetIDs).Error; err != nil {
		return err
	}
	deletedApps := stringSliceToSet(appIDs)
	deletedTargets := stringSliceToSet(targetIDs)

	if err := tx.Where("project_id = ?", projectID).Delete(&model.NotificationRule{}).Error; err != nil {
		return err
	}
	var globalRules []model.NotificationRule
	if err := tx.Where("project_id = ?", "").Find(&globalRules).Error; err != nil {
		return err
	}
	for _, rule := range globalRules {
		filter, err := notification.DecodeRuleFilter([]byte(rule.FilterJSON))
		if err != nil {
			continue
		}
		hadProjectFilter := filter.Scope == notification.RuleScopeProjects
		nextProjectIDs := filter.ProjectIDs
		if hadProjectFilter {
			nextProjectIDs = removeStringSet(filter.ProjectIDs, map[string]bool{projectID: true})
			if len(nextProjectIDs) != len(filter.ProjectIDs) && len(nextProjectIDs) == 0 {
				if err := tx.Delete(&rule).Error; err != nil {
					return err
				}
				continue
			}
		}
		hadAppFilter := len(filter.ApplicationIDs) > 0
		hadTargetFilter := len(filter.DeploymentTargetIDs) > 0
		nextAppIDs := filter.ApplicationIDs
		if hadAppFilter && len(deletedApps) > 0 {
			nextAppIDs = removeStringSet(filter.ApplicationIDs, deletedApps)
		}
		nextTargetIDs := filter.DeploymentTargetIDs
		if hadTargetFilter && len(deletedTargets) > 0 {
			nextTargetIDs = removeStringSet(filter.DeploymentTargetIDs, deletedTargets)
		}
		if len(nextProjectIDs) == len(filter.ProjectIDs) &&
			len(nextAppIDs) == len(filter.ApplicationIDs) &&
			len(nextTargetIDs) == len(filter.DeploymentTargetIDs) {
			continue
		}
		filter.ProjectIDs = nextProjectIDs
		filter.ApplicationIDs = nextAppIDs
		filter.DeploymentTargetIDs = nextTargetIDs
		encoded, err := notification.EncodeRuleFilter(filter)
		if err != nil {
			return err
		}
		updates := map[string]any{"filter_json": encoded}
		if (hadAppFilter && len(nextAppIDs) == 0) || (hadTargetFilter && len(nextTargetIDs) == 0) {
			updates["enabled"] = false
		}
		if err := tx.Model(&model.NotificationRule{}).Where("id = ?", rule.ID).Updates(updates).Error; err != nil {
			return err
		}
	}

	var bindings []model.ScopedResourceProjectBinding
	if err := tx.Where("project_id = ?", projectID).Find(&bindings).Error; err != nil {
		return err
	}
	if err := tx.Where("project_id = ?", projectID).Delete(&model.ScopedResourceProjectBinding{}).Error; err != nil {
		return err
	}
	idsByType := groupBoundResourceIDsByType(bindings)
	if err := cleanupOrphanProjectScopedResources(ctx, tx, secrets, projectID, idsByType); err != nil {
		return err
	}

	var repoBindings []model.RepositoryBinding
	if err := tx.Select("id", "webhook_secret").Where("project_id = ? and webhook_secret <> ''", projectID).Find(&repoBindings).Error; err != nil {
		return err
	}
	for _, repoBinding := range repoBindings {
		if err := secrets.DeleteRefContextWithDB(ctx, tx, repoBinding.WebhookSecret, "repository_binding:"+repoBinding.ID+":webhook"); err != nil {
			return err
		}
	}
	if err := tx.Where("project_id = ?", projectID).Delete(&model.DeploymentTargetHookBinding{}).Error; err != nil {
		return err
	}
	if err := tx.Where("project_id = ?", projectID).Delete(&model.GatewayRoute{}).Error; err != nil {
		return err
	}
	return tx.Where("project_id = ?", projectID).Delete(&model.RepositoryBinding{}).Error
}

func groupBoundResourceIDsByType(bindings []model.ScopedResourceProjectBinding) map[string][]string {
	seen := make(map[string]map[string]bool)
	out := make(map[string][]string)
	for _, binding := range bindings {
		resourceType := strings.TrimSpace(binding.ResourceType)
		resourceID := strings.TrimSpace(binding.ResourceID)
		if resourceType == "" || resourceID == "" {
			continue
		}
		if seen[resourceType] == nil {
			seen[resourceType] = make(map[string]bool)
		}
		if seen[resourceType][resourceID] {
			continue
		}
		seen[resourceType][resourceID] = true
		out[resourceType] = append(out[resourceType], resourceID)
	}
	return out
}

func orphanProjectScopedResourceIDs(tx *gorm.DB, resourceType string, candidateIDs []string) ([]string, error) {
	if len(candidateIDs) == 0 {
		return nil, nil
	}
	var stillBound []string
	if err := tx.Model(&model.ScopedResourceProjectBinding{}).
		Where("resource_type = ? and resource_id in ?", resourceType, candidateIDs).
		Distinct("resource_id").
		Pluck("resource_id", &stillBound).Error; err != nil {
		return nil, err
	}
	stillBoundSet := stringSliceToSet(stillBound)
	orphans := make([]string, 0, len(candidateIDs))
	for _, id := range candidateIDs {
		if !stillBoundSet[id] {
			orphans = append(orphans, id)
		}
	}
	return orphans, nil
}

func cleanupOrphanProjectScopedResources(ctx context.Context, tx *gorm.DB, secrets secret.Store, deletedProjectID string, idsByType map[string][]string) error {
	if orphanChannels, err := orphanProjectScopedResourceIDs(tx, notification.ScopedResourceChannelType, idsByType[notification.ScopedResourceChannelType]); err != nil {
		return err
	} else if len(orphanChannels) > 0 {
		var channels []model.NotificationChannel
		if err := tx.Where("id in ? and owner_user_id = ?", orphanChannels, "").Find(&channels).Error; err != nil {
			return err
		}
		for _, channel := range channels {
			if notification.NormalizeChannelScope(channel.Scope) != "project" {
				continue
			}
			secretRefs := map[string]string{}
			_ = json.Unmarshal([]byte(channel.SecretRefsJSON), &secretRefs)
			for key, ref := range secretRefs {
				if strings.TrimSpace(ref) == "" {
					continue
				}
				if err := secrets.DeleteRefContextWithDB(ctx, tx, ref, "notification_channel:"+channel.ID+":"+key); err != nil {
					return err
				}
			}
			if err := tx.Delete(&channel).Error; err != nil {
				return err
			}
		}
	}

	if orphanSets, err := orphanProjectScopedResourceIDs(tx, "build_variable_set", idsByType["build_variable_set"]); err != nil {
		return err
	} else if len(orphanSets) > 0 {
		var sets []model.BuildVariableSet
		if err := tx.Where("id in ? and scope = ?", orphanSets, "project").Find(&sets).Error; err != nil {
			return err
		}
		for _, set := range sets {
			secretRefs := map[string]string{}
			_ = json.Unmarshal([]byte(set.SecretRefs), &secretRefs)
			for key, ref := range secretRefs {
				if strings.TrimSpace(ref) == "" {
					continue
				}
				if err := secrets.DeleteRefContextWithDB(ctx, tx, ref, "build_variable_set:"+set.ID+":"+key); err != nil {
					return err
				}
			}
			if err := tx.Delete(&set).Error; err != nil {
				return err
			}
		}
	}

	if orphanCreds, err := orphanProjectScopedResourceIDs(tx, "registry_credential", idsByType["registry_credential"]); err != nil {
		return err
	} else if len(orphanCreds) > 0 {
		var creds []model.RegistryCredential
		if err := tx.Where("id in ? and scope = ?", orphanCreds, "project").Find(&creds).Error; err != nil {
			return err
		}
		for _, cred := range creds {
			if err := deleteRegistryCredentialSecrets(ctx, tx, secrets, cred); err != nil {
				return err
			}
			if err := tx.Model(&model.ArtifactRegistry{}).Where("id = ? and credential_ref = ?", cred.RegistryID, cred.ID).Update("credential_ref", "").Error; err != nil {
				return err
			}
			if err := tx.Delete(&cred).Error; err != nil {
				return err
			}
		}
	}

	if orphanRegistries, err := orphanProjectScopedResourceIDs(tx, "artifact_registry", idsByType["artifact_registry"]); err != nil {
		return err
	} else if len(orphanRegistries) > 0 {
		var registries []model.ArtifactRegistry
		if err := tx.Where("id in ? and scope = ?", orphanRegistries, "project").Find(&registries).Error; err != nil {
			return err
		}
		for _, reg := range registries {
			var otherTargetCount int64
			if err := tx.Model(&model.DeploymentTarget{}).
				Joins("join projects on projects.id = deployment_targets.project_id and projects.deleted_at is null").
				Where("deployment_targets.target_registry_id = ? and deployment_targets.project_id <> ?", reg.ID, deletedProjectID).
				Count(&otherTargetCount).Error; err != nil {
				return err
			}
			if otherTargetCount > 0 {
				continue
			}
			var childCreds []model.RegistryCredential
			if err := tx.Where("registry_id = ?", reg.ID).Find(&childCreds).Error; err != nil {
				return err
			}
			childCredIDs := make([]string, 0, len(childCreds))
			for _, cred := range childCreds {
				childCredIDs = append(childCredIDs, cred.ID)
				if err := deleteRegistryCredentialSecrets(ctx, tx, secrets, cred); err != nil {
					return err
				}
			}
			if len(childCredIDs) > 0 {
				if err := tx.Where("resource_type = ? and resource_id in ?", "registry_credential", childCredIDs).Delete(&model.ScopedResourceProjectBinding{}).Error; err != nil {
					return err
				}
				if err := tx.Where("id in ?", childCredIDs).Delete(&model.RegistryCredential{}).Error; err != nil {
					return err
				}
			}
			if err := tx.Delete(&reg).Error; err != nil {
				return err
			}
		}
	}

	if orphanAccounts, err := orphanProjectScopedResourceIDs(tx, "git_account", idsByType["git_account"]); err != nil {
		return err
	} else if len(orphanAccounts) > 0 {
		var accounts []model.GitAccount
		if err := tx.Where("id in ? and scope = ?", orphanAccounts, "project").Find(&accounts).Error; err != nil {
			return err
		}
		for _, acc := range accounts {
			var otherBindings int64
			if err := tx.Model(&model.RepositoryBinding{}).
				Joins("join projects on projects.id = repository_bindings.project_id and projects.deleted_at is null").
				Where("repository_bindings.git_account_id = ? and repository_bindings.project_id <> ?", acc.ID, deletedProjectID).
				Count(&otherBindings).Error; err != nil {
				return err
			}
			if otherBindings > 0 {
				continue
			}
			if err := deleteGitAccountSecrets(ctx, tx, secrets, acc); err != nil {
				return err
			}
			if err := tx.Delete(&acc).Error; err != nil {
				return err
			}
		}
	}

	if orphanProviders, err := orphanProjectScopedResourceIDs(tx, "git_provider", idsByType["git_provider"]); err != nil {
		return err
	} else if len(orphanProviders) > 0 {
		var providers []model.GitProvider
		if err := tx.Where("id in ? and scope = ?", orphanProviders, "project").Find(&providers).Error; err != nil {
			return err
		}
		for _, provider := range providers {
			var otherBindings int64
			if err := tx.Model(&model.RepositoryBinding{}).
				Joins("join projects on projects.id = repository_bindings.project_id and projects.deleted_at is null").
				Where("repository_bindings.git_provider_id = ? and repository_bindings.project_id <> ?", provider.ID, deletedProjectID).
				Count(&otherBindings).Error; err != nil {
				return err
			}
			if otherBindings > 0 {
				continue
			}
			var childAccounts []model.GitAccount
			if err := tx.Where("provider_id = ?", provider.ID).Find(&childAccounts).Error; err != nil {
				return err
			}
			childAccountIDs := make([]string, 0, len(childAccounts))
			for _, acc := range childAccounts {
				childAccountIDs = append(childAccountIDs, acc.ID)
				if err := deleteGitAccountSecrets(ctx, tx, secrets, acc); err != nil {
					return err
				}
			}
			if len(childAccountIDs) > 0 {
				if err := tx.Where("resource_type = ? and resource_id in ?", "git_account", childAccountIDs).Delete(&model.ScopedResourceProjectBinding{}).Error; err != nil {
					return err
				}
				if err := tx.Where("id in ?", childAccountIDs).Delete(&model.GitAccount{}).Error; err != nil {
					return err
				}
			}
			if err := secrets.DeleteRefContextWithDB(ctx, tx, provider.ClientSecretRef, "git_provider:"+provider.ID); err != nil {
				return err
			}
			if err := tx.Delete(&provider).Error; err != nil {
				return err
			}
		}
	}

	if orphanClusters, err := orphanProjectScopedResourceIDs(tx, "runtime_cluster", idsByType["runtime_cluster"]); err != nil {
		return err
	} else if len(orphanClusters) > 0 {
		var clusters []model.RuntimeCluster
		if err := tx.Where("id in ? and scope = ?", orphanClusters, "project").Find(&clusters).Error; err != nil {
			return err
		}
		finishedAt := time.Now().UTC()
		for _, cluster := range clusters {
			var otherTargets int64
			if err := tx.Model(&model.DeploymentTarget{}).
				Joins("join projects on projects.id = deployment_targets.project_id and projects.deleted_at is null").
				Where("deployment_targets.cluster_id = ? and deployment_targets.project_id <> ?", cluster.ID, deletedProjectID).
				Count(&otherTargets).Error; err != nil {
				return err
			}
			var otherVolumes int64
			if err := tx.Model(&model.ProjectVolume{}).
				Joins("join projects on projects.id = project_volumes.project_id and projects.deleted_at is null").
				Where("project_volumes.cluster_id = ? and project_volumes.project_id <> ?", cluster.ID, deletedProjectID).
				Count(&otherVolumes).Error; err != nil {
				return err
			}
			if otherTargets > 0 || otherVolumes > 0 {
				continue
			}
			if err := secrets.DeleteRefContextWithDB(ctx, tx, cluster.KubeconfigRef, "runtime_cluster:"+cluster.ID+":kubeconfig"); err != nil {
				return err
			}
			var exceptionIDs []string
			if err := tx.Model(&model.NetworkEgressException{}).Where("runtime_cluster_id = ?", cluster.ID).Pluck("id", &exceptionIDs).Error; err != nil {
				return err
			}
			if len(exceptionIDs) > 0 {
				if err := tx.Where("exception_id in ?", exceptionIDs).Delete(&model.NetworkEgressExceptionGrant{}).Error; err != nil {
					return err
				}
				if err := tx.Where("id in ?", exceptionIDs).Delete(&model.NetworkEgressException{}).Error; err != nil {
					return err
				}
			}
			if err := tx.Model(&model.RuntimeCluster{}).Where("id = ?", cluster.ID).Updates(map[string]any{
				"delete_status":      "deleted",
				"delete_message":     "",
				"delete_finished_at": &finishedAt,
				"kubeconfig_ref":     "",
			}).Error; err != nil {
				return err
			}
			if err := tx.Delete(&cluster).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func deleteRegistryCredentialSecrets(ctx context.Context, tx *gorm.DB, secrets secret.Store, cred model.RegistryCredential) error {
	if err := secrets.DeleteRefContextWithDB(ctx, tx, cred.PasswordRef, "registry_credential:"+cred.RegistryID+":password"); err != nil {
		return err
	}
	return secrets.DeleteRefContextWithDB(ctx, tx, cred.TokenRef, "registry_credential:"+cred.RegistryID+":token")
}

func deleteGitAccountSecrets(ctx context.Context, tx *gorm.DB, secrets secret.Store, account model.GitAccount) error {
	if err := secrets.DeleteRefContextWithDB(ctx, tx, account.AccessTokenRef, "git_account:"+account.ID+":access"); err != nil {
		return err
	}
	return secrets.DeleteRefContextWithDB(ctx, tx, account.RefreshTokenRef, "git_account:"+account.ID+":refresh")
}

func stringSliceToSet(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out[trimmed] = true
		}
	}
	return out
}

func cleanupTargetReferences(tx *gorm.DB, projectID string, deletedAppID string, deletedTargetIDs []string) (bool, error) {
	projectID = strings.TrimSpace(projectID)
	deletedAppID = strings.TrimSpace(deletedAppID)
	deletedTargets := make(map[string]bool, len(deletedTargetIDs))
	for _, targetID := range deletedTargetIDs {
		if trimmed := strings.TrimSpace(targetID); trimmed != "" {
			deletedTargets[trimmed] = true
		}
	}

	var rules []model.NotificationRule
	if err := tx.Where("project_id in ?", []string{"", projectID}).Find(&rules).Error; err != nil {
		return false, err
	}
	for _, rule := range rules {
		filter, err := notification.DecodeRuleFilter([]byte(rule.FilterJSON))
		if err != nil {
			continue
		}
		hadAppFilter := len(filter.ApplicationIDs) > 0
		hadTargetFilter := len(filter.DeploymentTargetIDs) > 0
		nextAppIDs := filter.ApplicationIDs
		if deletedAppID != "" && hadAppFilter {
			nextAppIDs = removeStringSet(filter.ApplicationIDs, map[string]bool{deletedAppID: true})
		}
		nextTargetIDs := filter.DeploymentTargetIDs
		if len(deletedTargets) > 0 && hadTargetFilter {
			nextTargetIDs = removeStringSet(filter.DeploymentTargetIDs, deletedTargets)
		}
		if len(nextAppIDs) == len(filter.ApplicationIDs) && len(nextTargetIDs) == len(filter.DeploymentTargetIDs) {
			continue
		}
		filter.ApplicationIDs = nextAppIDs
		filter.DeploymentTargetIDs = nextTargetIDs
		encoded, err := notification.EncodeRuleFilter(filter)
		if err != nil {
			return false, err
		}
		updates := map[string]any{"filter_json": encoded}
		// Narrowed rules whose sole target app/deployment target was deleted must fail closed
		// rather than silently widening to match every workload in the project.
		if (hadAppFilter && len(nextAppIDs) == 0) || (hadTargetFilter && len(nextTargetIDs) == 0) {
			updates["enabled"] = false
		}
		if err := tx.Model(&model.NotificationRule{}).Where("id = ?", rule.ID).Updates(updates).Error; err != nil {
			return false, err
		}
	}

	if len(deletedTargets) == 0 {
		return false, nil
	}
	var grants []model.NetworkEgressExceptionGrant
	if err := tx.Where("project_id = ? and deployment_target_ids <> ''", projectID).Find(&grants).Error; err != nil {
		return false, err
	}
	egressChanged := false
	for _, grant := range grants {
		current := runtimecluster.SplitIDList(grant.DeploymentTargetIDs)
		remaining := removeStringSet(current, deletedTargets)
		if len(remaining) == len(current) {
			continue
		}
		egressChanged = true
		if len(remaining) == 0 {
			// An empty deployment_target_ids grants egress to the entire project; delete the
			// narrowed grant when its last target is deleted instead of widening permissions.
			if err := tx.Delete(&grant).Error; err != nil {
				return false, err
			}
			continue
		}
		if err := tx.Model(&model.NetworkEgressExceptionGrant{}).
			Where("id = ?", grant.ID).
			Update("deployment_target_ids", strings.Join(remaining, ",")).Error; err != nil {
			return false, err
		}
	}
	return egressChanged, nil
}

func removeStringSet(values []string, remove map[string]bool) []string {
	if len(values) == 0 || len(remove) == 0 {
		return values
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" || remove[trimmed] {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func appendUniqueProjectID(projectIDs []string, projectID string) []string {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" || slices.Contains(projectIDs, projectID) {
		return projectIDs
	}
	return append(projectIDs, projectID)
}
