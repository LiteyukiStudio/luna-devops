package deploymentapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/buildenv"
	"github.com/LiteyukiStudio/devops/internal/model"
	"gorm.io/gorm"
)

func (h *Handlers) buildDeploymentTargetBundle(ctx context.Context, project model.Project, app model.Application, target model.DeploymentTarget) (deploymentTargetBundle, error) {
	targetWithHooks, err := h.deploymentTargetWithHookBindings(target, ctx)
	if err != nil {
		return deploymentTargetBundle{}, err
	}
	mountsByTarget, err := h.deploymentTargetVolumeMountsByTarget(ctx, []model.DeploymentTarget{targetWithHooks})
	if err != nil {
		return deploymentTargetBundle{}, err
	}
	mounts := mountsByTarget[targetWithHooks.ID]

	buildEnvironment, buildEnvironmentErr := h.findBuildEnvironmentConfig(h.dbWithContext(ctx), model.BuildEnvironmentScopeDeployment, targetWithHooks.ID)
	if buildEnvironmentErr != nil && !errors.Is(buildEnvironmentErr, gorm.ErrRecordNotFound) {
		return deploymentTargetBundle{}, buildEnvironmentErr
	}

	references, err := h.deploymentBundleReferences(ctx, targetWithHooks, mounts)
	if err != nil {
		return deploymentTargetBundle{}, err
	}
	secretRequirements := deploymentBundleSecretRequirements(targetWithHooks, buildEnvironment)
	configuration, err := deploymentBundleConfiguration(targetWithHooks, mounts, buildEnvironment)
	if err != nil {
		return deploymentTargetBundle{}, err
	}
	if shouldResetGeneratedTargetRepository(ctx, h, project, app, targetWithHooks) {
		configuration.TargetRepository = ""
		configuration.TargetTag = ""
		configuration.TargetImageRef = ""
	}

	return deploymentTargetBundle{
		SchemaVersion:      deploymentBundleSchemaVersion,
		Kind:               deploymentBundleKind,
		ExportedAt:         time.Now().UTC(),
		Configuration:      configuration,
		References:         references,
		SecretRequirements: secretRequirements,
		Omissions: []string{
			"sourceIdentifiers",
			"secretValues",
			"credentialReferences",
			"runtimeState",
			"buildAndReleaseHistory",
		},
	}, nil
}

func deploymentBundleConfiguration(target model.DeploymentTarget, mounts []model.DeploymentVolumeMount, buildEnvironment model.BuildEnvironmentConfig) (deploymentTargetInput, error) {
	response := deploymentTargetResponseFromModel(target, mounts)
	payload, err := json.Marshal(response)
	if err != nil {
		return deploymentTargetInput{}, err
	}
	values := map[string]json.RawMessage{}
	if err := json.Unmarshal(payload, &values); err != nil {
		return deploymentTargetInput{}, err
	}
	for _, key := range []string{
		"id", "projectId", "applicationId", "kubernetesName", "clusterId", "repositoryBindingId",
		"buildEnvironmentId", "targetRegistryId", "buildVariableSetIds", "buildHookBindings", "runtimeConfigRefs",
		"namespace", "allowPrivilegeEscalation", "capabilityAdd", "serviceAccountName",
		"automountServiceAccountToken", "serviceType", "serviceExternalTrafficPolicy",
		"secretFilesSet", "status", "observationCode", "lastCheckedAt", "desiredReplicas", "updatedReplicas",
		"readyReplicas", "availableReplicas", "deleteStatus", "deleteMessage", "deleteStartedAt", "deleteFinishedAt", "createdBy", "createdAt",
	} {
		delete(values, key)
	}
	portableVolumes := make([]deploymentTargetDataVolumeInput, 0, len(mounts))
	for _, mount := range mounts {
		item := deploymentTargetDataVolumeInput{
			LogicalName: mount.LogicalName,
			MountPath:   optionalStringValue(mount.MountPath),
			DevicePath:  optionalStringValue(mount.DevicePath),
			ReadOnly:    mount.ReadOnly,
		}
		switch mount.SourceType {
		case model.DeploymentVolumeSourceProjectVolume:
			item.SourceType = "projectVolume"
		case model.DeploymentVolumeSourceEmptyDir:
			item.SourceType = "emptyDir"
			item.EmptyDir = &deploymentTargetEmptyDirInput{Medium: mount.EmptyDirMedium, SizeLimit: mount.EmptyDirSizeLimit}
		default:
			continue
		}
		portableVolumes = append(portableVolumes, item)
	}
	if encodedVolumes, marshalErr := json.Marshal(portableVolumes); marshalErr == nil {
		values["dataVolumes"] = encodedVolumes
	} else {
		return deploymentTargetInput{}, marshalErr
	}
	if variables := buildenv.Decode(buildEnvironment.Variables); len(variables) > 0 {
		encodedVariables, marshalErr := json.Marshal(variables)
		if marshalErr != nil {
			return deploymentTargetInput{}, marshalErr
		}
		values["buildVariables"] = encodedVariables
	}
	configurationPayload, err := json.Marshal(values)
	if err != nil {
		return deploymentTargetInput{}, err
	}
	configuration := deploymentTargetInput{}
	if err := json.Unmarshal(configurationPayload, &configuration); err != nil {
		return deploymentTargetInput{}, err
	}
	configuration.Enabled = false
	configuration.ClusterID = ""
	configuration.RepositoryBindingID = ""
	configuration.BuildEnvironmentID = ""
	configuration.TargetRegistryID = ""
	configuration.BuildVariableSetIDs = nil
	configuration.BuildSecrets = nil
	configuration.BuildHookBindings = nil
	configuration.RuntimeConfigRefs = nil
	configuration.EnvironmentVariables = publicEnvironmentVariableInputs(target.EnvVars)
	configuration.SecretFiles = ""
	return configuration, nil
}

