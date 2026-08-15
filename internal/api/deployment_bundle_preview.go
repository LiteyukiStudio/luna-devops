package api

import (
	"context"
	"errors"
	"slices"
	"sort"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/internal/buildtemplate"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/resourceidentifier"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const deploymentBundleCandidateLimit = 100

type deploymentBundleCandidate struct {
	Public     deploymentBundleReferenceCandidate
	Descriptor deploymentBundleReferenceDescriptor
}

func (h *Handlers) buildDeploymentTargetImportPlan(ctx *gin.Context, user model.User, project model.Project, app model.Application, request deploymentTargetBundleImportRequest, requireSecrets bool) (deploymentTargetBundleImportPlan, error) {
	if err := validateDeploymentTargetBundle(request.Bundle); err != nil {
		return deploymentTargetBundleImportPlan{}, err
	}
	digest, err := deploymentBundleDigest(request.Bundle)
	if err != nil {
		return deploymentTargetBundleImportPlan{}, err
	}
	if requireSecrets && strings.TrimSpace(request.Digest) == "" {
		return deploymentTargetBundleImportPlan{}, &deploymentBundleError{Code: "deployment_bundle.digest_mismatch", Message: "deployment bundle import requires a successful preview digest"}
	}
	if strings.TrimSpace(request.Digest) != "" && !strings.EqualFold(strings.TrimSpace(request.Digest), digest) {
		return deploymentTargetBundleImportPlan{}, &deploymentBundleError{Code: "deployment_bundle.digest_mismatch", Message: "deployment bundle changed after preview"}
	}
	referenceKeys := make(map[string]bool, len(request.Bundle.References))
	for _, reference := range request.Bundle.References {
		referenceKeys[reference.Key] = true
	}
	for key := range request.Mappings {
		if !referenceKeys[key] {
			return deploymentTargetBundleImportPlan{}, &deploymentBundleError{Code: "deployment_bundle.invalid_json", Message: "deployment bundle mapping references an unknown key"}
		}
	}
	requirementKeys := make(map[string]bool, len(request.Bundle.SecretRequirements))
	for _, requirement := range request.Bundle.SecretRequirements {
		requirementKeys[requirement.Key] = true
	}
	for key := range request.SecretValues {
		if !requirementKeys[key] {
			return deploymentTargetBundleImportPlan{}, &deploymentBundleError{Code: "deployment_bundle.secret_requirement_invalid", Message: "deployment bundle secret value references an unknown requirement"}
		}
	}
	secretValues, err := deploymentBundleSecretValues(request.Bundle.SecretRequirements, request.SecretValues, requireSecrets)
	if err != nil {
		return deploymentTargetBundleImportPlan{}, err
	}
	input := request.Bundle.Configuration
	if strings.TrimSpace(request.Overrides.Name) != "" {
		input.Name = strings.TrimSpace(request.Overrides.Name)
	}
	if strings.TrimSpace(request.Overrides.Stage) != "" {
		input.Stage = strings.TrimSpace(request.Overrides.Stage)
	}
	if request.Overrides.Namespace != nil {
		input.Namespace = strings.TrimSpace(*request.Overrides.Namespace)
	}
	input.Enabled = true
	input.EnvironmentID = ""
	input.BuildEnvironmentID = ""

	preview := deploymentTargetBundlePreview{
		Digest: digest,
		Status: deploymentBundleStatusReady,
		Summary: deploymentTargetBundlePreviewSummary{
			Name: strings.TrimSpace(input.Name), Stage: normalizeStage(input.Stage), Namespace: strings.TrimSpace(input.Namespace),
			SourceType: normalizeDeploymentSourceType(input.SourceType),
		},
		References:         make([]deploymentBundleReferenceResolution, 0, len(request.Bundle.References)),
		SecretRequirements: append([]deploymentBundleSecretRequirement(nil), request.Bundle.SecretRequirements...),
		Warnings:           []string{},
	}
	if strings.TrimSpace(input.Namespace) != "" {
		preview.Warnings = append(preview.Warnings, "deployment_bundle.namespace_review_required")
	}

	stage := normalizeStage(input.Stage)
	if err := resourceidentifier.Validate(stage, stageIdentifierMinLength, stageIdentifierMaxLength); err != nil {
		preview.Status = deploymentBundleStatusInvalid
		preview.Warnings = append(preview.Warnings, "deployment.stage_invalid")
	} else {
		var count int64
		if err := h.dbFor(ctx).Model(&model.DeploymentTarget{}).Where("application_id = ? and stage = ?", app.ID, stage).Count(&count).Error; err != nil {
			return deploymentTargetBundleImportPlan{}, err
		}
		if count > 0 {
			preview.Status = deploymentBundleStatusRequiresMapping
			preview.Warnings = append(preview.Warnings, "deployment_bundle.stage_conflict")
		}
	}
	input.Stage = stage

	resolved := map[string]string{}
	for _, reference := range request.Bundle.References {
		resolution, candidates, resolveErr := h.resolveDeploymentBundleReference(ctx, user, project, app, reference, strings.TrimSpace(request.Mappings[reference.Key]))
		if resolveErr != nil {
			return deploymentTargetBundleImportPlan{}, resolveErr
		}
		preview.References = append(preview.References, resolution)
		if resolution.Status != deploymentBundleReferenceResolved {
			if preview.Status != deploymentBundleStatusInvalid {
				preview.Status = deploymentBundleStatusRequiresMapping
			}
			continue
		}
		resolved[reference.Key] = resolution.ResolvedID
		if err := applyDeploymentBundleResolution(&input, reference, resolution.ResolvedID); err != nil {
			return deploymentTargetBundleImportPlan{}, err
		}
		_ = candidates
	}
	if err := validateResolvedDeploymentBundle(input, request.Bundle.References, resolved); err != nil {
		if preview.Status == deploymentBundleStatusReady {
			preview.Status = deploymentBundleStatusInvalid
		}
		preview.Warnings = append(preview.Warnings, deploymentBundleErrorCode(err))
	}
	if err := h.validateDeploymentBundleVolumeMappings(ctx.Request.Context(), project.ID, input, &preview); err != nil {
		return deploymentTargetBundleImportPlan{}, err
	}
	preview.Warnings = uniqueStrings(preview.Warnings)

	return deploymentTargetBundleImportPlan{Preview: preview, Input: input, SecretValues: secretValues}, nil
}

func deploymentBundleSecretValues(requirements []deploymentBundleSecretRequirement, values map[string]string, required bool) ([]deploymentBundleSecretValue, error) {
	result := make([]deploymentBundleSecretValue, 0, len(requirements))
	for _, requirement := range requirements {
		value := values[requirement.Key]
		if required && strings.TrimSpace(value) == "" {
			return nil, &deploymentBundleError{Code: "deployment_bundle.secret_required", Message: "all deployment bundle secrets must be provided again"}
		}
		if len(value) > deploymentBundleSecretMaxBytes {
			return nil, &deploymentBundleError{Code: "deployment_bundle.secret_requirement_invalid", Message: "deployment bundle secret value exceeds the supported size"}
		}
		result = append(result, deploymentBundleSecretValue{Requirement: requirement, Value: value})
	}
	return result, nil
}

func validateDeploymentTargetBundle(bundle deploymentTargetBundle) error {
	if bundle.Kind != deploymentBundleKind {
		return &deploymentBundleError{Code: "deployment_bundle.unsupported_kind", Message: "unsupported deployment bundle kind"}
	}
	if bundle.SchemaVersion != deploymentBundleSchemaVersion {
		return &deploymentBundleError{Code: "deployment_bundle.unsupported_version", Message: "unsupported deployment bundle schema version"}
	}
	if len(bundle.References) > deploymentBundleMaxReferences || len(bundle.SecretRequirements) > deploymentBundleMaxReferences {
		return &deploymentBundleError{Code: "deployment_bundle.invalid_json", Message: "deployment bundle contains too many references or secret requirements"}
	}
	input := bundle.Configuration
	if strings.TrimSpace(input.EnvironmentID) != "" || strings.TrimSpace(input.ClusterID) != "" || strings.TrimSpace(input.RepositoryBindingID) != "" ||
		strings.TrimSpace(input.BuildEnvironmentID) != "" || strings.TrimSpace(input.TargetRegistryID) != "" || len(input.BuildVariableSetIDs) > 0 ||
		input.BuildSecrets != nil || len(input.BuildHookBindings) > 0 || len(input.RuntimeConfigSetIDs) > 0 || len(input.RuntimeConfigRefs) > 0 ||
		strings.TrimSpace(input.SecretRefs) != "" || strings.TrimSpace(input.SecretFiles) != "" || input.Enabled {
		return &deploymentBundleError{Code: "deployment_bundle.invalid_json", Message: "deployment bundle contains a non-portable identifier or secret field"}
	}
	for _, volume := range input.DataVolumes {
		if volume.SourceType == "projectVolume" && strings.TrimSpace(volume.ProjectVolumeID) != "" {
			return &deploymentBundleError{Code: "deployment_bundle.invalid_json", Message: "deployment bundle contains a project volume identifier"}
		}
	}
	seenReferences := map[string]bool{}
	for _, reference := range bundle.References {
		if strings.TrimSpace(reference.Key) == "" || seenReferences[reference.Key] || !deploymentBundleReferenceKindAllowed(reference.Kind) {
			return &deploymentBundleError{Code: "deployment_bundle.invalid_json", Message: "deployment bundle contains an invalid or duplicate reference"}
		}
		seenReferences[reference.Key] = true
	}
	seenRequirements := map[string]bool{}
	seenSecretDestinations := map[string]bool{}
	for _, requirement := range bundle.SecretRequirements {
		if strings.TrimSpace(requirement.Key) == "" || seenRequirements[requirement.Key] || !deploymentBundleSecretTargetAllowed(requirement.Target) {
			return &deploymentBundleError{Code: "deployment_bundle.invalid_json", Message: "deployment bundle contains an invalid or duplicate secret requirement"}
		}
		if requirement.Target == deploymentBundleSecretRuntimeFile && strings.TrimSpace(requirement.Path) == "" {
			return &deploymentBundleError{Code: "deployment_bundle.invalid_json", Message: "deployment bundle secret file requirement is missing a path"}
		}
		if requirement.Target != deploymentBundleSecretRuntimeFile && strings.TrimSpace(requirement.Name) == "" {
			return &deploymentBundleError{Code: "deployment_bundle.invalid_json", Message: "deployment bundle secret requirement is missing a name"}
		}
		destination := strings.TrimSpace(requirement.Name)
		if requirement.Target == deploymentBundleSecretRuntimeFile {
			destination = strings.TrimSpace(requirement.Path)
		}
		destinationKey := requirement.Target + ":" + destination
		if seenSecretDestinations[destinationKey] {
			return &deploymentBundleError{Code: "deployment_bundle.invalid_json", Message: "deployment bundle contains duplicate secret destinations"}
		}
		seenRequirements[requirement.Key] = true
		seenSecretDestinations[destinationKey] = true
	}
	return nil
}

func deploymentBundleReferenceKindAllowed(kind string) bool {
	return slices.Contains([]string{
		deploymentBundleReferenceRepositoryBinding,
		deploymentBundleReferenceRuntimeCluster,
		deploymentBundleReferenceArtifactRegistry,
		deploymentBundleReferenceBuildVariableSet,
		deploymentBundleReferenceRuntimeConfigSet,
		deploymentBundleReferenceHookConfig,
		deploymentBundleReferenceProjectVolume,
	}, kind)
}

func deploymentBundleSecretTargetAllowed(target string) bool {
	return slices.Contains([]string{deploymentBundleSecretBuild, deploymentBundleSecretRuntimeEnv, deploymentBundleSecretRuntimeFile}, target)
}

func validateResolvedDeploymentBundle(input deploymentTargetInput, references []deploymentBundleReference, resolved map[string]string) error {
	sourceType := normalizeDeploymentSourceType(input.SourceType)
	if sourceType == "repository" {
		if strings.TrimSpace(input.RepositoryBindingID) == "" {
			return &deploymentBundleError{Code: "deployment_bundle.repository_binding_missing", Message: "repository source requires a repository binding in the destination application"}
		}
		if strings.TrimSpace(input.TargetRegistryID) == "" {
			return &deploymentBundleError{Code: "deployment_bundle.registry_push_credential_required", Message: "repository source requires a destination registry with push credentials"}
		}
	} else if strings.TrimSpace(input.ImageRef) == "" {
		return &deploymentBundleError{Code: "deployment_target.image_ref_required", Message: "image source requires an image reference"}
	}
	for _, reference := range references {
		if reference.Required && strings.TrimSpace(resolved[reference.Key]) == "" {
			return &deploymentBundleError{Code: "deployment_bundle.reference_missing", Message: "required deployment bundle reference is unresolved"}
		}
	}
	for _, dataVolume := range input.DataVolumes {
		if dataVolume.SourceType == "projectVolume" && strings.TrimSpace(dataVolume.ProjectVolumeID) == "" {
			return &deploymentBundleError{Code: "deployment_bundle.reference_missing", Message: "project volume mount requires a resolved destination volume"}
		}
	}
	if strings.TrimSpace(input.BuildDefinitionMode) == buildtemplate.DefinitionModeTemplate {
		if _, found := buildtemplate.Find(input.BuildTemplateID, input.BuildTemplateVersion); !found {
			return &deploymentBundleError{Code: "build_template.not_found", Message: "deployment bundle build template is unavailable"}
		}
	}
	return nil
}

func (h *Handlers) validateDeploymentBundleVolumeMappings(ctx context.Context, projectID string, input deploymentTargetInput, preview *deploymentTargetBundlePreview) error {
	for _, dataVolume := range input.DataVolumes {
		if dataVolume.SourceType != "projectVolume" || strings.TrimSpace(dataVolume.ProjectVolumeID) == "" {
			continue
		}
		var projectVolume model.ProjectVolume
		if err := h.dbWithContext(ctx).First(&projectVolume, "id = ? and project_id = ?", dataVolume.ProjectVolumeID, projectID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				markDeploymentBundleVolumeResolution(preview, dataVolume.LogicalName, deploymentBundleReferenceForbidden, "deployment_bundle.reference_forbidden")
				continue
			}
			return err
		}
		if !deploymentBundleVolumeDestinationCompatible(input, projectVolume) {
			markDeploymentBundleVolumeResolution(preview, dataVolume.LogicalName, deploymentBundleReferenceIncompatible, "deployment_bundle.reference_incompatible")
		}
	}
	return nil
}

