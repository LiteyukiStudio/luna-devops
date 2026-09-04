package deploymentapi

import (
	"sort"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/buildtemplate"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/runtimeconfig"
)

type deploymentTargetResponse struct {
	ID                        string                               `json:"id"`
	ProjectID                 string                               `json:"projectId"`
	ApplicationID             string                               `json:"applicationId"`
	Name                      string                               `json:"name"`
	Stage                     string                               `json:"stage"`
	KubernetesName            string                               `json:"kubernetesName"`
	ClusterID                 string                               `json:"clusterId"`
	WorkloadType              string                               `json:"workloadType"`
	Replicas                  int                                  `json:"replicas"`
	CPURequest                string                               `json:"cpuRequest"`
	MemoryRequest             string                               `json:"memoryRequest"`
	ImagePullPolicy           string                               `json:"imagePullPolicy"`
	ContainerCommand          string                               `json:"containerCommand"`
	ContainerArgs             string                               `json:"containerArgs"`
	Lifecycle                 string                               `json:"lifecycle"`
	InitContainers            string                               `json:"initContainers"`
	SidecarContainers         string                               `json:"sidecarContainers"`
	ReadinessProbe            string                               `json:"readinessProbe"`
	LivenessProbe             string                               `json:"livenessProbe"`
	StartupProbe              string                               `json:"startupProbe"`
	RunAsUser                 string                               `json:"runAsUser"`
	RunAsGroup                string                               `json:"runAsGroup"`
	FSGroup                   string                               `json:"fsGroup"`
	FSGroupChangePolicy       string                               `json:"fsGroupChangePolicy"`
	ReadOnlyRootFilesystem    bool                                 `json:"readOnlyRootFilesystem"`
	CapabilityDrop            string                               `json:"capabilityDrop"`
	NodeSelector              string                               `json:"nodeSelector"`
	Tolerations               string                               `json:"tolerations"`
	Affinity                  string                               `json:"affinity"`
	TopologySpreadConstraints string                               `json:"topologySpreadConstraints"`
	PriorityClassName         string                               `json:"priorityClassName"`
	ServiceAnnotations        string                               `json:"serviceAnnotations"`
	ServiceSessionAffinity    string                               `json:"serviceSessionAffinity"`
	AutoScalingEnabled        bool                                 `json:"autoScalingEnabled"`
	AutoScalingMinReplicas    int                                  `json:"autoScalingMinReplicas"`
	AutoScalingMaxReplicas    int                                  `json:"autoScalingMaxReplicas"`
	AutoScalingCPUPercent     int                                  `json:"autoScalingCpuPercent"`
	AutoScalingMemoryPercent  int                                  `json:"autoScalingMemoryPercent"`
	AutoScalingBehavior       string                               `json:"autoScalingBehavior"`
	ServicePorts              []model.DeploymentServicePort        `json:"servicePorts"`
	SourceType                string                               `json:"sourceType"`
	RepositoryBindingID       string                               `json:"repositoryBindingId"`
	BuildDefinitionMode       string                               `json:"buildDefinitionMode"`
	BuildTemplateID           string                               `json:"buildTemplateId"`
	BuildTemplateVersion      string                               `json:"buildTemplateVersion"`
	BuildTemplateValues       string                               `json:"buildTemplateValues"`
	DockerfilePath            string                               `json:"dockerfilePath"`
	BuildContext              string                               `json:"buildContext"`
	BuildDirectory            string                               `json:"buildDirectory"`
	BuildArgs                 string                               `json:"buildArgs"`
	BuildEnvironmentID        string                               `json:"buildEnvironmentId"`
	BuildCPURequest           string                               `json:"buildCpuRequest"`
	BuildMemoryRequest        string                               `json:"buildMemoryRequest"`
	BuildTimeoutSeconds       int                                  `json:"buildTimeoutSeconds"`
	TargetRegistryID          string                               `json:"targetRegistryId"`
	TargetRepository          string                               `json:"targetRepository"`
	TargetTag                 string                               `json:"targetTag"`
	ImageRef                  string                               `json:"imageRef"`
	BuildVariableSetIDs       []string                             `json:"buildVariableSetIds"`
	BuildHooksEnabled         bool                                 `json:"buildHooksEnabled"`
	BuildHookBindings         []model.DeploymentTargetHookBinding  `json:"buildHookBindings"`
	AutoDeploy                bool                                 `json:"autoDeploy"`
	BranchPattern             string                               `json:"branchPattern"`
	TagPattern                string                               `json:"tagPattern"`
	ConcurrencyPolicy         string                               `json:"concurrencyPolicy"`
	RuntimeConfigRefs         []deploymentRuntimeConfigRefResponse `json:"runtimeConfigRefs"`
	EnvironmentVariables      []runtimeEnvironmentVariableResponse `json:"environmentVariables"`
	ConfigFiles               string                               `json:"configFiles"`
	SecretFilesSet            bool                                 `json:"secretFilesSet"`
	DataVolumes               []deploymentTargetDataVolumeResponse `json:"dataVolumes"`
	RequireApproval           bool                                 `json:"requireApproval"`
	WebConsoleEnabled         *bool                                `json:"webConsoleEnabled"`
	Enabled                   bool                                 `json:"enabled"`
	Status                    string                               `json:"status"`
	ObservationCode           string                               `json:"observationCode,omitempty"`
	LastCheckedAt             *time.Time                           `json:"lastCheckedAt,omitempty"`
	DesiredReplicas           int32                                `json:"desiredReplicas"`
	UpdatedReplicas           int32                                `json:"updatedReplicas"`
	ReadyReplicas             int32                                `json:"readyReplicas"`
	AvailableReplicas         int32                                `json:"availableReplicas"`
	DeleteStatus              string                               `json:"deleteStatus"`
	DeleteMessage             string                               `json:"deleteMessage"`
	DeleteStartedAt           *time.Time                           `json:"deleteStartedAt"`
	DeleteFinishedAt          *time.Time                           `json:"deleteFinishedAt"`
	CreatedBy                 string                               `json:"createdBy"`
	CreatedAt                 time.Time                            `json:"createdAt"`
}

