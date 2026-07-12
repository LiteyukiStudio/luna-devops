package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
)

func (h *Handlers) deploymentTargetFromInput(ctx *gin.Context, user model.User, app model.Application, input deploymentTargetInput, targetID string, existingSecretFiles map[string]string, existingRuntimeConfigRefs string) (model.DeploymentTarget, bool) {
	sourceType := normalizeDeploymentSourceType(input.SourceType)
	repositoryBindingID := strings.TrimSpace(input.RepositoryBindingID)
	if sourceType == "repository" {
		if repositoryBindingID == "" {
			writeError(ctx, http.StatusBadRequest, "代码仓库不能为空")
			return model.DeploymentTarget{}, false
		}
		var binding model.RepositoryBinding
		if err := h.db.First(&binding, "id = ? and project_id = ? and application_id = ?", repositoryBindingID, app.ProjectID, app.ID).Error; err != nil {
			writeError(ctx, http.StatusBadRequest, "代码仓库绑定不存在")
			return model.DeploymentTarget{}, false
		}
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
	dataCapacity, ok := normalizeDataCapacity(ctx, input.DataCapacity, input.DataRetentionEnabled)
	if !ok {
		return model.DeploymentTarget{}, false
	}
	dataMountPath, ok := normalizeDataMountPath(ctx, input.DataMountPath, input.DataRetentionEnabled)
	if !ok {
		return model.DeploymentTarget{}, false
	}
	dataVolumes, ok := normalizeDataVolumes(ctx, input.DataVolumes, input.DataRetentionEnabled, dataMountPath, dataCapacity)
	if !ok {
		return model.DeploymentTarget{}, false
	}
	if len(dataVolumes) > 0 {
		dataMountPath = dataVolumes[0].MountPath
		dataCapacity = dataVolumes[0].Capacity
	}
	servicePorts, ok := normalizeDeploymentServicePorts(ctx, input.ServicePorts, input.ServicePort)
	if !ok {
		return model.DeploymentTarget{}, false
	}
	servicePort := servicePorts[0].Port
	replicas := input.Replicas
	if replicas <= 0 {
		replicas = 1
	}
	runtimeCPURequest, ok := normalizeBuildResourceQuantity(ctx, input.CPURequest, "1", "运行 CPU")
	if !ok {
		return model.DeploymentTarget{}, false
	}
	runtimeMemoryRequest, ok := normalizeBuildResourceQuantity(ctx, input.MemoryRequest, "1Gi", "运行内存")
	if !ok {
		return model.DeploymentTarget{}, false
	}
	runtimeCPULimit, ok := normalizeOptionalResourceQuantity(ctx, input.CPULimit, "运行 CPU 上限")
	if !ok {
		return model.DeploymentTarget{}, false
	}
	runtimeMemoryLimit, ok := normalizeOptionalResourceQuantity(ctx, input.MemoryLimit, "运行内存上限")
	if !ok {
		return model.DeploymentTarget{}, false
	}
	kubernetesAdvanced, ok := normalizeDeploymentKubernetesAdvanced(ctx, input)
	if !ok {
		return model.DeploymentTarget{}, false
	}
	autoScaling, ok := normalizeDeploymentAutoScaling(ctx, input, replicas)
	if !ok {
		return model.DeploymentTarget{}, false
	}
	buildCPURequest, ok := normalizeBuildResourceQuantity(ctx, input.BuildCPURequest, defaultBuildCPURequest, "构建 CPU")
	if !ok {
		return model.DeploymentTarget{}, false
	}
	buildMemoryRequest, ok := normalizeBuildResourceQuantity(ctx, input.BuildMemoryRequest, defaultBuildMemoryRequest, "构建内存")
	if !ok {
		return model.DeploymentTarget{}, false
	}
	buildTimeoutSeconds, ok := normalizeBuildTimeoutSeconds(ctx, input.BuildTimeoutSeconds)
	if !ok {
		return model.DeploymentTarget{}, false
	}
	buildArgs, ok := normalizeBuildArgsInput(ctx, input.BuildArgs)
	if !ok {
		return model.DeploymentTarget{}, false
	}
	clusterID := strings.TrimSpace(input.ClusterID)
	targetRegistryID := strings.TrimSpace(input.TargetRegistryID)
	if _, ok := h.runtimeClusterForProjectUse(ctx, user, app.ProjectID, clusterID); !ok {
		return model.DeploymentTarget{}, false
	}
	targetRepository, targetTag, ok = h.applyRegistryCredentialImageTemplate(ctx, user, app, sourceType, targetRegistryID, targetRepository, targetTag, model.DeploymentTarget{
		ID:    targetID,
		Name:  name,
		Stage: stage,
	})
	if !ok {
		return model.DeploymentTarget{}, false
	}
	runtimeConfigRefs, ok := h.runtimeConfigRefsFromInput(ctx, app.ProjectID, input, existingRuntimeConfigRefs)
	if !ok {
		return model.DeploymentTarget{}, false
	}
	runtimeConfigSetIDs := model.DeploymentRuntimeConfigLiveSetIDs(runtimeConfigRefs)
	configFiles, ok := normalizeRuntimeConfigFilesInput(ctx, input.ConfigFiles)
	if !ok {
		return model.DeploymentTarget{}, false
	}
	secretFiles, ok := h.runtimeSecretFilesFromInput(ctx, user, targetID, input.SecretFiles, existingSecretFiles)
	if !ok {
		return model.DeploymentTarget{}, false
	}
	secretFilesContent, err := json.Marshal(secretFiles)
	if err != nil {
		writeError(ctx, http.StatusInternalServerError, err.Error())
		return model.DeploymentTarget{}, false
	}
	for _, volume := range dataVolumes {
		if runtimeDataPathConflicts(volume.MountPath, configFiles, string(secretFilesContent)) {
			writeError(ctx, http.StatusBadRequest, "运行数据目录不能与配置文件或密钥文件挂载路径重叠")
			return model.DeploymentTarget{}, false
		}
	}
	return model.DeploymentTarget{
		ID:                           targetID,
		ProjectID:                    app.ProjectID,
		ApplicationID:                app.ID,
		EnvironmentID:                strings.TrimSpace(input.EnvironmentID),
		Name:                         name,
		Stage:                        stage,
		ClusterID:                    clusterID,
		Namespace:                    strings.TrimSpace(input.Namespace),
		WorkloadType:                 normalizeWorkloadType(input.WorkloadType),
		Replicas:                     replicas,
		CPURequest:                   runtimeCPURequest,
		MemoryRequest:                runtimeMemoryRequest,
		CPULimit:                     runtimeCPULimit,
		MemoryLimit:                  runtimeMemoryLimit,
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
		ServicePort:                  servicePort,
		ServicePorts:                 model.EncodeDeploymentServicePorts(servicePorts, servicePort),
		SourceType:                   sourceType,
		RepositoryBindingID:          repositoryBindingID,
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
		RuntimeConfigSetIDs:          encodeBuildVariableSetIDs(runtimeConfigSetIDs),
		RuntimeConfigRefs:            model.EncodeDeploymentRuntimeConfigRefs(runtimeConfigRefs),
		EnvVars:                      strings.TrimSpace(input.EnvVars),
		ConfigRefs:                   strings.TrimSpace(input.ConfigRefs),
		SecretRefs:                   normalizeSecretRefsInput(input.SecretRefs),
		ConfigFiles:                  configFiles,
		SecretFiles:                  string(secretFilesContent),
		DataRetentionEnabled:         input.DataRetentionEnabled,
		DataCapacity:                 dataCapacity,
		DataMountPath:                dataMountPath,
		DataVolumes:                  encodeDataVolumes(dataVolumes),
		DataStorageClassName:         kubernetesAdvanced.DataStorageClassName,
		DataAccessMode:               kubernetesAdvanced.DataAccessMode,
		DataVolumeMode:               kubernetesAdvanced.DataVolumeMode,
		RequireApproval:              input.RequireApproval,
		WebConsoleEnabled:            normalizeWebConsoleOverride(input.WebConsoleEnabled),
		Enabled:                      input.Enabled,
		CreatedBy:                    user.ID,
	}, true
}
