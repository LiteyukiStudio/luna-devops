package api

import (
	"sort"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/buildtemplate"
	"github.com/LiteyukiStudio/devops/internal/model"
)

type deploymentTargetResponse struct {
	ID                           string                               `json:"id"`
	ProjectID                    string                               `json:"projectId"`
	ApplicationID                string                               `json:"applicationId"`
	EnvironmentID                string                               `json:"environmentId"`
	Name                         string                               `json:"name"`
	Stage                        string                               `json:"stage"`
	ClusterID                    string                               `json:"clusterId"`
	Namespace                    string                               `json:"namespace"`
	WorkloadType                 string                               `json:"workloadType"`
	Replicas                     int                                  `json:"replicas"`
	CPURequest                   string                               `json:"cpuRequest"`
	MemoryRequest                string                               `json:"memoryRequest"`
	CPULimit                     string                               `json:"cpuLimit"`
	MemoryLimit                  string                               `json:"memoryLimit"`
	ImagePullPolicy              string                               `json:"imagePullPolicy"`
	ContainerCommand             string                               `json:"containerCommand"`
	ContainerArgs                string                               `json:"containerArgs"`
	Lifecycle                    string                               `json:"lifecycle"`
	InitContainers               string                               `json:"initContainers"`
	SidecarContainers            string                               `json:"sidecarContainers"`
	ReadinessProbe               string                               `json:"readinessProbe"`
	LivenessProbe                string                               `json:"livenessProbe"`
	StartupProbe                 string                               `json:"startupProbe"`
	RunAsUser                    string                               `json:"runAsUser"`
	RunAsGroup                   string                               `json:"runAsGroup"`
	FSGroup                      string                               `json:"fsGroup"`
	FSGroupChangePolicy          string                               `json:"fsGroupChangePolicy"`
	ReadOnlyRootFilesystem       bool                                 `json:"readOnlyRootFilesystem"`
	AllowPrivilegeEscalation     string                               `json:"allowPrivilegeEscalation"`
	CapabilityAdd                string                               `json:"capabilityAdd"`
	CapabilityDrop               string                               `json:"capabilityDrop"`
	NodeSelector                 string                               `json:"nodeSelector"`
	Tolerations                  string                               `json:"tolerations"`
	Affinity                     string                               `json:"affinity"`
	TopologySpreadConstraints    string                               `json:"topologySpreadConstraints"`
	PriorityClassName            string                               `json:"priorityClassName"`
	ServiceType                  string                               `json:"serviceType"`
	ServiceAnnotations           string                               `json:"serviceAnnotations"`
	ServiceExternalTrafficPolicy string                               `json:"serviceExternalTrafficPolicy"`
	ServiceSessionAffinity       string                               `json:"serviceSessionAffinity"`
	AutoScalingEnabled           bool                                 `json:"autoScalingEnabled"`
	AutoScalingMinReplicas       int                                  `json:"autoScalingMinReplicas"`
	AutoScalingMaxReplicas       int                                  `json:"autoScalingMaxReplicas"`
	AutoScalingCPUPercent        int                                  `json:"autoScalingCpuPercent"`
	AutoScalingMemoryPercent     int                                  `json:"autoScalingMemoryPercent"`
	AutoScalingBehavior          string                               `json:"autoScalingBehavior"`
	ServicePort                  int                                  `json:"servicePort"`
	ServicePorts                 []model.DeploymentServicePort        `json:"servicePorts"`
	SourceType                   string                               `json:"sourceType"`
	RepositoryBindingID          string                               `json:"repositoryBindingId"`
	BuildDefinitionMode          string                               `json:"buildDefinitionMode"`
	BuildTemplateID              string                               `json:"buildTemplateId"`
	BuildTemplateVersion         string                               `json:"buildTemplateVersion"`
	BuildTemplateValues          string                               `json:"buildTemplateValues"`
	DockerfilePath               string                               `json:"dockerfilePath"`
	BuildContext                 string                               `json:"buildContext"`
	BuildDirectory               string                               `json:"buildDirectory"`
	BuildArgs                    string                               `json:"buildArgs"`
	BuildEnvironmentID           string                               `json:"buildEnvironmentId"`
	BuildCPURequest              string                               `json:"buildCpuRequest"`
	BuildMemoryRequest           string                               `json:"buildMemoryRequest"`
	BuildTimeoutSeconds          int                                  `json:"buildTimeoutSeconds"`
	TargetRegistryID             string                               `json:"targetRegistryId"`
	TargetRepository             string                               `json:"targetRepository"`
	TargetTag                    string                               `json:"targetTag"`
	ImageRef                     string                               `json:"imageRef"`
	BuildLabels                  string                               `json:"buildLabels"`
	BuildVariableSetIDs          string                               `json:"buildVariableSetIds"`
	BuildHooksEnabled            bool                                 `json:"buildHooksEnabled"`
	BuildHookBindings            []model.DeploymentTargetHookBinding  `json:"buildHookBindings"`
	AutoDeploy                   bool                                 `json:"autoDeploy"`
	BranchPattern                string                               `json:"branchPattern"`
	TagPattern                   string                               `json:"tagPattern"`
	ConcurrencyPolicy            string                               `json:"concurrencyPolicy"`
	RuntimeConfigSetIDs          string                               `json:"runtimeConfigSetIds"`
	RuntimeConfigRefs            []deploymentRuntimeConfigRefResponse `json:"runtimeConfigRefs"`
	EnvVars                      string                               `json:"envVars"`
	ConfigRefs                   string                               `json:"configRefs"`
	SecretRefsSet                bool                                 `json:"secretRefsSet"`
	ConfigFiles                  string                               `json:"configFiles"`
	SecretFilesSet               bool                                 `json:"secretFilesSet"`
	DataRetentionEnabled         bool                                 `json:"dataRetentionEnabled"`
	DataCapacity                 string                               `json:"dataCapacity"`
	DataMountPath                string                               `json:"dataMountPath"`
	DataVolumes                  string                               `json:"dataVolumes"`
	DataStorageClassName         string                               `json:"dataStorageClassName"`
	DataAccessMode               string                               `json:"dataAccessMode"`
	DataVolumeMode               string                               `json:"dataVolumeMode"`
	RequireApproval              bool                                 `json:"requireApproval"`
	WebConsoleEnabled            *bool                                `json:"webConsoleEnabled"`
	Enabled                      bool                                 `json:"enabled"`
	DeleteStatus                 string                               `json:"deleteStatus"`
	DeleteMessage                string                               `json:"deleteMessage"`
	DeleteStartedAt              *time.Time                           `json:"deleteStartedAt"`
	DeleteFinishedAt             *time.Time                           `json:"deleteFinishedAt"`
	CreatedBy                    string                               `json:"createdBy"`
	CreatedAt                    time.Time                            `json:"createdAt"`
}