func deploymentBundleVolumeDestinationCompatible(input deploymentTargetInput, projectVolume model.ProjectVolume) bool {
	if strings.TrimSpace(input.ClusterID) == "" || strings.TrimSpace(projectVolume.ClusterID) != strings.TrimSpace(input.ClusterID) {
		return false
	}
	return strings.TrimSpace(input.Namespace) == "" || strings.TrimSpace(projectVolume.Namespace) == strings.TrimSpace(input.Namespace)
}

func markDeploymentBundleVolumeResolution(preview *deploymentTargetBundlePreview, logicalName, status, code string) {
	for index := range preview.References {
		if preview.References[index].Kind != deploymentBundleReferenceProjectVolume || preview.References[index].Source.LogicalName != logicalName {
			continue
		}
		preview.References[index].Status = status
		preview.References[index].Code = code
		preview.References[index].ResolvedID = ""
	}
	if preview.Status != deploymentBundleStatusInvalid {
		preview.Status = deploymentBundleStatusRequiresMapping
	}
	preview.Warnings = append(preview.Warnings, code)
}

func (h *Handlers) resolveDeploymentBundleReference(ctx *gin.Context, user model.User, project model.Project, app model.Application, reference deploymentBundleReference, mappedID string) (deploymentBundleReferenceResolution, []deploymentBundleCandidate, error) {
	candidates, total, err := h.deploymentBundleCandidates(ctx.Request.Context(), user, project, app, reference)
	if err != nil {
		return deploymentBundleReferenceResolution{}, nil, err
	}
	resolution := deploymentBundleReferenceResolution{
		deploymentBundleReference: reference,
		Status:                    deploymentBundleReferenceMissing,
		Candidates:                make([]deploymentBundleReferenceCandidate, 0, len(candidates)),
		CandidateCount:            total,
		Truncated:                 total > len(candidates),
		Code:                      "deployment_bundle.reference_missing",
	}
	for index := range candidates {
		candidates[index].Public.Matched = deploymentBundleReferenceDescriptorMatches(reference.Source, candidates[index].Descriptor)
		resolution.Candidates = append(resolution.Candidates, candidates[index].Public)
	}
	if mappedID != "" {
		for _, candidate := range candidates {
			if candidate.Public.ID != mappedID {
				continue
			}
			if !candidate.Public.Compatible {
				resolution.Status = deploymentBundleReferenceIncompatible
				resolution.Code = "deployment_bundle.reference_incompatible"
				return resolution, candidates, nil
			}
			resolution.Status = deploymentBundleReferenceResolved
			resolution.ResolvedID = candidate.Public.ID
			resolution.Code = ""
			return resolution, candidates, nil
		}
		resolution.Status = deploymentBundleReferenceForbidden
		resolution.Code = "deployment_bundle.reference_forbidden"
		return resolution, candidates, nil
	}
	matches := make([]deploymentBundleCandidate, 0)
	for _, candidate := range candidates {
		if candidate.Public.Compatible && candidate.Public.Matched {
			matches = append(matches, candidate)
		}
	}
	if len(matches) == 1 {
		resolution.Status = deploymentBundleReferenceResolved
		resolution.ResolvedID = matches[0].Public.ID
		resolution.Code = ""
	} else if len(matches) > 1 {
		resolution.Status = deploymentBundleReferenceAmbiguous
		resolution.Code = "deployment_bundle.reference_ambiguous"
	}
	return resolution, candidates, nil
}

