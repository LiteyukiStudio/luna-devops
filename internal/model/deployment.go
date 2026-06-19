package model

import (
	"time"

	"gorm.io/gorm"
)

type RuntimeCluster struct {
	ID                  string         `gorm:"primaryKey" json:"id"`
	Name                string         `gorm:"not null" json:"name"`
	Type                string         `gorm:"not null;default:kubernetes" json:"type"`
	Endpoint            string         `json:"endpoint"`
	Scope               string         `gorm:"index;not null;default:global" json:"scope"`
	OwnerRef            string         `gorm:"index" json:"ownerRef"`
	ProjectIDs          []string       `gorm:"-" json:"projectIds"`
	KubeconfigRef       string         `json:"-"`
	KubeconfigSet       bool           `gorm:"-" json:"kubeconfigSet"`
	Kubeconfig          string         `gorm:"-" json:"kubeconfig,omitempty"`
	IsDefault           bool           `gorm:"not null;default:false" json:"isDefault"`
	MaxConcurrentBuilds int            `gorm:"not null;default:4" json:"maxConcurrentBuilds"`
	Status              string         `gorm:"not null;default:unknown" json:"status"`
	LastCheckedAt       *time.Time     `json:"lastCheckedAt"`
	CreatedBy           string         `gorm:"index" json:"createdBy"`
	CreatedAt           time.Time      `json:"createdAt"`
	UpdatedAt           time.Time      `json:"updatedAt"`
	DeletedAt           gorm.DeletedAt `gorm:"index" json:"-"`
}