func deploymentTargetResponses(targets []model.DeploymentTarget) []deploymentTargetResponse {
	responses := make([]deploymentTargetResponse, 0, len(targets))
	for _, target := range targets {
		responses = append(responses, deploymentTargetResponseFromModel(target))
	}
	return responses
}

func deploymentTargetResponseFromModel(target model.DeploymentTarget) deploymentTargetResponse {
	return deploymentTargetResponse{
		ID:                           target.ID,
		ProjectID:                    target.ProjectID,
		ApplicationID:                target.ApplicationID,
		EnvironmentID:                target.EnvironmentID,
		Name:                         target.Name,
		Stage:                        normalizeStage(target.Stage),
		ClusterID:                    target.ClusterID,
		Namespace:                    target.Namespace,
		WorkloadType:                 normalizeWorkloadType(target.WorkloadType),
		Replicas:                     fallbackInt(target.Replicas, 1),
		CPURequest:                   fallback(strings.TrimSpace(target.CPURequest), "1"),
		MemoryRequest:                fallback(strings.TrimSpace(target.MemoryRequest), "1Gi"),
		CPULimit:                     strings.TrimSpace(target.CPULimit),
		MemoryLimit:                  strings.TrimSpace(target.MemoryLimit),
		ImagePullPolicy:              normalizeImagePullPolicyValue(target.ImagePullPolicy),
		ContainerCommand:             target.ContainerCommand,
		ContainerArgs:                target.ContainerArgs,
		Lifecycle:                    target.Lifecycle,
		InitContainers:               target.InitContainers,
		SidecarContainers:            target.SidecarContainers,
		ReadinessProbe:               target.ReadinessProbe,
		LivenessProbe:                target.LivenessProbe,
		StartupProbe:                 target.StartupProbe,
		RunAsUser:                    strings.TrimSpace(target.RunAsUser),
		RunAsGroup:                   strings.TrimSpace(target.RunAsGroup),
		FSGroup:                      strings.TrimSpace(target.FSGroup),
		FSGroupChangePolicy:          normalizeFSGroupChangePolicy(target.FSGroupChangePolicy),
		ReadOnlyRootFilesystem:       target.ReadOnlyRootFilesystem,
		AllowPrivilegeEscalation:     normalizeTriStateBool(target.AllowPrivilegeEscalation),
		CapabilityAdd:                target.CapabilityAdd,
		CapabilityDrop:               target.CapabilityDrop,
		NodeSelector:                 target.NodeSelector,
		Tolerations:                  target.Tolerations,
		Affinity:                     target.Affinity,
		TopologySpreadConstraints:    target.TopologySpreadConstraints,
		PriorityClassName:            strings.TrimSpace(target.PriorityClassName),
		ServiceType:                  normalizeServiceType(target.ServiceType),
		ServiceAnnotations:           target.ServiceAnnotations,
		ServiceExternalTrafficPolicy: normalizeServiceExternalTrafficPolicy(target.ServiceExternalTrafficPolicy),
		ServiceSessionAffinity:       normalizeServiceSessionAffinity(target.ServiceSessionAffinity),
		AutoScalingEnabled:           target.AutoScalingEnabled,
		AutoScalingMinReplicas:       fallbackInt(target.AutoScalingMinReplicas, 1),
		AutoScalingMaxReplicas:       fallbackInt(target.AutoScalingMaxReplicas, fallbackInt(target.Replicas, 1)),
		AutoScalingCPUPercent:        target.AutoScalingCPUPercent,
		AutoScalingMemoryPercent:     target.AutoScalingMemoryPercent,
		AutoScalingBehavior:          target.AutoScalingBehavior,
		ServicePort:                  fallbackInt(target.ServicePort, 8080),
		ServicePorts:                 model.DeploymentTargetServicePorts(target),
		SourceType:                   normalizeDeploymentSourceType(target.SourceType),
		RepositoryBindingID:          target.RepositoryBindingID,
		BuildDefinitionMode:          fallback(strings.TrimSpace(target.BuildDefinitionMode), buildtemplate.DefinitionModeRepository),
		BuildTemplateID:              target.BuildTemplateID,
		BuildTemplateVersion:         target.BuildTemplateVersion,
		BuildTemplateValues:          fallback(strings.TrimSpace(target.BuildTemplateValues), "{}"),
		DockerfilePath:               target.DockerfilePath,
		BuildContext:                 target.BuildContext,
		BuildDirectory:               target.BuildDirectory,
		BuildArgs:                    buildArgsResponseText(target.BuildArgs),
		BuildEnvironmentID:           strings.TrimSpace(target.BuildEnvironmentID),
		BuildCPURequest:              fallback(strings.TrimSpace(target.BuildCPURequest), defaultBuildCPURequest),
		BuildMemoryRequest:           fallback(strings.TrimSpace(target.BuildMemoryRequest), defaultBuildMemoryRequest),
		BuildTimeoutSeconds:          normalizeBuildTimeoutSecondsValue(target.BuildTimeoutSeconds),
		TargetRegistryID:             target.TargetRegistryID,
		TargetRepository:             target.TargetRepository,
		TargetTag:                    target.TargetTag,
		ImageRef:                     target.ImageRef,
		BuildLabels:                  target.BuildLabels,
		BuildVariableSetIDs:          target.BuildVariableSetIDs,
		BuildHooksEnabled:            target.BuildHooksEnabled,
		BuildHookBindings:            target.BuildHookBindings,
		AutoDeploy:                   target.AutoDeploy,
		BranchPattern:                target.BranchPattern,
		TagPattern:                   target.TagPattern,
		ConcurrencyPolicy:            target.ConcurrencyPolicy,
		RuntimeConfigSetIDs:          target.RuntimeConfigSetIDs,
		RuntimeConfigRefs:            deploymentRuntimeConfigRefsResponse(target),
		EnvVars:                      target.EnvVars,
		ConfigRefs:                   target.ConfigRefs,
		SecretRefsSet:                strings.TrimSpace(target.SecretRefs) != "",
		ConfigFiles:                  target.ConfigFiles,
		SecretFilesSet:               strings.TrimSpace(target.SecretFiles) != "" && strings.TrimSpace(target.SecretFiles) != "{}",
		DataRetentionEnabled:         target.DataRetentionEnabled,
		DataCapacity:                 target.DataCapacity,
		DataMountPath:                deploymentTargetDataMountPath(target),
		DataVolumes:                  encodeDataVolumes(deploymentTargetDataVolumes(target)),
		DataStorageClassName:         strings.TrimSpace(target.DataStorageClassName),
		DataAccessMode:               normalizePersistentVolumeAccessMode(target.DataAccessMode),
		DataVolumeMode:               normalizePersistentVolumeMode(target.DataVolumeMode),
		RequireApproval:              target.RequireApproval,
		WebConsoleEnabled:            target.WebConsoleEnabled,
		Enabled:                      target.Enabled,
		DeleteStatus:                 target.DeleteStatus,
		DeleteMessage:                target.DeleteMessage,
		DeleteStartedAt:              target.DeleteStartedAt,
		DeleteFinishedAt:             target.DeleteFinishedAt,
		CreatedBy:                    target.CreatedBy,
		CreatedAt:                    target.CreatedAt,
	}
}