func (h *Handlers) deploymentBundleCandidates(ctx context.Context, user model.User, project model.Project, app model.Application, reference deploymentBundleReference) ([]deploymentBundleCandidate, int, error) {
	candidates := make([]deploymentBundleCandidate, 0)
	appendCandidate := func(id, name, description string, descriptor deploymentBundleReferenceDescriptor, compatible bool) {
		candidates = append(candidates, deploymentBundleCandidate{
			Public:     deploymentBundleReferenceCandidate{ID: id, Name: name, Description: description, Compatible: compatible},
			Descriptor: descriptor,
		})
	}
	switch reference.Kind {
	case deploymentBundleReferenceRepositoryBinding:
		var items []struct {
			model.RepositoryBinding
			ProviderType string `gorm:"column:provider_type"`
		}
		err := h.dbWithContext(ctx).Table("repository_bindings").
			Select("repository_bindings.*, git_providers.type as provider_type").
			Joins("join git_providers on git_providers.id = repository_bindings.git_provider_id and git_providers.deleted_at is null").
			Where("repository_bindings.project_id = ? and repository_bindings.application_id = ? and repository_bindings.deleted_at is null", project.ID, app.ID).
			Order("repository_bindings.created_at asc").Limit(deploymentBundleCandidateLimit + 1).Scan(&items).Error
		if err != nil {
			return nil, 0, err
		}
		for _, item := range items {
			name := strings.Trim(strings.TrimSpace(item.Owner)+"/"+strings.TrimSpace(item.Repo), "/")
			appendCandidate(item.ID, name, item.ProviderType, deploymentBundleReferenceDescriptor{Name: name, Type: item.ProviderType, Owner: item.Owner, Repository: item.Repo}, true)
		}
	case deploymentBundleReferenceRuntimeCluster:
		var items []model.RuntimeCluster
		query := h.applyScopedResourceVisibilityForProject(h.dbWithContext(ctx).Model(&model.RuntimeCluster{}), scopedResourceRuntimeCluster, user, project.ID, ctx)
		if err := query.Where("type in ?", []string{"kubernetes", "k3s"}).Order("name asc, created_at asc").Limit(deploymentBundleCandidateLimit + 1).Find(&items).Error; err != nil {
			return nil, 0, err
		}
		for _, item := range items {
			appendCandidate(item.ID, item.Name, item.Type, deploymentBundleReferenceDescriptor{Name: item.Name, Type: item.Type}, true)
		}
	case deploymentBundleReferenceArtifactRegistry:
		var items []model.ArtifactRegistry
		query := h.applyScopedResourceVisibilityForProject(h.dbWithContext(ctx).Model(&model.ArtifactRegistry{}), scopedResourceArtifactRegistry, user, project.ID, ctx)
		if err := query.Order("name asc, created_at asc").Limit(deploymentBundleCandidateLimit + 1).Find(&items).Error; err != nil {
			return nil, 0, err
		}
		for _, item := range items {
			_, hasPushCredential := h.registryPushCredentialForProject(user, item, project.ID, ctx)
			appendCandidate(item.ID, item.Name, item.Provider, deploymentBundleReferenceDescriptor{Name: item.Name, Type: item.Provider, Namespace: item.Namespace}, hasPushCredential)
		}
	case deploymentBundleReferenceBuildVariableSet:
		var items []model.BuildVariableSet
		query := h.applyScopedResourceVisibilityForProject(h.dbWithContext(ctx).Model(&model.BuildVariableSet{}), scopedResourceBuildVariableSet, user, project.ID, ctx)
		if err := query.Where("enabled = ?", true).Order("name asc, created_at asc").Limit(deploymentBundleCandidateLimit + 1).Find(&items).Error; err != nil {
			return nil, 0, err
		}
		for _, item := range items {
			appendCandidate(item.ID, item.Name, item.Scope, deploymentBundleReferenceDescriptor{Name: item.Name, Scope: item.Scope}, true)
		}
	case deploymentBundleReferenceRuntimeConfigSet:
		var items []model.ProjectRuntimeConfigSet
		if err := h.dbWithContext(ctx).Where("project_id = ? and enabled = ? and delete_status = ?", project.ID, true, "active").Order("name asc, created_at asc").Limit(deploymentBundleCandidateLimit + 1).Find(&items).Error; err != nil {
			return nil, 0, err
		}
		for _, item := range items {
			appendCandidate(item.ID, item.Name, "", deploymentBundleReferenceDescriptor{Name: item.Name}, true)
		}
	case deploymentBundleReferenceHookConfig:
		var items []model.ProjectHookConfig
		if err := h.dbWithContext(ctx).Where("project_id = ?", project.ID).Order("name asc, created_at asc").Limit(deploymentBundleCandidateLimit + 1).Find(&items).Error; err != nil {
			return nil, 0, err
		}
		for _, item := range items {
			appendCandidate(item.ID, item.Name, item.Shell, deploymentBundleReferenceDescriptor{Name: item.Name, Type: item.Shell}, true)
		}
	case deploymentBundleReferenceProjectVolume:
		var items []model.ProjectVolume
		if err := h.dbWithContext(ctx).Where("project_id = ?", project.ID).Order("display_name asc, created_at asc").Limit(deploymentBundleCandidateLimit + 1).Find(&items).Error; err != nil {
			return nil, 0, err
		}
		clusterNames := map[string]model.RuntimeCluster{}
		clusterIDs := make([]string, 0)
		for _, item := range items {
			clusterIDs = append(clusterIDs, item.ClusterID)
		}
		var clusters []model.RuntimeCluster
		if len(clusterIDs) > 0 {
			_ = h.dbWithContext(ctx).Where("id in ?", uniqueStrings(clusterIDs)).Find(&clusters).Error
		}
		for _, cluster := range clusters {
			clusterNames[cluster.ID] = cluster
		}
		for _, item := range items {
			cluster := clusterNames[item.ClusterID]
			descriptor := deploymentBundleReferenceDescriptor{
				Name: item.DisplayName, AccessMode: item.AccessMode, VolumeMode: item.VolumeMode, StorageClassName: item.StorageClassName,
				ClusterName: cluster.Name, ClusterType: cluster.Type,
			}
			compatible := item.LifecycleState == model.ProjectVolumeLifecycleReady && strings.TrimSpace(item.PendingOperation) == ""
			appendCandidate(item.ID, item.DisplayName, item.VolumeMode+" · "+cluster.Name, descriptor, compatible)
		}
	default:
		return nil, 0, &deploymentBundleError{Code: "deployment_bundle.invalid_json", Message: "unsupported deployment bundle reference kind"}
	}
	total := len(candidates)
	if total > deploymentBundleCandidateLimit {
		candidates = candidates[:deploymentBundleCandidateLimit]
	}
	for index := range candidates {
		if reference.Kind == deploymentBundleReferenceProjectVolume && !deploymentBundleReferenceDescriptorMatches(reference.Source, candidates[index].Descriptor) {
			// A user may explicitly map a differently named volume, but mode and
			// access semantics must remain compatible.
			candidates[index].Public.Compatible = candidates[index].Public.Compatible &&
				reference.Source.VolumeMode == candidates[index].Descriptor.VolumeMode &&
				reference.Source.AccessMode == candidates[index].Descriptor.AccessMode
		}
	}
	return candidates, total, nil
}

