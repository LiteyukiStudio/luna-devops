package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/buildtemplate"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/runtimeconfig"
	"github.com/gin-gonic/gin"
)

func (h *Handlers) deploymentTargetFromInput(ctx *gin.Context, user model.User, app model.Application, input deploymentTargetInput, targetID, kubernetesName string, existingSecretFiles map[string]string, existingRuntimeConfigRefs string) (model.DeploymentTarget, []deploymentTargetDataVolumeInput, bool) {
	_, ok := h.projectNamespaceForDeploymentTarget(ctx, app.ProjectID)
	if !ok {
		return model.DeploymentTarget{}, nil, false
	}
	if namespace := strings.TrimSpace(input.Namespace); namespace != "" {
		writeErrorCode(ctx, http.StatusBadRequest, "deployment_target.namespace_forbidden", "deployment target namespace is managed by the project and cannot be overridden")
		return model.DeploymentTarget{}, nil, false
	}
	sourceType := normalizeDeploymentSourceType(input.SourceType)
	repositoryBindingID := strings.TrimSpace(input.RepositoryBindingID)
	if sourceType == "repository" {
		if repositoryBindingID == "" {
			writeErrorCode(ctx, http.StatusBadRequest, "deployment_target.repository_binding_required", "代码仓库不能为空")
			return model.DeploymentTarget{}, nil, false
		}
		var binding model.RepositoryBinding
		if err := h.dbFor(ctx).First(&binding, "id = ? and project_id = ? and application_id = ?", repositoryBindingID, app.ProjectID, app.ID).Error; err != nil {
			writeErrorCode(ctx, http.StatusBadRequest, "deployment_target.repository_binding_not_found", "代码仓库绑定不存在")
			return model.DeploymentTarget{}, nil, false
		}
	} else if strings.TrimSpace(input.ImageRef) == "" {
		writeErrorCode(ctx, http.StatusBadRequest, "deployment_target.image_ref_required", "镜像地址不能为空")
		return model.DeploymentTarget{}, nil, false
	}
	targetRepository, targetTag := splitTargetImageRef(input.TargetImageRef)
	if targetRepository == "" {
		targetRepository = strings.Trim(strings.TrimSpace(input.TargetRepository), "/")
		targetTag = strings.TrimSpace(input.TargetTag)
	}
	stage := normalizeStage(input.Stage)
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = stage
	}
	buildHooksEnabled := true
	if input.BuildHooksEnabled != nil {
		buildHooksEnabled = *input.BuildHooksEnabled
	}
	dataVolumes, ok := normalizeDataVolumes(ctx, input.DataVolumes)
	if !ok {
		return model.DeploymentTarget{}, nil, false
	}
	servicePorts, ok := normalizeDeploymentServicePorts(ctx, input.ServicePorts, 8080)
	if !ok {
		return model.DeploymentTarget{}, nil, false
	}
	replicas := input.Replicas
	if replicas <= 0 {
		replicas = 1
	}
	runtimeCPURequest, ok := normalizeBuildResourceQuantity(ctx, input.CPURequest, model.DefaultDeploymentCPURequest, "运行 CPU")
	if !ok {
		return model.DeploymentTarget{}, nil, false
	}
	runtimeMemoryRequest, ok := normalizeBuildResourceQuantity(ctx, input.MemoryRequest, model.DefaultDeploymentMemoryRequest, "运行内存")
	if !ok {
		return model.DeploymentTarget{}, nil, false
	}
	kubernetesAdvanced, ok := normalizeDeploymentKubernetesAdvanced(ctx, input)
	if !ok {
		return model.DeploymentTarget{}, nil, false
	}
	autoScaling, ok := normalizeDeploymentAutoScaling(ctx, input, replicas)
	if !ok {
		return model.DeploymentTarget{}, nil, false
	}
	buildCPURequest, ok := normalizeBuildResourceQuantity(ctx, input.BuildCPURequest, defaultBuildCPURequest, "构建 CPU")
	if !ok {
		return model.DeploymentTarget{}, nil, false
	}
	buildMemoryRequest, ok := normalizeBuildResourceQuantity(ctx, input.BuildMemoryRequest, defaultBuildMemoryRequest, "构建内存")
	if !ok {
		return model.DeploymentTarget{}, nil, false
	}
	buildTimeoutSeconds, ok := normalizeBuildTimeoutSeconds(ctx, input.BuildTimeoutSeconds)
	if !ok {
		return model.DeploymentTarget{}, nil, false
	}
	buildArgs, ok := normalizeBuildArgsInput(ctx, input.BuildArgs)
	if !ok {
		return model.DeploymentTarget{}, nil, false
	}
	buildDefinitionMode := buildtemplate.DefinitionModeRepository
	buildTemplateID := ""
	buildTemplateVersion := ""
	buildTemplateValues := "{}"
	if sourceType == "repository" && strings.TrimSpace(input.BuildDefinitionMode) == buildtemplate.DefinitionModeTemplate {
		buildDefinitionMode = buildtemplate.DefinitionModeTemplate
		buildTemplateID = strings.TrimSpace(input.BuildTemplateID)
		buildTemplateVersion = strings.TrimSpace(input.BuildTemplateVersion)
		definition, found := buildtemplate.Find(buildTemplateID, buildTemplateVersion)
		if !found {
			writeErrorCode(ctx, http.StatusBadRequest, "build_template.not_found", "build template not found")
			return model.DeploymentTarget{}, nil, false
		}
		values, err := buildtemplate.NormalizeValues(definition, input.BuildTemplateValues)
		if err != nil {
			writeErrorCode(ctx, http.StatusBadRequest, "build_template.invalid", "build template values are invalid")
			return model.DeploymentTarget{}, nil, false
		}
		buildTemplateVersion = definition.Version
		buildTemplateValues = buildtemplate.EncodeValues(values)
	}
	clusterID := strings.TrimSpace(input.ClusterID)
	targetRegistryID := strings.TrimSpace(input.TargetRegistryID)
	if _, ok := h.runtimeClusterForProjectUse(ctx, user, app.ProjectID, clusterID); !ok {
		return model.DeploymentTarget{}, nil, false
	}
	targetRepository, targetTag, ok = h.applyRegistryCredentialImageTemplate(ctx, user, app, sourceType, targetRegistryID, targetRepository, targetTag, model.DeploymentTarget{
		ID:    targetID,
		Name:  name,
		Stage: stage,
	})
	if !ok {
		return model.DeploymentTarget{}, nil, false
	}
	publicEnvironment, ok := normalizePublicEnvironmentVariables(ctx, input.EnvironmentVariables)
	if !ok {
		return model.DeploymentTarget{}, nil, false
	}
	envVars, err := runtimeconfig.EncodeKeyValue(publicEnvironment)
	if err != nil {
		writeErrorCode(ctx, http.StatusBadRequest, "deployment.runtime_config_invalid", "运行时环境变量格式无效")
		return model.DeploymentTarget{}, nil, false
	}
	runtimeConfigRefs, ok := h.runtimeConfigRefsFromInput(ctx, app.ProjectID, input, existingRuntimeConfigRefs)
	if !ok {
		return model.DeploymentTarget{}, nil, false
	}
	configFiles, ok := normalizeRuntimeConfigFilesInput(ctx, input.ConfigFiles)
	if !ok {
		return model.DeploymentTarget{}, nil, false
	}
	secretFiles, ok := h.runtimeSecretFilesFromInput(ctx, user, targetID, input.SecretFiles, existingSecretFiles)
	if !ok {
		return model.DeploymentTarget{}, nil, false
	}
	secretFilesContent, err := json.Marshal(secretFiles)
	if err != nil {
		writeErrorCode(ctx, http.StatusInternalServerError, "deployment.runtime_secret_files_invalid", "runtime secret files could not be encoded")
		return model.DeploymentTarget{}, nil, false
	}
	for _, volume := range dataVolumes {
		if runtimeDataPathConflicts(volume.MountPath, configFiles, string(secretFilesContent)) {
			writeErrorCode(ctx, http.StatusBadRequest, "deployment.runtime_path_invalid", "deployment runtime paths conflict")
			return model.DeploymentTarget{}, nil, false
		}
	}
	return model.DeploymentTarget{
		ID:                           targetID,
		ProjectID:                    app.ProjectID,
		ApplicationID:                app.ID,
		EnvironmentID:                strings.TrimSpace(input.EnvironmentID),
		Name:                         name,
		Stage:                        stage,
		KubernetesName:               kubernetesName,
		ClusterID:                    clusterID,
		WorkloadType:                 normalizeWorkloadType(input.WorkloadType),
		Replicas:                     replicas,
		CPURequest:                   runtimeCPURequest,
		MemoryRequest:                runtimeMemoryRequest,
		CPULimit:                     "",
		MemoryLimit:                  "",
		ImagePullPolicy:              kubernetesAdvanced.ImagePullPolicy,
		ContainerCommand:             kubernetesAdvanced.ContainerCommand,
		ContainerArgs:                kubernetesAdvanced.ContainerArgs,
		Lifecycle:                    kubernetesAdvanced.Lifecycle,
		InitContainers:               kubernetesAdvanced.InitContainers,
		SidecarContainers:            kubernetesAdvanced.SidecarContainers,
		ReadinessProbe:               kubernetesAdvanced.ReadinessProbe,
		LivenessProbe:                kubernetesAdvanced.LivenessProbe,
		StartupProbe:                 kubernetesAdvanced.StartupProbe,
		RunAsUser:                    kubernetesAdvanced.RunAsUser,
		RunAsGroup:                   kubernetesAdvanced.RunAsGroup,
		FSGroup:                      kubernetesAdvanced.FSGroup,
		FSGroupChangePolicy:          kubernetesAdvanced.FSGroupChangePolicy,
		ReadOnlyRootFilesystem:       kubernetesAdvanced.ReadOnlyRootFilesystem,
		AllowPrivilegeEscalation:     kubernetesAdvanced.AllowPrivilegeEscalation,
		CapabilityAdd:                kubernetesAdvanced.CapabilityAdd,
		CapabilityDrop:               kubernetesAdvanced.CapabilityDrop,
		NodeSelector:                 kubernetesAdvanced.NodeSelector,
		Tolerations:                  kubernetesAdvanced.Tolerations,
		Affinity:                     kubernetesAdvanced.Affinity,
		TopologySpreadConstraints:    kubernetesAdvanced.TopologySpreadConstraints,
		PriorityClassName:            kubernetesAdvanced.PriorityClassName,
		ServiceAccountName:           kubernetesAdvanced.ServiceAccountName,
		AutomountServiceAccountToken: kubernetesAdvanced.AutomountServiceAccountToken,
		ServiceType:                  kubernetesAdvanced.ServiceType,
		ServiceAnnotations:           kubernetesAdvanced.ServiceAnnotations,
		ServiceExternalTrafficPolicy: kubernetesAdvanced.ServiceExternalTrafficPolicy,
		ServiceSessionAffinity:       kubernetesAdvanced.ServiceSessionAffinity,
		AutoScalingEnabled:           autoScaling.Enabled,
		AutoScalingMinReplicas:       autoScaling.MinReplicas,
		AutoScalingMaxReplicas:       autoScaling.MaxReplicas,
		AutoScalingCPUPercent:        autoScaling.CPUPercent,
		AutoScalingMemoryPercent:     autoScaling.MemoryPercent,
		AutoScalingBehavior:          autoScaling.Behavior,
		ServicePorts:                 model.EncodeDeploymentServicePorts(servicePorts, servicePorts[0].Port),
		SourceType:                   sourceType,
		RepositoryBindingID:          repositoryBindingID,
		BuildDefinitionMode:          buildDefinitionMode,
		BuildTemplateID:              buildTemplateID,
		BuildTemplateVersion:         buildTemplateVersion,
		BuildTemplateValues:          buildTemplateValues,
		DockerfilePath:               fallback(strings.TrimSpace(input.DockerfilePath), "Dockerfile"),
		BuildContext:                 fallback(strings.TrimSpace(input.BuildContext), "."),
		BuildDirectory:               strings.TrimSpace(input.BuildDirectory),
		BuildArgs:                    buildArgs,
		BuildEnvironmentID:           strings.TrimSpace(input.BuildEnvironmentID),
		BuildCPURequest:              buildCPURequest,
		BuildMemoryRequest:           buildMemoryRequest,
		BuildTimeoutSeconds:          buildTimeoutSeconds,
		TargetRegistryID:             targetRegistryID,
		TargetRepository:             targetRepository,
		TargetTag:                    fallback(targetTag, "latest"),
		ImageRef:                     strings.TrimSpace(input.ImageRef),
		BuildLabels:                  strings.Join(normalizeBuildSelectorList(strings.Split(input.BuildLabels, ",")), ","),
		BuildVariableSetIDs:          encodeBuildVariableSetIDs(input.BuildVariableSetIDs),
		BuildHooksEnabled:            buildHooksEnabled,
		AutoDeploy:                   input.AutoDeploy,
		BranchPattern:                strings.TrimSpace(input.BranchPattern),
		TagPattern:                   strings.TrimSpace(input.TagPattern),
		ConcurrencyPolicy:            normalizeBuildConcurrencyPolicy(input.ConcurrencyPolicy),
		RuntimeConfigRefs:            model.EncodeDeploymentRuntimeConfigRefs(runtimeConfigRefs),
		EnvVars:                      envVars,
		ConfigFiles:                  configFiles,
		SecretFiles:                  string(secretFilesContent),
		RequireApproval:              input.RequireApproval,
		WebConsoleEnabled:            normalizeWebConsoleOverride(input.WebConsoleEnabled),
		Enabled:                      input.Enabled,
		CreatedBy:                    user.ID,
	}, dataVolumes, true
}

func (h *Handlers) projectNamespaceForDeploymentTarget(ctx *gin.Context, projectID string) (string, bool) {
	var project model.Project
	if err := h.dbFor(ctx).Select("id", "kubernetes_namespace").First(&project, "id = ?", strings.TrimSpace(projectID)).Error; err != nil {
		writeError(ctx, http.StatusNotFound, "project not found")
		return "", false
	}
	namespace := runtimeProjectNamespace(project)
	if namespace == "" {
		writeError(ctx, http.StatusBadRequest, "项目空间缺少 Kubernetes Namespace")
		return "", false
	}
	return namespace, true
}
