package api

import (
	"context"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
)

type deploymentBundleCandidateQuery struct {
	Pagination  paginationParams
	Search      string
	ID          string
	MatchSource bool
}

func normalizeDeploymentBundleCandidateQuery(query deploymentBundleCandidateQuery) deploymentBundleCandidateQuery {
	if query.Pagination.Page < 1 {
		query.Pagination.Page = 1
	}
	if query.Pagination.PageSize < 1 {
		query.Pagination.PageSize = defaultPageSize
	}
	if query.Pagination.PageSize > maxPageSize {
		query.Pagination.PageSize = maxPageSize
	}
	if query.Pagination.SortBy != "createdAt" {
		query.Pagination.SortBy = "name"
	}
	if query.Pagination.SortOrder != "desc" {
		query.Pagination.SortOrder = "asc"
	}
	query.Search = strings.TrimSpace(query.Search)
	query.ID = strings.TrimSpace(query.ID)
	return query
}

func deploymentBundleCandidatePage(candidates []deploymentBundleCandidate, total int64, pagination paginationParams) deploymentBundleReferenceCandidatePage {
	items := make([]deploymentBundleReferenceCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		items = append(items, candidate.Public)
	}
	totalPages := 0
	if total > 0 {
		totalPages = int((total + int64(pagination.PageSize) - 1) / int64(pagination.PageSize))
	}
	return deploymentBundleReferenceCandidatePage{
		Items: items, Page: pagination.Page, PageSize: pagination.PageSize,
		SortBy: pagination.SortBy, SortOrder: pagination.SortOrder,
		Total: total, TotalPages: totalPages,
	}
}

func deploymentBundleCandidateOrder(query deploymentBundleCandidateQuery, nameColumn, createdAtColumn, idColumn string) string {
	column := nameColumn
	if query.Pagination.SortBy == "createdAt" {
		column = createdAtColumn
	}
	return column + " " + query.Pagination.SortOrder + ", " + idColumn + " asc"
}

func applyDeploymentBundleCandidateSearch(query *gorm.DB, search string, columns ...string) *gorm.DB {
	if search == "" {
		return query
	}
	escaped := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(search)
	pattern := "%" + escaped + "%"
	conditions := make([]string, 0, len(columns))
	args := make([]any, 0, len(columns))
	for _, column := range columns {
		conditions = append(conditions, "LOWER("+column+") LIKE LOWER(?) ESCAPE '\\'")
		args = append(args, pattern)
	}
	return query.Where("("+strings.Join(conditions, " OR ")+")", args...)
}

func applyDeploymentBundleSourceMatch(query *gorm.DB, source deploymentBundleReferenceDescriptor, columns map[string]string) *gorm.DB {
	values := map[string]string{
		"name": source.Name, "type": source.Type, "scope": source.Scope,
		"owner": source.Owner, "repository": source.Repository, "namespace": source.Namespace,
		"accessMode": source.AccessMode, "volumeMode": source.VolumeMode,
		"storageClassName": source.StorageClassName, "clusterName": source.ClusterName, "clusterType": source.ClusterType,
	}
	for key, column := range columns {
		if value := strings.TrimSpace(values[key]); value != "" {
			query = query.Where("LOWER("+column+") = LOWER(?)", value)
		}
	}
	return query
}