func deploymentBundleReferenceDescriptorMatches(source, candidate deploymentBundleReferenceDescriptor) bool {
	equal := func(left, right string) bool {
		return strings.EqualFold(strings.TrimSpace(left), strings.TrimSpace(right))
	}
	checks := [][2]string{
		{source.Name, candidate.Name}, {source.Type, candidate.Type}, {source.Scope, candidate.Scope},
		{source.Owner, candidate.Owner}, {source.Repository, candidate.Repository}, {source.Namespace, candidate.Namespace},
		{source.AccessMode, candidate.AccessMode}, {source.VolumeMode, candidate.VolumeMode},
		{source.ClusterName, candidate.ClusterName}, {source.ClusterType, candidate.ClusterType},
	}
	for _, check := range checks {
		if strings.TrimSpace(check[0]) != "" && !equal(check[0], check[1]) {
			return false
		}
	}
	return strings.TrimSpace(source.Name) != "" || strings.TrimSpace(source.Owner) != "" || strings.TrimSpace(source.Repository) != ""
}

func applyDeploymentBundleResolution(input *deploymentTargetInput, reference deploymentBundleReference, resolvedID string) error {
	switch reference.Kind {
	case deploymentBundleReferenceRepositoryBinding:
		input.RepositoryBindingID = resolvedID
	case deploymentBundleReferenceRuntimeCluster:
		input.ClusterID = resolvedID
	case deploymentBundleReferenceArtifactRegistry:
		input.TargetRegistryID = resolvedID
	case deploymentBundleReferenceBuildVariableSet:
		input.BuildVariableSetIDs = append(input.BuildVariableSetIDs, resolvedID)
	case deploymentBundleReferenceRuntimeConfigSet:
		input.RuntimeConfigRefs = append(input.RuntimeConfigRefs, deploymentRuntimeConfigRefInput{SetID: resolvedID, Mode: model.RuntimeConfigRefMode(reference.Source.Mode)})
	case deploymentBundleReferenceHookConfig:
		input.BuildHookBindings = append(input.BuildHookBindings, deploymentTargetHookBindingInput{HookConfigID: resolvedID, Phase: reference.Source.Phase, RunOrder: reference.Source.RunOrder})
	case deploymentBundleReferenceProjectVolume:
		for index := range input.DataVolumes {
			if input.DataVolumes[index].SourceType == "projectVolume" && input.DataVolumes[index].LogicalName == reference.Source.LogicalName {
				input.DataVolumes[index].ProjectVolumeID = resolvedID
				return nil
			}
		}
		return &deploymentBundleError{Code: "deployment_bundle.reference_incompatible", Message: "project volume reference does not match a portable mount"}
	default:
		return &deploymentBundleError{Code: "deployment_bundle.invalid_json", Message: "unsupported deployment bundle reference kind"}
	}
	return nil
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func deploymentBundleProjectRoleAllowsSecrets(ctx context.Context, db *gorm.DB, user model.User, projectID string) bool {
	if authz.IsPlatformAdmin(user.Role) {
		return true
	}
	var member model.ProjectMember
	if err := db.WithContext(ctx).First(&member, "project_id = ? and user_id = ?", projectID, user.ID).Error; err != nil {
		return false
	}
	return projectRoleAllowed(member.Role, []string{authz.ProjectRoleOwner, authz.ProjectRoleAdmin})
}