func (h *Handlers) deploymentBundleReferences(ctx context.Context, target model.DeploymentTarget, mounts []model.DeploymentVolumeMount) ([]deploymentBundleReference, error) {
	references := make([]deploymentBundleReference, 0)
	if normalizeDeploymentSourceType(target.SourceType) == "repository" {
		descriptor := deploymentBundleReferenceDescriptor{}
		var binding model.RepositoryBinding
		if err := h.dbWithContext(ctx).First(&binding, "id = ? and project_id = ? and application_id = ?", target.RepositoryBindingID, target.ProjectID, target.ApplicationID).Error; err == nil {
			descriptor.Name = strings.TrimSpace(binding.Owner + "/" + binding.Repo)
			descriptor.Owner = binding.Owner
			descriptor.Repository = binding.Repo
			var provider model.GitProvider
			if providerErr := h.dbWithContext(ctx).First(&provider, "id = ?", binding.GitProviderID).Error; providerErr == nil {
				descriptor.Type = provider.Type
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		references = append(references, deploymentBundleReference{Key: "repository", Kind: deploymentBundleReferenceRepositoryBinding, Required: true, Usage: "source", Source: descriptor})
	}
	if strings.TrimSpace(target.ClusterID) != "" {
		descriptor := deploymentBundleReferenceDescriptor{}
		var cluster model.RuntimeCluster
		if err := h.dbWithContext(ctx).First(&cluster, "id = ?", target.ClusterID).Error; err == nil {
			descriptor.Name = cluster.Name
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		references = append(references, deploymentBundleReference{Key: "runtimeCluster", Kind: deploymentBundleReferenceRuntimeCluster, Required: true, Usage: "runtime", Source: descriptor})
	}
	if normalizeDeploymentSourceType(target.SourceType) == "repository" {
		descriptor := deploymentBundleReferenceDescriptor{}
		if strings.TrimSpace(target.TargetRegistryID) != "" {
			var registry model.ArtifactRegistry
			if err := h.dbWithContext(ctx).First(&registry, "id = ?", target.TargetRegistryID).Error; err == nil {
				descriptor.Name = registry.Name
				descriptor.Type = registry.Provider
				descriptor.Namespace = registry.Namespace
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, err
			}
		}
		references = append(references, deploymentBundleReference{Key: "targetRegistry", Kind: deploymentBundleReferenceArtifactRegistry, Required: true, Usage: "buildOutput", Source: descriptor})
	}

	for index, setID := range buildVariableSetIDs(target.BuildVariableSetIDs) {
		descriptor := deploymentBundleReferenceDescriptor{}
		var set model.BuildVariableSet
		if err := h.dbWithContext(ctx).First(&set, "id = ?", setID).Error; err == nil {
			descriptor.Name = set.Name
			descriptor.Scope = set.Scope
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		references = append(references, deploymentBundleReference{Key: fmt.Sprintf("buildVariableSet:%d", index), Kind: deploymentBundleReferenceBuildVariableSet, Required: true, Usage: "buildVariables", Source: descriptor})
	}
	for index, ref := range model.DecodeDeploymentRuntimeConfigRefs(target.RuntimeConfigRefs) {
		descriptor := deploymentBundleReferenceDescriptor{Mode: model.RuntimeConfigRefMode(ref.Mode)}
		var set model.ProjectRuntimeConfigSet
		if err := h.dbWithContext(ctx).First(&set, "id = ? and project_id = ?", ref.SetID, target.ProjectID).Error; err == nil {
			descriptor.Name = set.Name
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		references = append(references, deploymentBundleReference{Key: fmt.Sprintf("runtimeConfigSet:%d", index), Kind: deploymentBundleReferenceRuntimeConfigSet, Required: true, Usage: "runtimeConfig", Source: descriptor})
	}
	for index, binding := range target.BuildHookBindings {
		descriptor := deploymentBundleReferenceDescriptor{Phase: binding.Phase, RunOrder: binding.RunOrder}
		var hook model.ProjectHookConfig
		if err := h.dbWithContext(ctx).First(&hook, "id = ? and project_id = ?", binding.HookConfigID, target.ProjectID).Error; err == nil {
			descriptor.Name = hook.Name
			descriptor.Type = hook.Shell
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		references = append(references, deploymentBundleReference{Key: fmt.Sprintf("hookConfig:%d", index), Kind: deploymentBundleReferenceHookConfig, Required: true, Usage: "buildHook", Source: descriptor})
	}
	volumeIndex := 0
	for _, mount := range mounts {
		if mount.SourceType != model.DeploymentVolumeSourceProjectVolume || mount.ProjectVolumeID == nil {
			continue
		}
		descriptor := deploymentBundleReferenceDescriptor{
			LogicalName: mount.LogicalName,
			MountPath:   optionalStringValue(mount.MountPath),
			DevicePath:  optionalStringValue(mount.DevicePath),
			ReadOnly:    mount.ReadOnly,
		}
		var projectVolume model.ProjectVolume
		if err := h.dbWithContext(ctx).First(&projectVolume, "id = ? and project_id = ?", *mount.ProjectVolumeID, target.ProjectID).Error; err == nil {
			descriptor.Name = projectVolume.DisplayName
			descriptor.AccessMode = projectVolume.AccessMode
			descriptor.VolumeMode = projectVolume.VolumeMode
			descriptor.StorageClassName = projectVolume.StorageClassName
			var cluster model.RuntimeCluster
			if clusterErr := h.dbWithContext(ctx).First(&cluster, "id = ?", projectVolume.ClusterID).Error; clusterErr == nil {
				descriptor.ClusterName = cluster.Name
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		references = append(references, deploymentBundleReference{Key: fmt.Sprintf("projectVolume:%d", volumeIndex), Kind: deploymentBundleReferenceProjectVolume, Required: true, Usage: "dataVolume", Source: descriptor})
		volumeIndex++
	}
	return references, nil
}

func deploymentBundleSecretRequirements(target model.DeploymentTarget, buildEnvironment model.BuildEnvironmentConfig) []deploymentBundleSecretRequirement {
	requirements := make([]deploymentBundleSecretRequirement, 0)
	appendNames := func(targetKind string, values map[string]string) {
		keys := make([]string, 0, len(values))
		for key, ref := range values {
			if strings.TrimSpace(key) != "" && strings.TrimSpace(ref) != "" {
				keys = append(keys, strings.TrimSpace(key))
			}
		}
		sort.Strings(keys)
		for _, key := range keys {
			requirements = append(requirements, deploymentBundleSecretRequirement{
				Key: fmt.Sprintf("secret:%s:%d", targetKind, len(requirements)), Target: targetKind, Name: key,
			})
		}
	}
	appendNames(deploymentBundleSecretBuild, buildenv.Decode(buildEnvironment.SecretRefs))
	appendNames(deploymentBundleSecretRuntimeEnv, decodeSecretRefs(target.SecretRefs))
	paths := make([]string, 0)
	for path, ref := range decodeSecretRefs(target.SecretFiles) {
		if strings.TrimSpace(path) != "" && strings.TrimSpace(ref) != "" {
			paths = append(paths, strings.TrimSpace(path))
		}
	}
	sort.Strings(paths)
	for _, path := range paths {
		requirements = append(requirements, deploymentBundleSecretRequirement{
			Key: fmt.Sprintf("secret:%s:%d", deploymentBundleSecretRuntimeFile, len(requirements)), Target: deploymentBundleSecretRuntimeFile, Path: path,
		})
	}
	return requirements
}

func shouldResetGeneratedTargetRepository(ctx context.Context, h *Handlers, project model.Project, app model.Application, target model.DeploymentTarget) bool {
	if strings.TrimSpace(target.TargetRegistryID) == "" || strings.TrimSpace(target.TargetRepository) == "" {
		return false
	}
	var registry model.ArtifactRegistry
	if err := h.dbWithContext(ctx).First(&registry, "id = ?", target.TargetRegistryID).Error; err != nil {
		return false
	}
	return isDefaultImageRepository(registry, project, app, target.TargetRepository)
}