func (h *Handlers) deploymentBundleCandidates(ctx context.Context, user model.User, project model.Project, app model.Application, reference deploymentBundleReference, rawQuery deploymentBundleCandidateQuery) (deploymentBundleReferenceCandidatePage, []deploymentBundleCandidate, error) {
	query := normalizeDeploymentBundleCandidateQuery(rawQuery)
	candidates := make([]deploymentBundleCandidate, 0, query.Pagination.PageSize)
	appendCandidate := func(id, name, description string, descriptor deploymentBundleReferenceDescriptor, compatible bool) {
		matched := deploymentBundleReferenceDescriptorMatches(reference.Source, descriptor)
		if reference.Kind == deploymentBundleReferenceProjectVolume && !matched {
			compatible = compatible && reference.Source.VolumeMode == descriptor.VolumeMode && reference.Source.AccessMode == descriptor.AccessMode
		}
		candidates = append(candidates, deploymentBundleCandidate{
			Public:     deploymentBundleReferenceCandidate{ID: id, Name: name, Description: description, Matched: matched, Compatible: compatible},
			Descriptor: descriptor,
		})
	}
	var total int64
	offset := query.Pagination.Offset()

	switch reference.Kind {
	case deploymentBundleReferenceRepositoryBinding:
		var items []struct {
			model.RepositoryBinding
			ProviderType string `gorm:"column:provider_type"`
		}
		base := h.dbWithContext(ctx).Table("repository_bindings").
			Joins("join git_providers on git_providers.id = repository_bindings.git_provider_id and git_providers.deleted_at is null").
			Where("repository_bindings.project_id = ? and repository_bindings.application_id = ? and repository_bindings.deleted_at is null", project.ID, app.ID)
		if query.ID != "" {
			base = base.Where("repository_bindings.id = ?", query.ID)
		}
		base = applyDeploymentBundleCandidateSearch(base, query.Search, "repository_bindings.owner", "repository_bindings.repo", "repository_bindings.owner || '/' || repository_bindings.repo", "git_providers.type")
		if query.MatchSource {
			base = applyDeploymentBundleSourceMatch(base, reference.Source, map[string]string{"name": "repository_bindings.owner || '/' || repository_bindings.repo", "owner": "repository_bindings.owner", "repository": "repository_bindings.repo", "type": "git_providers.type"})
		}
		if err := base.Count(&total).Error; err != nil {
			return deploymentBundleReferenceCandidatePage{}, nil, err
		}
		if err := base.Select("repository_bindings.*, git_providers.type as provider_type").
			Order(deploymentBundleCandidateOrder(query, "repository_bindings.owner || '/' || repository_bindings.repo", "repository_bindings.created_at", "repository_bindings.id")).Offset(offset).Limit(query.Pagination.PageSize).Scan(&items).Error; err != nil {
			return deploymentBundleReferenceCandidatePage{}, nil, err
		}
		for _, item := range items {
			name := strings.Trim(strings.TrimSpace(item.Owner)+"/"+strings.TrimSpace(item.Repo), "/")
			appendCandidate(item.ID, name, item.ProviderType, deploymentBundleReferenceDescriptor{Name: name, Type: item.ProviderType, Owner: item.Owner, Repository: item.Repo}, true)
		}

	case deploymentBundleReferenceRuntimeCluster:
		var items []model.RuntimeCluster
		base := h.applyScopedResourceVisibilityForProject(h.dbWithContext(ctx).Model(&model.RuntimeCluster{}), scopedResourceRuntimeCluster, user, project.ID, ctx).
			Where("type in ?", []string{"kubernetes", "k3s"})
		if query.ID != "" {
			base = base.Where("runtime_clusters.id = ?", query.ID)
		}
		base = applyDeploymentBundleCandidateSearch(base, query.Search, "runtime_clusters.name", "runtime_clusters.type")
		if query.MatchSource {
			base = applyDeploymentBundleSourceMatch(base, reference.Source, map[string]string{"name": "runtime_clusters.name", "type": "runtime_clusters.type"})
		}
		if err := base.Count(&total).Error; err != nil {
			return deploymentBundleReferenceCandidatePage{}, nil, err
		}
		if err := base.Order(deploymentBundleCandidateOrder(query, "runtime_clusters.name", "runtime_clusters.created_at", "runtime_clusters.id")).Offset(offset).Limit(query.Pagination.PageSize).Find(&items).Error; err != nil {
			return deploymentBundleReferenceCandidatePage{}, nil, err
		}
		for _, item := range items {
			appendCandidate(item.ID, item.Name, item.Type, deploymentBundleReferenceDescriptor{Name: item.Name, Type: item.Type}, true)
		}

	case deploymentBundleReferenceArtifactRegistry:
		var items []model.ArtifactRegistry
		base := h.applyScopedResourceVisibilityForProject(h.dbWithContext(ctx).Model(&model.ArtifactRegistry{}), scopedResourceArtifactRegistry, user, project.ID, ctx)
		if query.ID != "" {
			base = base.Where("artifact_registries.id = ?", query.ID)
		}
		base = applyDeploymentBundleCandidateSearch(base, query.Search, "artifact_registries.name", "artifact_registries.provider", "artifact_registries.namespace")
		if query.MatchSource {
			base = applyDeploymentBundleSourceMatch(base, reference.Source, map[string]string{"name": "artifact_registries.name", "type": "artifact_registries.provider", "namespace": "artifact_registries.namespace"})
		}
		if err := base.Count(&total).Error; err != nil {
			return deploymentBundleReferenceCandidatePage{}, nil, err
		}
		if err := base.Order(deploymentBundleCandidateOrder(query, "artifact_registries.name", "artifact_registries.created_at", "artifact_registries.id")).Offset(offset).Limit(query.Pagination.PageSize).Find(&items).Error; err != nil {
			return deploymentBundleReferenceCandidatePage{}, nil, err
		}
		for _, item := range items {
			_, hasPushCredential := h.registryPushCredentialForProject(user, item, project.ID, ctx)
			appendCandidate(item.ID, item.Name, item.Provider, deploymentBundleReferenceDescriptor{Name: item.Name, Type: item.Provider, Namespace: item.Namespace}, hasPushCredential)
		}

	case deploymentBundleReferenceBuildVariableSet:
		var items []model.BuildVariableSet
		base := h.applyScopedResourceVisibilityForProject(h.dbWithContext(ctx).Model(&model.BuildVariableSet{}), scopedResourceBuildVariableSet, user, project.ID, ctx).Where("enabled = ?", true)
		if query.ID != "" {
			base = base.Where("build_variable_sets.id = ?", query.ID)
		}
		base = applyDeploymentBundleCandidateSearch(base, query.Search, "build_variable_sets.name", "build_variable_sets.scope")
		if query.MatchSource {
			base = applyDeploymentBundleSourceMatch(base, reference.Source, map[string]string{"name": "build_variable_sets.name", "scope": "build_variable_sets.scope"})
		}
		if err := base.Count(&total).Error; err != nil {
			return deploymentBundleReferenceCandidatePage{}, nil, err
		}
		if err := base.Order(deploymentBundleCandidateOrder(query, "build_variable_sets.name", "build_variable_sets.created_at", "build_variable_sets.id")).Offset(offset).Limit(query.Pagination.PageSize).Find(&items).Error; err != nil {
			return deploymentBundleReferenceCandidatePage{}, nil, err
		}
		for _, item := range items {
			appendCandidate(item.ID, item.Name, item.Scope, deploymentBundleReferenceDescriptor{Name: item.Name, Scope: item.Scope}, true)
		}

	case deploymentBundleReferenceRuntimeConfigSet:
		var items []model.ProjectRuntimeConfigSet
		base := h.dbWithContext(ctx).Model(&model.ProjectRuntimeConfigSet{}).Where("project_id = ? and enabled = ? and delete_status = ?", project.ID, true, "active")
		if query.ID != "" {
			base = base.Where("project_runtime_config_sets.id = ?", query.ID)
		}
		base = applyDeploymentBundleCandidateSearch(base, query.Search, "project_runtime_config_sets.name")
		if query.MatchSource {
			base = applyDeploymentBundleSourceMatch(base, reference.Source, map[string]string{"name": "project_runtime_config_sets.name"})
		}
		if err := base.Count(&total).Error; err != nil {
			return deploymentBundleReferenceCandidatePage{}, nil, err
		}
		if err := base.Order(deploymentBundleCandidateOrder(query, "project_runtime_config_sets.name", "project_runtime_config_sets.created_at", "project_runtime_config_sets.id")).Offset(offset).Limit(query.Pagination.PageSize).Find(&items).Error; err != nil {
			return deploymentBundleReferenceCandidatePage{}, nil, err
		}
		for _, item := range items {
			appendCandidate(item.ID, item.Name, "", deploymentBundleReferenceDescriptor{Name: item.Name}, true)
		}

	case deploymentBundleReferenceHookConfig:
		var items []model.ProjectHookConfig
		base := h.dbWithContext(ctx).Model(&model.ProjectHookConfig{}).Where("project_id = ?", project.ID)
		if query.ID != "" {
			base = base.Where("project_hook_configs.id = ?", query.ID)
		}
		base = applyDeploymentBundleCandidateSearch(base, query.Search, "project_hook_configs.name", "project_hook_configs.shell")
		if query.MatchSource {
			base = applyDeploymentBundleSourceMatch(base, reference.Source, map[string]string{"name": "project_hook_configs.name", "type": "project_hook_configs.shell"})
		}
		if err := base.Count(&total).Error; err != nil {
			return deploymentBundleReferenceCandidatePage{}, nil, err
		}
		if err := base.Order(deploymentBundleCandidateOrder(query, "project_hook_configs.name", "project_hook_configs.created_at", "project_hook_configs.id")).Offset(offset).Limit(query.Pagination.PageSize).Find(&items).Error; err != nil {
			return deploymentBundleReferenceCandidatePage{}, nil, err
		}
		for _, item := range items {
			appendCandidate(item.ID, item.Name, item.Shell, deploymentBundleReferenceDescriptor{Name: item.Name, Type: item.Shell}, true)
		}

	case deploymentBundleReferenceProjectVolume:
		var items []struct {
			model.ProjectVolume
			ClusterName string `gorm:"column:cluster_name"`
			ClusterType string `gorm:"column:cluster_type"`
		}
		base := h.dbWithContext(ctx).Table("project_volumes").
			Joins("left join runtime_clusters on runtime_clusters.id = project_volumes.cluster_id and runtime_clusters.deleted_at is null").
			Where("project_volumes.project_id = ? and project_volumes.deleted_at is null", project.ID)
		if query.ID != "" {
			base = base.Where("project_volumes.id = ?", query.ID)
		}
		base = applyDeploymentBundleCandidateSearch(base, query.Search, "project_volumes.display_name", "project_volumes.volume_mode", "runtime_clusters.name")
		if query.MatchSource {
			base = applyDeploymentBundleSourceMatch(base, reference.Source, map[string]string{
				"name": "project_volumes.display_name", "accessMode": "project_volumes.access_mode", "volumeMode": "project_volumes.volume_mode",
				"clusterName": "runtime_clusters.name", "clusterType": "runtime_clusters.type",
			})
		}
		if err := base.Count(&total).Error; err != nil {
			return deploymentBundleReferenceCandidatePage{}, nil, err
		}
		if err := base.Select("project_volumes.*, runtime_clusters.name as cluster_name, runtime_clusters.type as cluster_type").
			Order(deploymentBundleCandidateOrder(query, "project_volumes.display_name", "project_volumes.created_at", "project_volumes.id")).Offset(offset).Limit(query.Pagination.PageSize).Scan(&items).Error; err != nil {
			return deploymentBundleReferenceCandidatePage{}, nil, err
		}
		for _, item := range items {
			descriptor := deploymentBundleReferenceDescriptor{
				Name: item.DisplayName, AccessMode: item.AccessMode, VolumeMode: item.VolumeMode, StorageClassName: item.StorageClassName,
				ClusterName: item.ClusterName, ClusterType: item.ClusterType,
			}
			compatible := item.LifecycleState == model.ProjectVolumeLifecycleReady && strings.TrimSpace(item.PendingOperation) == ""
			appendCandidate(item.ID, item.DisplayName, item.VolumeMode+" · "+item.ClusterName, descriptor, compatible)
		}

	default:
		return deploymentBundleReferenceCandidatePage{}, nil, &deploymentBundleError{Code: "deployment_bundle.invalid_json", Message: "unsupported deployment bundle reference kind"}
	}
	return deploymentBundleCandidatePage(candidates, total, query.Pagination), candidates, nil
}