func buildArgsResponseText(raw string) string {
	values := model.BuildArgs(raw)
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, key+"="+values[key])
	}
	return strings.Join(lines, "\n")
}

func deploymentTargetEnvironmentProfile(target model.DeploymentTarget) model.Environment {
	environmentID := strings.TrimSpace(target.EnvironmentID)
	if environmentID == "" {
		environmentID = target.ID
	}
	replicas := target.Replicas
	if replicas <= 0 {
		replicas = 1
	}
	return model.Environment{
		ID:            environmentID,
		ProjectID:     target.ProjectID,
		Name:          firstNonEmpty(strings.TrimSpace(target.Name), strings.TrimSpace(target.Stage), target.ID),
		Slug:          firstNonEmpty(strings.TrimSpace(target.Stage), strings.TrimSpace(target.Name), "prod"),
		ClusterID:     strings.TrimSpace(target.ClusterID),
		Namespace:     strings.TrimSpace(target.Namespace),
		Replicas:      replicas,
		CPURequest:    fallback(strings.TrimSpace(target.CPURequest), "1"),
		MemoryRequest: fallback(strings.TrimSpace(target.MemoryRequest), "1Gi"),
	}
}

type deploymentTargetInput struct {
	Name                         string                             `json:"name"`
	EnvironmentID                string                             `json:"environmentId"`
	Stage                        string                             `json:"stage"`
	ClusterID                    string                             `json:"clusterId"`
	Namespace                    string                             `json:"namespace"`
	WorkloadType                 string                             `json:"workloadType"`
	Replicas                     int                                `json:"replicas"`
	CPURequest                   string                             `json:"cpuRequest"`
	MemoryRequest                string                             `json:"memoryRequest"`
	CPULimit                     string                             `json:"cpuLimit"`
	MemoryLimit                  string                             `json:"memoryLimit"`
	ImagePullPolicy              string                             `json:"imagePullPolicy"`
	ContainerCommand             string                             `json:"containerCommand"`
	ContainerArgs                string                             `json:"containerArgs"`
	Lifecycle                    string                             `json:"lifecycle"`
	InitContainers               string                             `json:"initContainers"`
	SidecarContainers            string                             `json:"sidecarContainers"`
	ReadinessProbe               string                             `json:"readinessProbe"`
	LivenessProbe                string                             `json:"livenessProbe"`
	StartupProbe                 string                             `json:"startupProbe"`
	RunAsUser                    string                             `json:"runAsUser"`
	RunAsGroup                   string                             `json:"runAsGroup"`
	FSGroup                      string                             `json:"fsGroup"`
	FSGroupChangePolicy          string                             `json:"fsGroupChangePolicy"`
	ReadOnlyRootFilesystem       bool                               `json:"readOnlyRootFilesystem"`
	AllowPrivilegeEscalation     string                             `json:"allowPrivilegeEscalation"`
	CapabilityAdd                string                             `json:"capabilityAdd"`
	CapabilityDrop               string                             `json:"capabilityDrop"`
	NodeSelector                 string                             `json:"nodeSelector"`
	Tolerations                  string                             `json:"tolerations"`
	Affinity                     string                             `json:"affinity"`
	TopologySpreadConstraints    string                             `json:"topologySpreadConstraints"`
	PriorityClassName            string                             `json:"priorityClassName"`
	ServiceType                  string                             `json:"serviceType"`
	ServiceAnnotations           string                             `json:"serviceAnnotations"`
	ServiceExternalTrafficPolicy string                             `json:"serviceExternalTrafficPolicy"`
	ServiceSessionAffinity       string                             `json:"serviceSessionAffinity"`
	AutoScalingEnabled           bool                               `json:"autoScalingEnabled"`
	AutoScalingMinReplicas       int                                `json:"autoScalingMinReplicas"`
	AutoScalingMaxReplicas       int                                `json:"autoScalingMaxReplicas"`
	AutoScalingCPUPercent        int                                `json:"autoScalingCpuPercent"`
	AutoScalingMemoryPercent     int                                `json:"autoScalingMemoryPercent"`
	AutoScalingBehavior          string                             `json:"autoScalingBehavior"`
	ServicePort                  int                                `json:"servicePort"`
	ServicePorts                 []model.DeploymentServicePort      `json:"servicePorts"`
	SourceType                   string                             `json:"sourceType"`
	RepositoryBindingID          string                             `json:"repositoryBindingId"`
	BuildDefinitionMode          string                             `json:"buildDefinitionMode"`
	BuildTemplateID              string                             `json:"buildTemplateId"`
	BuildTemplateVersion         string                             `json:"buildTemplateVersion"`
	BuildTemplateValues          string                             `json:"buildTemplateValues"`
	DockerfilePath               string                             `json:"dockerfilePath"`
	BuildContext                 string                             `json:"buildContext"`
	BuildDirectory               string                             `json:"buildDirectory"`
	BuildArgs                    string                             `json:"buildArgs"`
	BuildEnvironmentID           string                             `json:"buildEnvironmentId"`
	BuildCPURequest              string                             `json:"buildCpuRequest"`
	BuildMemoryRequest           string                             `json:"buildMemoryRequest"`
	BuildTimeoutSeconds          int                                `json:"buildTimeoutSeconds"`
	TargetRegistryID             string                             `json:"targetRegistryId"`
	TargetImageRef               string                             `json:"targetImageRef"`
	TargetRepository             string                             `json:"targetRepository"`
	TargetTag                    string                             `json:"targetTag"`
	ImageRef                     string                             `json:"imageRef"`
	BuildLabels                  string                             `json:"buildLabels"`
	BuildVariableSetIDs          []string                           `json:"buildVariableSetIds"`
	BuildVariables               *map[string]string                 `json:"buildVariables"`
	BuildSecrets                 *map[string]string                 `json:"buildSecrets"`
	BuildHooksEnabled            *bool                              `json:"buildHooksEnabled"`
	BuildHookBindings            []deploymentTargetHookBindingInput `json:"buildHookBindings"`
	AutoDeploy                   bool                               `json:"autoDeploy"`
	BranchPattern                string                             `json:"branchPattern"`
	TagPattern                   string                             `json:"tagPattern"`
	ConcurrencyPolicy            string                             `json:"concurrencyPolicy"`
	RuntimeConfigSetIDs          []string                           `json:"runtimeConfigSetIds"`
	RuntimeConfigRefs            []deploymentRuntimeConfigRefInput  `json:"runtimeConfigRefs"`
	EnvVars                      string                             `json:"envVars"`
	ConfigRefs                   string                             `json:"configRefs"`
	SecretRefs                   string                             `json:"secretRefs"`
	ConfigFiles                  string                             `json:"configFiles"`
	SecretFiles                  string                             `json:"secretFiles"`
	DataRetentionEnabled         bool                               `json:"dataRetentionEnabled"`
	DataCapacity                 string                             `json:"dataCapacity"`
	DataMountPath                string                             `json:"dataMountPath"`
	DataVolumes                  string                             `json:"dataVolumes"`
	DataStorageClassName         string                             `json:"dataStorageClassName"`
	DataAccessMode               string                             `json:"dataAccessMode"`
	DataVolumeMode               string                             `json:"dataVolumeMode"`
	RequireApproval              bool                               `json:"requireApproval"`
	WebConsoleEnabled            *bool                              `json:"webConsoleEnabled"`
	Enabled                      bool                               `json:"enabled"`
}

