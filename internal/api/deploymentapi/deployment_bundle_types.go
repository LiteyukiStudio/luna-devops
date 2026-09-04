package deploymentapi

import "time"

const (
	deploymentBundleKind           = "luna-devops.deployment-target"
	deploymentBundleSchemaVersion  = 1
	deploymentBundleMaxBytes       = 1 << 20
	deploymentBundleMaxDepth       = 32
	deploymentBundleMaxReferences  = 100
	deploymentBundleSecretMaxBytes = 8192

	deploymentBundleStatusReady           = "ready"
	deploymentBundleStatusRequiresMapping = "requires_mapping"
	deploymentBundleStatusInvalid         = "invalid"

	deploymentBundleReferenceResolved     = "resolved"
	deploymentBundleReferenceMissing      = "missing"
	deploymentBundleReferenceAmbiguous    = "ambiguous"
	deploymentBundleReferenceForbidden    = "forbidden"
	deploymentBundleReferenceIncompatible = "incompatible"
)

const (
	deploymentBundleReferenceRepositoryBinding = "repositoryBinding"
	deploymentBundleReferenceRuntimeCluster    = "runtimeCluster"
	deploymentBundleReferenceArtifactRegistry  = "artifactRegistry"
	deploymentBundleReferenceBuildVariableSet  = "buildVariableSet"
	deploymentBundleReferenceRuntimeConfigSet  = "runtimeConfigSet"
	deploymentBundleReferenceHookConfig        = "hookConfig"
	deploymentBundleReferenceProjectVolume     = "projectVolume"
)

const (
	deploymentBundleSecretBuild       = "build"
	deploymentBundleSecretRuntimeEnv  = "runtimeEnv"
	deploymentBundleSecretRuntimeFile = "runtimeFile"
)

type deploymentTargetBundle struct {
	SchemaVersion      int                                 `json:"schemaVersion"`
	Kind               string                              `json:"kind"`
	ExportedAt         time.Time                           `json:"exportedAt"`
	Configuration      deploymentTargetInput               `json:"configuration"`
	References         []deploymentBundleReference         `json:"references"`
	SecretRequirements []deploymentBundleSecretRequirement `json:"secretRequirements"`
	Omissions          []string                            `json:"omissions"`
}

type deploymentBundleReference struct {
	Key      string                              `json:"key"`
	Kind     string                              `json:"kind"`
	Required bool                                `json:"required"`
	Usage    string                              `json:"usage"`
	Source   deploymentBundleReferenceDescriptor `json:"source"`
}

type deploymentBundleReferenceDescriptor struct {
	Name             string `json:"name,omitempty"`
	Type             string `json:"type,omitempty"`
	Scope            string `json:"scope,omitempty"`
	Owner            string `json:"owner,omitempty"`
	Repository       string `json:"repository,omitempty"`
	Namespace        string `json:"namespace,omitempty"`
	Mode             string `json:"mode,omitempty"`
	Phase            string `json:"phase,omitempty"`
	RunOrder         int    `json:"runOrder,omitempty"`
	LogicalName      string `json:"logicalName,omitempty"`
	MountPath        string `json:"mountPath,omitempty"`
	DevicePath       string `json:"devicePath,omitempty"`
	ReadOnly         bool   `json:"readOnly,omitempty"`
	AccessMode       string `json:"accessMode,omitempty"`
	VolumeMode       string `json:"volumeMode,omitempty"`
	StorageClassName string `json:"storageClassName,omitempty"`
	ClusterName      string `json:"clusterName,omitempty"`
}

type deploymentBundleSecretRequirement struct {
	Key    string `json:"key"`
	Target string `json:"target"`
	Name   string `json:"name,omitempty"`
	Path   string `json:"path,omitempty"`
}

type deploymentTargetBundleImportRequest struct {
	Bundle       deploymentTargetBundle          `json:"bundle"`
	Digest       string                          `json:"digest,omitempty"`
	Mappings     map[string]string               `json:"mappings,omitempty"`
	Overrides    deploymentTargetBundleOverrides `json:"overrides,omitempty"`
	SecretValues map[string]string               `json:"secretValues,omitempty"`
}

type deploymentTargetBundleOverrides struct {
	Name  string `json:"name,omitempty"`
	Stage string `json:"stage,omitempty"`
}

type deploymentTargetBundlePreview struct {
	Digest             string                                `json:"digest"`
	Status             string                                `json:"status"`
	Summary            deploymentTargetBundlePreviewSummary  `json:"summary"`
	References         []deploymentBundleReferenceResolution `json:"references"`
	SecretRequirements []deploymentBundleSecretRequirement   `json:"secretRequirements"`
	Warnings           []string                              `json:"warnings"`
}

type deploymentTargetBundlePreviewSummary struct {
	Name       string `json:"name"`
	Stage      string `json:"stage"`
	SourceType string `json:"sourceType"`
}

type deploymentBundleReferenceResolution struct {
	deploymentBundleReference
	Status         string                               `json:"status"`
	ResolvedID     string                               `json:"resolvedId,omitempty"`
	Candidates     []deploymentBundleReferenceCandidate `json:"candidates"`
	CandidateCount int                                  `json:"candidateCount"`
	Truncated      bool                                 `json:"truncated"`
	Code           string                               `json:"code,omitempty"`
}

type deploymentBundleReferenceCandidatesRequest struct {
	Reference deploymentBundleReference `json:"reference"`
}

type deploymentBundleReferenceCandidatePage struct {
	Items      []deploymentBundleReferenceCandidate `json:"items"`
	Page       int                                  `json:"page"`
	PageSize   int                                  `json:"pageSize"`
	SortBy     string                               `json:"sortBy"`
	SortOrder  string                               `json:"sortOrder"`
	Total      int64                                `json:"total"`
	TotalPages int                                  `json:"totalPages"`
}

type deploymentBundleReferenceCandidate struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Matched     bool   `json:"matched"`
	Compatible  bool   `json:"compatible"`
}

type deploymentTargetBundleImportPlan struct {
	Preview      deploymentTargetBundlePreview
	Input        deploymentTargetInput
	SecretValues []deploymentBundleSecretValue
}

type deploymentBundleSecretValue struct {
	Requirement deploymentBundleSecretRequirement
	Value       string
}

type deploymentBundleError struct {
	Code    string
	Message string
	Cause   error
}

func (err *deploymentBundleError) Error() string {
	if err == nil {
		return "deployment bundle operation failed"
	}
	if spec, ok := deploymentBundleErrorSpecFor(err.Code); ok {
		return spec.Message
	}
	return deploymentBundleErrorCatalog["deployment_bundle.internal_error"].Message
}

func (err *deploymentBundleError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}
