package api

import (
	"context"
	"errors"
	"slices"
	"sort"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/buildtemplate"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/resourceidentifier"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

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
	input.Enabled = true
	input.EnvironmentID = ""
	input.BuildEnvironmentID = ""

	preview := deploymentTargetBundlePreview{
		Digest: digest,
		Status: deploymentBundleStatusReady,
		Summary: deploymentTargetBundlePreviewSummary{
			Name: strings.TrimSpace(input.Name), Stage: normalizeStage(input.Stage),
			SourceType: normalizeDeploymentSourceType(input.SourceType),
		},
		References:         make([]deploymentBundleReferenceResolution, 0, len(request.Bundle.References)),
		SecretRequirements: append([]deploymentBundleSecretRequirement(nil), request.Bundle.SecretRequirements...),
		Warnings:           []string{},
	}

	stage, validStage := normalizePublicStage(input.Stage)
	if !validStage {
		preview.Status = deploymentBundleStatusInvalid
		preview.Warnings = append(preview.Warnings, "deployment.stage_invalid")
	} else if err := resourceidentifier.Validate(stage, stageIdentifierMinLength, stageIdentifierMaxLength); err != nil {
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
	if err := h.validateDeploymentBundleVolumeMappings(ctx.Request.Context(), project, input, &preview); err != nil {
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
		input.BuildSecrets != nil || len(input.BuildHookBindings) > 0 || len(input.RuntimeConfigRefs) > 0 ||
		strings.TrimSpace(input.SecretFiles) != "" || input.Enabled {
		return &deploymentBundleError{Code: "deployment_bundle.invalid_json", Message: "deployment bundle contains a non-portable identifier or secret field"}
	}
	if strings.TrimSpace(input.Namespace) != "" || normalizeTriStateBool(input.AllowPrivilegeEscalation) == "true" ||
		strings.TrimSpace(normalizeStringArrayText(input.CapabilityAdd)) != "" ||
		strings.TrimSpace(input.ServiceAccountName) != "" ||
		normalizeTriStateBool(input.AutomountServiceAccountToken) == "true" ||
		(strings.TrimSpace(input.ServiceType) != "" && normalizeServiceType(input.ServiceType) != "ClusterIP") ||
		strings.TrimSpace(input.ServiceExternalTrafficPolicy) != "" {
		return &deploymentBundleError{Code: "deployment_bundle.invalid_json", Message: "deployment bundle contains an unsupported namespace or workload security override"}
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

func (h *Handlers) validateDeploymentBundleVolumeMappings(ctx context.Context, project model.Project, input deploymentTargetInput, preview *deploymentTargetBundlePreview) error {
	for _, dataVolume := range input.DataVolumes {
		if dataVolume.SourceType != "projectVolume" || strings.TrimSpace(dataVolume.ProjectVolumeID) == "" {
			continue
		}
		var projectVolume model.ProjectVolume
		if err := h.dbWithContext(ctx).First(&projectVolume, "id = ? and project_id = ?", dataVolume.ProjectVolumeID, project.ID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				markDeploymentBundleVolumeResolution(preview, dataVolume.LogicalName, deploymentBundleReferenceForbidden, "deployment_bundle.reference_forbidden")
				continue
			}
			return err
		}
		if !deploymentBundleVolumeDestinationCompatible(runtimeProjectNamespace(project), input, projectVolume) {
			markDeploymentBundleVolumeResolution(preview, dataVolume.LogicalName, deploymentBundleReferenceIncompatible, "deployment_bundle.reference_incompatible")
		}
	}
	return nil
}

func deploymentBundleVolumeDestinationCompatible(projectNamespace string, input deploymentTargetInput, projectVolume model.ProjectVolume) bool {
	if strings.TrimSpace(input.ClusterID) == "" || strings.TrimSpace(projectVolume.ClusterID) != strings.TrimSpace(input.ClusterID) {
		return false
	}
	return strings.TrimSpace(projectNamespace) != "" && strings.TrimSpace(projectVolume.Namespace) == strings.TrimSpace(projectNamespace)
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
	page, candidates, err := h.deploymentBundleCandidates(ctx.Request.Context(), user, project, app, reference, deploymentBundleCandidateQuery{
		Pagination: paginationParams{Page: 1, PageSize: defaultPageSize, SortBy: "name", SortOrder: "asc"},
	})
	if err != nil {
		return deploymentBundleReferenceResolution{}, nil, err
	}
	resolution := deploymentBundleReferenceResolution{
		deploymentBundleReference: reference,
		Status:                    deploymentBundleReferenceMissing,
		Candidates:                make([]deploymentBundleReferenceCandidate, 0, len(candidates)),
		CandidateCount:            int(page.Total),
		Truncated:                 page.Total > int64(len(candidates)),
		Code:                      "deployment_bundle.reference_missing",
	}
	for index := range candidates {
		candidates[index].Public.Matched = deploymentBundleReferenceDescriptorMatches(reference.Source, candidates[index].Descriptor)
		resolution.Candidates = append(resolution.Candidates, candidates[index].Public)
	}
	if mappedID != "" {
		_, mappedCandidates, mappedErr := h.deploymentBundleCandidates(ctx.Request.Context(), user, project, app, reference, deploymentBundleCandidateQuery{
			Pagination: paginationParams{Page: 1, PageSize: 1, SortBy: "name", SortOrder: "asc"},
			ID:         mappedID,
		})
		if mappedErr != nil {
			return deploymentBundleReferenceResolution{}, nil, mappedErr
		}
		if len(mappedCandidates) == 0 {
			// Deliberately collapse missing and invisible IDs so project members
			// cannot use this endpoint to enumerate resources outside their scope.
			resolution.Status = deploymentBundleReferenceForbidden
			resolution.Code = "deployment_bundle.reference_forbidden"
			return resolution, candidates, nil
		}
		mappedCandidate := mappedCandidates[0]
		if !slices.ContainsFunc(resolution.Candidates, func(candidate deploymentBundleReferenceCandidate) bool {
			return candidate.ID == mappedCandidate.Public.ID
		}) {
			resolution.Candidates = append(resolution.Candidates, mappedCandidate.Public)
		}
		if !mappedCandidate.Public.Compatible {
			resolution.Status = deploymentBundleReferenceIncompatible
			resolution.Code = "deployment_bundle.reference_incompatible"
			return resolution, candidates, nil
		}
		resolution.Status = deploymentBundleReferenceResolved
		resolution.ResolvedID = mappedCandidate.Public.ID
		resolution.Code = ""
		return resolution, candidates, nil
	}
	matches, matchErr := h.deploymentBundleCompatibleMatches(ctx.Request.Context(), user, project, app, reference)
	if matchErr != nil {
		return deploymentBundleReferenceResolution{}, nil, matchErr
	}
	if len(matches) == 1 {
		if !slices.ContainsFunc(resolution.Candidates, func(candidate deploymentBundleReferenceCandidate) bool { return candidate.ID == matches[0].Public.ID }) {
			resolution.Candidates = append(resolution.Candidates, matches[0].Public)
		}
		resolution.Status = deploymentBundleReferenceResolved
		resolution.ResolvedID = matches[0].Public.ID
		resolution.Code = ""
	} else if len(matches) > 1 {
		resolution.Status = deploymentBundleReferenceAmbiguous
		resolution.Code = "deployment_bundle.reference_ambiguous"
	}
	return resolution, candidates, nil
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