type Environment struct {
	ID            string         `gorm:"primaryKey" json:"id"`
	ProjectID     string         `gorm:"index;not null" json:"projectId"`
	Name          string         `gorm:"not null" json:"name"`
	Slug          string         `gorm:"index;not null" json:"slug"`
	Stage         string         `gorm:"not null;default:dev" json:"stage"`
	ClusterID     string         `gorm:"index" json:"clusterId"`
	Namespace     string         `json:"namespace"`
	Replicas      int            `gorm:"not null;default:1" json:"replicas"`
	CPURequest    string         `json:"cpuRequest"`
	MemoryRequest string         `json:"memoryRequest"`
	EnvVars       string         `json:"envVars"`
	ConfigRefs    string         `json:"configRefs"`
	SecretRefs    string         `json:"secretRefs"`
	CreatedBy     string         `gorm:"index" json:"createdBy"`
	CreatedAt     time.Time      `json:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

type Release struct {
	ID                 string         `gorm:"primaryKey" json:"id"`
	ProjectID          string         `gorm:"index;not null" json:"projectId"`
	ApplicationID      string         `gorm:"index;not null" json:"applicationId"`
	EnvironmentID      string         `gorm:"index;not null" json:"environmentId"`
	DeploymentTargetID string         `gorm:"index;not null;default:''" json:"deploymentTargetId"`
	BuildRunID         string         `gorm:"index" json:"buildRunId"`
	ImageRef           string         `gorm:"not null" json:"imageRef"`
	Type               string         `gorm:"not null;default:deploy" json:"type"`
	Status             string         `gorm:"index;not null;default:pending" json:"status"`
	Revision           int            `gorm:"not null;default:1" json:"revision"`
	RollbackFromID     string         `gorm:"index" json:"rollbackFromId"`
	Message            string         `json:"message"`
	StartedAt          *time.Time     `json:"startedAt"`
	FinishedAt         *time.Time     `json:"finishedAt"`
	CreatedBy          string         `gorm:"index" json:"createdBy"`
	CreatedAt          time.Time      `json:"createdAt"`
	UpdatedAt          time.Time      `json:"updatedAt"`
	DeletedAt          gorm.DeletedAt `gorm:"index" json:"-"`
}

type DeploymentTarget struct {
	ID                   string                        `gorm:"primaryKey" json:"id"`
	ProjectID            string                        `gorm:"index;not null" json:"projectId"`
	ApplicationID        string                        `gorm:"index;not null" json:"applicationId"`
	EnvironmentID        string                        `gorm:"index;not null" json:"environmentId"`
	Name                 string                        `gorm:"not null" json:"name"`
	ServicePort          int                           `gorm:"not null;default:8080" json:"servicePort"`
	DeleteStatus         string                        `gorm:"index;not null;default:active" json:"deleteStatus"`
	DeleteMessage        string                        `gorm:"type:text;not null;default:''" json:"deleteMessage"`
	DeleteStartedAt      *time.Time                    `json:"deleteStartedAt"`
	DeleteFinishedAt     *time.Time                    `json:"deleteFinishedAt"`
	SourceType           string                        `gorm:"not null;default:repository" json:"sourceType"`
	RepositoryBindingID  string                        `gorm:"index" json:"repositoryBindingId"`
	DockerfilePath       string                        `gorm:"not null;default:Dockerfile" json:"dockerfilePath"`
	BuildContext         string                        `gorm:"not null;default:." json:"buildContext"`
	BuildDirectory       string                        `json:"buildDirectory"`
	BuildEnvironmentID   string                        `gorm:"index;not null;default:''" json:"buildEnvironmentId"`
	BuildCPURequest      string                        `gorm:"not null;default:'1'" json:"buildCpuRequest"`
	BuildMemoryRequest   string                        `gorm:"not null;default:'1Gi'" json:"buildMemoryRequest"`
	TargetRegistryID     string                        `gorm:"index" json:"targetRegistryId"`
	TargetRepository     string                        `json:"targetRepository"`
	TargetTag            string                        `json:"targetTag"`
	ImageRef             string                        `json:"imageRef"`
	BuildLabels          string                        `json:"buildLabels"`
	BuildVariableSetIDs  string                        `gorm:"type:text" json:"buildVariableSetIds"`
	BuildHooksEnabled    bool                          `gorm:"not null;default:true" json:"buildHooksEnabled"`
	BuildHookBindings    []DeploymentTargetHookBinding `gorm:"-" json:"buildHookBindings"`
	AutoDeploy           bool                          `gorm:"not null;default:false" json:"autoDeploy"`
	BranchPattern        string                        `json:"branchPattern"`
	TagPattern           string                        `json:"tagPattern"`
	ConcurrencyPolicy    string                        `gorm:"not null;default:queue" json:"concurrencyPolicy"`
	RuntimeConfigSetIDs  string                        `gorm:"type:text;not null;default:''" json:"runtimeConfigSetIds"`
	EnvVars              string                        `gorm:"type:text;not null;default:''" json:"envVars"`
	ConfigRefs           string                        `gorm:"type:text;not null;default:''" json:"configRefs"`
	SecretRefs           string                        `gorm:"type:text;not null;default:''" json:"-"`
	ConfigFiles          string                        `gorm:"type:text;not null;default:''" json:"configFiles"`
	SecretFiles          string                        `gorm:"type:text;not null;default:''" json:"-"`
	DataRetentionEnabled bool                          `gorm:"not null;default:false" json:"dataRetentionEnabled"`
	DataCapacity         string                        `gorm:"not null;default:''" json:"dataCapacity"`
	DataMountPath        string                        `gorm:"not null;default:'/data'" json:"dataMountPath"`
	DataVolumes          string                        `gorm:"type:text;not null;default:''" json:"dataVolumes"`
	RequireApproval      bool                          `gorm:"not null;default:false" json:"requireApproval"`
	Enabled              bool                          `gorm:"not null;default:true" json:"enabled"`
	CreatedBy            string                        `gorm:"index" json:"createdBy"`
	CreatedAt            time.Time                     `json:"createdAt"`
	UpdatedAt            time.Time                     `json:"updatedAt"`
	DeletedAt            gorm.DeletedAt                `gorm:"index" json:"-"`
}

type ProjectRuntimeConfigSet struct {
	ID               string         `gorm:"primaryKey" json:"id"`
	ProjectID        string         `gorm:"index;not null" json:"projectId"`
	Name             string         `gorm:"not null" json:"name"`
	EnvVars          string         `gorm:"type:text;not null;default:''" json:"envVars"`
	ConfigFiles      string         `gorm:"type:text;not null;default:''" json:"configFiles"`
	SecretRefs       string         `gorm:"type:text;not null;default:''" json:"-"`
	SecretFiles      string         `gorm:"type:text;not null;default:''" json:"-"`
	Enabled          bool           `gorm:"not null;default:true" json:"enabled"`
	DeleteStatus     string         `gorm:"index;not null;default:active" json:"deleteStatus"`
	DeleteMessage    string         `gorm:"type:text;not null;default:''" json:"deleteMessage"`
	DeleteStartedAt  *time.Time     `json:"deleteStartedAt"`
	DeleteFinishedAt *time.Time     `json:"deleteFinishedAt"`
	CreatedBy        string         `gorm:"index" json:"createdBy"`
	CreatedAt        time.Time      `json:"createdAt"`
	UpdatedAt        time.Time      `json:"updatedAt"`
	DeletedAt        gorm.DeletedAt `gorm:"index" json:"-"`
}

type ReleaseLog struct {
	ID        string    `gorm:"primaryKey" json:"id"`
	ReleaseID string    `gorm:"uniqueIndex;not null" json:"releaseId"`
	ProjectID string    `gorm:"index;not null" json:"projectId"`
	Content   string    `gorm:"type:text" json:"content"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