// deploymentBundleCompatibleMatches preserves the original auto-match rule:
// only descriptor matches that are also usable in the destination participate
// in uniqueness. It walks every exact-match page when needed, but stops as soon
// as ambiguity is proven by two compatible candidates.
func (h *Handlers) deploymentBundleCompatibleMatches(ctx context.Context, user model.User, project model.Project, app model.Application, reference deploymentBundleReference) ([]deploymentBundleCandidate, error) {
	matches := make([]deploymentBundleCandidate, 0, 2)
	for pageNumber := 1; ; pageNumber++ {
		page, candidates, err := h.deploymentBundleCandidates(ctx, user, project, app, reference, deploymentBundleCandidateQuery{
			Pagination:  paginationParams{Page: pageNumber, PageSize: maxPageSize, SortBy: "name", SortOrder: "asc"},
			MatchSource: true,
		})
		if err != nil {
			return nil, err
		}
		matches = appendCompatibleDeploymentBundleMatches(matches, candidates)
		if len(matches) == 2 {
			return matches, nil
		}
		if pageNumber >= page.TotalPages {
			return matches, nil
		}
	}
}

func appendCompatibleDeploymentBundleMatches(matches, candidates []deploymentBundleCandidate) []deploymentBundleCandidate {
	for _, candidate := range candidates {
		if candidate.Public.Compatible && candidate.Public.Matched {
			matches = append(matches, candidate)
			if len(matches) == 2 {
				return matches
			}
		}
	}
	return matches
}