type deploymentTargetDataVolumeInput struct {
	Name              string `json:"name"`
	MountPath         string `json:"mountPath"`
	Capacity          string `json:"capacity"`
	SourceType        string `json:"sourceType"`
	ExistingClaimName string `json:"existingClaimName"`
	EmptyDirMedium    string `json:"emptyDirMedium"`
	EmptyDirSizeLimit string `json:"emptyDirSizeLimit"`
}

type deploymentTargetHookBindingInput struct {
	HookConfigID string `json:"hookConfigId"`
	Phase        string `json:"phase"`
	RunOrder     int    `json:"runOrder"`
}

type deploymentRuntimeConfigRefInput struct {
	SetID string `json:"setId"`
	Mode  string `json:"mode"`
}

type deploymentRuntimeConfigRefResponse struct {
	SetID string `json:"setId"`
	Mode  string `json:"mode"`
}

func deploymentRuntimeConfigRefsResponse(target model.DeploymentTarget) []deploymentRuntimeConfigRefResponse {
	refs := model.DecodeDeploymentRuntimeConfigRefs(target.RuntimeConfigRefs)
	if len(refs) == 0 {
		for _, setID := range buildVariableSetIDs(target.RuntimeConfigSetIDs) {
			refs = append(refs, model.DeploymentRuntimeConfigRef{SetID: setID, Mode: model.RuntimeConfigRefModeLive})
		}
	}
	output := make([]deploymentRuntimeConfigRefResponse, 0, len(refs))
	for _, ref := range refs {
		output = append(output, deploymentRuntimeConfigRefResponse{
			SetID: ref.SetID,
			Mode:  model.RuntimeConfigRefMode(ref.Mode),
		})
	}
	return output
}
