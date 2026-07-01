package api

import (
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
)

type deploymentTargetResponse struct {
	ID                   string                               `json:"id"`
	ProjectID            string                               `json:"projectId"`
	ApplicationID        string                               `json:"applicationId"`
	EnvironmentID        string                               `json:"environmentId"`
	Name                 string                               `json:"name"`
	Stage                string                               `json:"stage"`
	ClusterID            string                               `json:"clusterId"`
	Namespace            string                               `json:"namespace"`
	Replicas             int                                  `json:"replicas"`
	CPURequest           string                               `json:"cpuRequest"`
	MemoryRequest        string                               `json:"memoryRequest"`
	ServicePort          int                                  `json:"servicePort"`
	ServicePorts         []model.DeploymentServicePort        `json:"servicePorts"`
	SourceType           string                               `json:"sourceType"`
	RepositoryBindingID  string                               `json:"repositoryBindingId"`
	DockerfilePath       string                               `json:"dockerfilePath"`
	BuildContext         string                               `json:"buildContext"`
	BuildDirectory       string                               `json:"buildDirectory"`
	BuildEnvironmentID   string                               `json:"buildEnvironmentId"`
	BuildCPURequest      string                               `json:"buildCpuRequest"`
	BuildMemoryRequest   string                               `json:"buildMemoryRequest"`
	BuildTimeoutSeconds  int                                  `json:"buildTimeoutSeconds"`
	TargetRegistryID     string                               `json:"targetRegistryId"`
	TargetRepository     string                               `json:"targetRepository"`
	TargetTag            string                               `json:"targetTag"`
	ImageRef             string                               `json:"imageRef"`
	BuildLabels          string                               `json:"buildLabels"`
	BuildVariableSetIDs  string                               `json:"buildVariableSetIds"`
	BuildHooksEnabled    bool                                 `json:"buildHooksEnabled"`
	BuildHookBindings    []model.DeploymentTargetHookBinding  `json:"buildHookBindings"`
	AutoDeploy           bool                                 `json:"autoDeploy"`
	BranchPattern        string                               `json:"branchPattern"`
	TagPattern           string                               `json:"tagPattern"`
	ConcurrencyPolicy    string                               `json:"concurrencyPolicy"`
	RuntimeConfigSetIDs  string                               `json:"runtimeConfigSetIds"`
	RuntimeConfigRefs    []deploymentRuntimeConfigRefResponse `json:"runtimeConfigRefs"`
	EnvVars              string                               `json:"envVars"`
	ConfigRefs           string                               `json:"configRefs"`
	SecretRefsSet        bool                                 `json:"secretRefsSet"`
	ConfigFiles          string                               `json:"configFiles"`
	SecretFilesSet       bool                                 `json:"secretFilesSet"`
	DataRetentionEnabled bool                                 `json:"dataRetentionEnabled"`
	DataCapacity         string                               `json:"dataCapacity"`
	DataMountPath        string                               `json:"dataMountPath"`
	DataVolumes          string                               `json:"dataVolumes"`
	RequireApproval      bool                                 `json:"requireApproval"`
	Enabled              bool                                 `json:"enabled"`
	DeleteStatus         string                               `json:"deleteStatus"`
	DeleteMessage        string                               `json:"deleteMessage"`
	DeleteStartedAt      *time.Time                           `json:"deleteStartedAt"`
	DeleteFinishedAt     *time.Time                           `json:"deleteFinishedAt"`
	CreatedBy            string                               `json:"createdBy"`
	CreatedAt            time.Time                            `json:"createdAt"`
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
		ID:                   target.ID,
		ProjectID:            target.ProjectID,
		ApplicationID:        target.ApplicationID,
		EnvironmentID:        target.EnvironmentID,
		Name:                 target.Name,
		Stage:                normalizeStage(target.Stage),
		ClusterID:            target.ClusterID,
		Namespace:            target.Namespace,
		Replicas:             fallbackInt(target.Replicas, 1),
		CPURequest:           fallback(strings.TrimSpace(target.CPURequest), "1"),
		MemoryRequest:        fallback(strings.TrimSpace(target.MemoryRequest), "1Gi"),
		ServicePort:          fallbackInt(target.ServicePort, 8080),
		ServicePorts:         model.DeploymentTargetServicePorts(target),
		SourceType:           normalizeDeploymentSourceType(target.SourceType),
		RepositoryBindingID:  target.RepositoryBindingID,
		DockerfilePath:       target.DockerfilePath,
		BuildContext:         target.BuildContext,
		BuildDirectory:       target.BuildDirectory,
		BuildEnvironmentID:   strings.TrimSpace(target.BuildEnvironmentID),
		BuildCPURequest:      fallback(strings.TrimSpace(target.BuildCPURequest), defaultBuildCPURequest),
		BuildMemoryRequest:   fallback(strings.TrimSpace(target.BuildMemoryRequest), defaultBuildMemoryRequest),
		BuildTimeoutSeconds:  normalizeBuildTimeoutSecondsValue(target.BuildTimeoutSeconds),
		TargetRegistryID:     target.TargetRegistryID,
		TargetRepository:     target.TargetRepository,
		TargetTag:            target.TargetTag,
		ImageRef:             target.ImageRef,
		BuildLabels:          target.BuildLabels,
		BuildVariableSetIDs:  target.BuildVariableSetIDs,
		BuildHooksEnabled:    target.BuildHooksEnabled,
		BuildHookBindings:    target.BuildHookBindings,
		AutoDeploy:           target.AutoDeploy,
		BranchPattern:        target.BranchPattern,
		TagPattern:           target.TagPattern,
		ConcurrencyPolicy:    target.ConcurrencyPolicy,
		RuntimeConfigSetIDs:  target.RuntimeConfigSetIDs,
		RuntimeConfigRefs:    deploymentRuntimeConfigRefsResponse(target),
		EnvVars:              target.EnvVars,
		ConfigRefs:           target.ConfigRefs,
		SecretRefsSet:        strings.TrimSpace(target.SecretRefs) != "",
		ConfigFiles:          target.ConfigFiles,
		SecretFilesSet:       strings.TrimSpace(target.SecretFiles) != "" && strings.TrimSpace(target.SecretFiles) != "{}",
		DataRetentionEnabled: target.DataRetentionEnabled,
		DataCapacity:         target.DataCapacity,
		DataMountPath:        deploymentTargetDataMountPath(target),
		DataVolumes:          encodeDataVolumes(deploymentTargetDataVolumes(target)),
		RequireApproval:      target.RequireApproval,
		Enabled:              target.Enabled,
		DeleteStatus:         target.DeleteStatus,
		DeleteMessage:        target.DeleteMessage,
		DeleteStartedAt:      target.DeleteStartedAt,
		DeleteFinishedAt:     target.DeleteFinishedAt,
		CreatedBy:            target.CreatedBy,
		CreatedAt:            target.CreatedAt,
	}
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
	Name                 string                             `json:"name"`
	EnvironmentID        string                             `json:"environmentId"`
	Stage                string                             `json:"stage"`
	ClusterID            string                             `json:"clusterId"`
	Namespace            string                             `json:"namespace"`
	Replicas             int                                `json:"replicas"`
	CPURequest           string                             `json:"cpuRequest"`
	MemoryRequest        string                             `json:"memoryRequest"`
	ServicePort          int                                `json:"servicePort"`
	ServicePorts         []model.DeploymentServicePort      `json:"servicePorts"`
	SourceType           string                             `json:"sourceType"`
	RepositoryBindingID  string                             `json:"repositoryBindingId"`
	DockerfilePath       string                             `json:"dockerfilePath"`
	BuildContext         string                             `json:"buildContext"`
	BuildDirectory       string                             `json:"buildDirectory"`
	BuildEnvironmentID   string                             `json:"buildEnvironmentId"`
	BuildCPURequest      string                             `json:"buildCpuRequest"`
	BuildMemoryRequest   string                             `json:"buildMemoryRequest"`
	BuildTimeoutSeconds  int                                `json:"buildTimeoutSeconds"`
	TargetRegistryID     string                             `json:"targetRegistryId"`
	TargetImageRef       string                             `json:"targetImageRef"`
	TargetRepository     string                             `json:"targetRepository"`
	TargetTag            string                             `json:"targetTag"`
	ImageRef             string                             `json:"imageRef"`
	BuildLabels          string                             `json:"buildLabels"`
	BuildVariableSetIDs  []string                           `json:"buildVariableSetIds"`
	BuildHooksEnabled    *bool                              `json:"buildHooksEnabled"`
	BuildHookBindings    []deploymentTargetHookBindingInput `json:"buildHookBindings"`
	AutoDeploy           bool                               `json:"autoDeploy"`
	BranchPattern        string                             `json:"branchPattern"`
	TagPattern           string                             `json:"tagPattern"`
	ConcurrencyPolicy    string                             `json:"concurrencyPolicy"`
	RuntimeConfigSetIDs  []string                           `json:"runtimeConfigSetIds"`
	RuntimeConfigRefs    []deploymentRuntimeConfigRefInput  `json:"runtimeConfigRefs"`
	EnvVars              string                             `json:"envVars"`
	ConfigRefs           string                             `json:"configRefs"`
	SecretRefs           string                             `json:"secretRefs"`
	ConfigFiles          string                             `json:"configFiles"`
	SecretFiles          string                             `json:"secretFiles"`
	DataRetentionEnabled bool                               `json:"dataRetentionEnabled"`
	DataCapacity         string                             `json:"dataCapacity"`
	DataMountPath        string                             `json:"dataMountPath"`
	DataVolumes          string                             `json:"dataVolumes"`
	RequireApproval      bool                               `json:"requireApproval"`
	Enabled              bool                               `json:"enabled"`
}

type deploymentTargetDataVolumeInput struct {
	Name      string `json:"name"`
	MountPath string `json:"mountPath"`
	Capacity  string `json:"capacity"`
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
