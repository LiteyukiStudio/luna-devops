package api

import (
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
)

type deploymentTargetResponse struct {
	ID                   string                              `json:"id"`
	ProjectID            string                              `json:"projectId"`
	ApplicationID        string                              `json:"applicationId"`
	EnvironmentID        string                              `json:"environmentId"`
	Name                 string                              `json:"name"`
	ServicePort          int                                 `json:"servicePort"`
	SourceType           string                              `json:"sourceType"`
	RepositoryBindingID  string                              `json:"repositoryBindingId"`
	DockerfilePath       string                              `json:"dockerfilePath"`
	BuildContext         string                              `json:"buildContext"`
	BuildDirectory       string                              `json:"buildDirectory"`
	BuildEnvironmentID   string                              `json:"buildEnvironmentId"`
	BuildCPURequest      string                              `json:"buildCpuRequest"`
	BuildMemoryRequest   string                              `json:"buildMemoryRequest"`
	TargetRegistryID     string                              `json:"targetRegistryId"`
	TargetRepository     string                              `json:"targetRepository"`
	TargetTag            string                              `json:"targetTag"`
	ImageRef             string                              `json:"imageRef"`
	BuildLabels          string                              `json:"buildLabels"`
	BuildVariableSetIDs  string                              `json:"buildVariableSetIds"`
	BuildHooksEnabled    bool                                `json:"buildHooksEnabled"`
	BuildHookBindings    []model.DeploymentTargetHookBinding `json:"buildHookBindings"`
	AutoDeploy           bool                                `json:"autoDeploy"`
	BranchPattern        string                              `json:"branchPattern"`
	TagPattern           string                              `json:"tagPattern"`
	ConcurrencyPolicy    string                              `json:"concurrencyPolicy"`
	RuntimeConfigSetIDs  string                              `json:"runtimeConfigSetIds"`
	EnvVars              string                              `json:"envVars"`
	ConfigRefs           string                              `json:"configRefs"`
	SecretRefsSet        bool                                `json:"secretRefsSet"`
	ConfigFiles          string                              `json:"configFiles"`
	SecretFilesSet       bool                                `json:"secretFilesSet"`
	DataRetentionEnabled bool                                `json:"dataRetentionEnabled"`
	DataCapacity         string                              `json:"dataCapacity"`
	DataMountPath        string                              `json:"dataMountPath"`
	DataVolumes          string                              `json:"dataVolumes"`
	RequireApproval      bool                                `json:"requireApproval"`
	Enabled              bool                                `json:"enabled"`
	DeleteStatus         string                              `json:"deleteStatus"`
	DeleteMessage        string                              `json:"deleteMessage"`
	DeleteStartedAt      *time.Time                          `json:"deleteStartedAt"`
	DeleteFinishedAt     *time.Time                          `json:"deleteFinishedAt"`
	CreatedBy            string                              `json:"createdBy"`
	CreatedAt            time.Time                           `json:"createdAt"`
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
		ServicePort:          fallbackInt(target.ServicePort, 8080),
		SourceType:           normalizeDeploymentSourceType(target.SourceType),
		RepositoryBindingID:  target.RepositoryBindingID,
		DockerfilePath:       target.DockerfilePath,
		BuildContext:         target.BuildContext,
		BuildDirectory:       target.BuildDirectory,
		BuildEnvironmentID:   fallback(strings.TrimSpace(target.BuildEnvironmentID), target.EnvironmentID),
		BuildCPURequest:      fallback(strings.TrimSpace(target.BuildCPURequest), defaultBuildCPURequest),
		BuildMemoryRequest:   fallback(strings.TrimSpace(target.BuildMemoryRequest), defaultBuildMemoryRequest),
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

type deploymentTargetInput struct {
	Name                 string                             `json:"name"`
	EnvironmentID        string                             `json:"environmentId" binding:"required"`
	ServicePort          int                                `json:"servicePort"`
	SourceType           string                             `json:"sourceType"`
	RepositoryBindingID  string                             `json:"repositoryBindingId"`
	DockerfilePath       string                             `json:"dockerfilePath"`
	BuildContext         string                             `json:"buildContext"`
	BuildDirectory       string                             `json:"buildDirectory"`
	BuildEnvironmentID   string                             `json:"buildEnvironmentId"`
	BuildCPURequest      string                             `json:"buildCpuRequest"`
	BuildMemoryRequest   string                             `json:"buildMemoryRequest"`
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