func deploymentTargetResponses(targets []model.DeploymentTarget, mountsByTarget map[string][]model.DeploymentVolumeMount) []deploymentTargetResponse {
	responses := make([]deploymentTargetResponse, 0, len(targets))
	for _, target := range targets {
		responses = append(responses, deploymentTargetResponseFromModel(target, mountsByTarget[target.ID]))
	}
	return responses
}

func deploymentTargetResponseFromModel(target model.DeploymentTarget, mounts ...[]model.DeploymentVolumeMount) deploymentTargetResponse {
	var dataVolumes []deploymentTargetDataVolumeResponse
	if len(mounts) > 0 {
		dataVolumes = deploymentTargetDataVolumeResponses(mounts[0])
	}
	if dataVolumes == nil {
		dataVolumes = []deploymentTargetDataVolumeResponse{}
	}
	servicePorts := model.DeploymentTargetServicePorts(target)
	return deploymentTargetResponse{
		ID:                        target.ID,
		ProjectID:                 target.ProjectID,
		ApplicationID:             target.ApplicationID,
		Name:                      target.Name,
		Stage:                     fallback(strings.TrimSpace(target.Stage), model.DefaultDeploymentStage),
		KubernetesName:            strings.TrimSpace(target.KubernetesName),
		ClusterID:                 target.ClusterID,
		WorkloadType:              normalizeWorkloadType(target.WorkloadType),
		Replicas:                  fallbackInt(target.Replicas, 1),
		CPURequest:                fallback(strings.TrimSpace(target.CPURequest), model.DefaultDeploymentCPURequest),
		MemoryRequest:             fallback(strings.TrimSpace(target.MemoryRequest), model.DefaultDeploymentMemoryRequest),
		ImagePullPolicy:           normalizeImagePullPolicyValue(target.ImagePullPolicy),
		ContainerCommand:          target.ContainerCommand,
		ContainerArgs:             target.ContainerArgs,
		Lifecycle:                 target.Lifecycle,
		InitContainers:            target.InitContainers,
		SidecarContainers:         target.SidecarContainers,
		ReadinessProbe:            target.ReadinessProbe,
		LivenessProbe:             target.LivenessProbe,
		StartupProbe:              target.StartupProbe,
		RunAsUser:                 strings.TrimSpace(target.RunAsUser),
		RunAsGroup:                strings.TrimSpace(target.RunAsGroup),
		FSGroup:                   strings.TrimSpace(target.FSGroup),
		FSGroupChangePolicy:       normalizeFSGroupChangePolicy(target.FSGroupChangePolicy),
		ReadOnlyRootFilesystem:    target.ReadOnlyRootFilesystem,
		CapabilityDrop:            target.CapabilityDrop,
		NodeSelector:              target.NodeSelector,
		Tolerations:               target.Tolerations,
		Affinity:                  target.Affinity,
		TopologySpreadConstraints: target.TopologySpreadConstraints,
		PriorityClassName:         strings.TrimSpace(target.PriorityClassName),
		ServiceAnnotations:        target.ServiceAnnotations,
		ServiceSessionAffinity:    normalizeServiceSessionAffinity(target.ServiceSessionAffinity),
		AutoScalingEnabled:        target.AutoScalingEnabled,
		AutoScalingMinReplicas:    fallbackInt(target.AutoScalingMinReplicas, 1),
		AutoScalingMaxReplicas:    fallbackInt(target.AutoScalingMaxReplicas, fallbackInt(target.Replicas, 1)),
		AutoScalingCPUPercent:     target.AutoScalingCPUPercent,
		AutoScalingMemoryPercent:  target.AutoScalingMemoryPercent,
		AutoScalingBehavior:       target.AutoScalingBehavior,
		ServicePorts:              servicePorts,
		SourceType:                normalizeDeploymentSourceType(target.SourceType),
		RepositoryBindingID:       target.RepositoryBindingID,
		BuildDefinitionMode:       fallback(strings.TrimSpace(target.BuildDefinitionMode), buildtemplate.DefinitionModeRepository),
		BuildTemplateID:           target.BuildTemplateID,
		BuildTemplateVersion:      target.BuildTemplateVersion,
		BuildTemplateValues:       fallback(strings.TrimSpace(target.BuildTemplateValues), "{}"),
		DockerfilePath:            target.DockerfilePath,
		BuildContext:              target.BuildContext,
		BuildDirectory:            target.BuildDirectory,
		BuildArgs:                 buildArgsResponseText(target.BuildArgs),
		BuildEnvironmentID:        strings.TrimSpace(target.BuildEnvironmentID),
		BuildCPURequest:           fallback(strings.TrimSpace(target.BuildCPURequest), defaultBuildCPURequest),
		BuildMemoryRequest:        fallback(strings.TrimSpace(target.BuildMemoryRequest), defaultBuildMemoryRequest),
		BuildTimeoutSeconds:       normalizeBuildTimeoutSecondsValue(target.BuildTimeoutSeconds),
		TargetRegistryID:          target.TargetRegistryID,
		TargetRepository:          target.TargetRepository,
		TargetTag:                 target.TargetTag,
		ImageRef:                  target.ImageRef,
		BuildVariableSetIDs:       buildVariableSetIDs(target.BuildVariableSetIDs),
		BuildHooksEnabled:         target.BuildHooksEnabled,
		BuildHookBindings:         target.BuildHookBindings,
		AutoDeploy:                target.AutoDeploy,
		BranchPattern:             target.BranchPattern,
		TagPattern:                target.TagPattern,
		ConcurrencyPolicy:         target.ConcurrencyPolicy,
		RuntimeConfigRefs:         deploymentRuntimeConfigRefsResponse(target),
		EnvironmentVariables:      runtimeEnvironmentVariables(target.EnvVars, target.SecretRefs),
		ConfigFiles:               target.ConfigFiles,
		SecretFilesSet:            strings.TrimSpace(target.SecretFiles) != "" && strings.TrimSpace(target.SecretFiles) != "{}",
		DataVolumes:               dataVolumes,
		RequireApproval:           target.RequireApproval,
		WebConsoleEnabled:         target.WebConsoleEnabled,
		Enabled:                   target.Enabled,
		Status:                    target.Status,
		ObservationCode:           target.ObservationCode,
		LastCheckedAt:             target.LastCheckedAt,
		DesiredReplicas:           target.DesiredReplicas,
		UpdatedReplicas:           target.UpdatedReplicas,
		ReadyReplicas:             target.ReadyReplicas,
		AvailableReplicas:         target.AvailableReplicas,
		DeleteStatus:              target.DeleteStatus,
		DeleteMessage:             target.DeleteMessage,
		DeleteStartedAt:           target.DeleteStartedAt,
		DeleteFinishedAt:          target.DeleteFinishedAt,
		CreatedBy:                 target.CreatedBy,
		CreatedAt:                 target.CreatedAt,
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

type deploymentTargetInput struct {
	Name                         string                             `json:"name"`
	Stage                        string                             `json:"stage"`
	ClusterID                    string                             `json:"clusterId"`
	Namespace                    string                             `json:"namespace"`
	WorkloadType                 string                             `json:"workloadType"`
	Replicas                     int                                `json:"replicas"`
	CPURequest                   string                             `json:"cpuRequest"`
	MemoryRequest                string                             `json:"memoryRequest"`
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
	ServiceAccountName           string                             `json:"serviceAccountName"`
	AutomountServiceAccountToken string                             `json:"automountServiceAccountToken"`
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
	BuildVariableSetIDs          []string                           `json:"buildVariableSetIds"`
	BuildVariables               *map[string]string                 `json:"buildVariables"`
	BuildSecrets                 *map[string]string                 `json:"buildSecrets"`
	BuildHooksEnabled            *bool                              `json:"buildHooksEnabled"`
	BuildHookBindings            []deploymentTargetHookBindingInput `json:"buildHookBindings"`
	AutoDeploy                   bool                               `json:"autoDeploy"`
	BranchPattern                string                             `json:"branchPattern"`
	TagPattern                   string                             `json:"tagPattern"`
	ConcurrencyPolicy            string                             `json:"concurrencyPolicy"`
	RuntimeConfigRefs            []deploymentRuntimeConfigRefInput  `json:"runtimeConfigRefs"`
	EnvironmentVariables         []runtimeEnvironmentVariableInput  `json:"environmentVariables"`
	ConfigFiles                  string                             `json:"configFiles"`
	SecretFiles                  string                             `json:"secretFiles"`
	DataVolumes                  []deploymentTargetDataVolumeInput  `json:"dataVolumes"`
	RequireApproval              bool                               `json:"requireApproval"`
	WebConsoleEnabled            *bool                              `json:"webConsoleEnabled"`
	Enabled                      bool                               `json:"enabled"`
}

func runtimeConfigMap(raw string) map[string]string {
	values, err := runtimeconfig.DecodeKeyValue(raw)
	if err != nil {
		return map[string]string{}
	}
	return values
}

type deploymentTargetEmptyDirInput struct {
	Medium    string `json:"medium"`
	SizeLimit string `json:"sizeLimit"`
}

type deploymentTargetDataVolumeInput struct {
	LogicalName     string                         `json:"logicalName"`
	SourceType      string                         `json:"sourceType"`
	ProjectVolumeID string                         `json:"projectVolumeId,omitempty"`
	MountPath       string                         `json:"mountPath,omitempty"`
	DevicePath      string                         `json:"devicePath,omitempty"`
	ReadOnly        bool                           `json:"readOnly,omitempty"`
	EmptyDir        *deploymentTargetEmptyDirInput `json:"emptyDir,omitempty"`
}

type deploymentTargetDataVolumeResponse struct {
	BindingID       string                         `json:"bindingId"`
	LogicalName     string                         `json:"logicalName"`
	SourceType      string                         `json:"sourceType"`
	ProjectVolumeID string                         `json:"projectVolumeId,omitempty"`
	MountPath       string                         `json:"mountPath,omitempty"`
	DevicePath      string                         `json:"devicePath,omitempty"`
	ReadOnly        bool                           `json:"readOnly"`
	EmptyDir        *deploymentTargetEmptyDirInput `json:"emptyDir,omitempty"`
	ActivationState string                         `json:"activationState"`
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
	output := make([]deploymentRuntimeConfigRefResponse, 0, len(refs))
	for _, ref := range refs {
		output = append(output, deploymentRuntimeConfigRefResponse{
			SetID: ref.SetID,
			Mode:  model.RuntimeConfigRefMode(ref.Mode),
		})
	}
	return output
}
