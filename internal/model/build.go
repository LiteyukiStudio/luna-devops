package model

import (
	"time"

	"gorm.io/gorm"
)

type BuildRun struct {
	ID                      string         `gorm:"primaryKey" json:"id"`
	ProjectID               string         `gorm:"index;not null" json:"projectId"`
	ApplicationID           string         `gorm:"index" json:"applicationId"`
	DeploymentTargetID      string         `gorm:"index" json:"deploymentTargetId"`
	BuildLabels             string         `json:"buildLabels"`
	BuildVariableSetIDs     string         `gorm:"type:text" json:"buildVariableSetIds"`
	BuildVariablesSnapshot  string         `gorm:"type:text;not null;default:'{}'" json:"-"`
	BuildSecretRefsSnapshot string         `gorm:"type:text;not null;default:'{}'" json:"-"`
	Status                  string         `gorm:"index;not null;default:queued" json:"status"`
	TriggerType             string         `gorm:"not null;default:manual" json:"triggerType"`
	SourceBranch            string         `json:"sourceBranch"`
	SourceTag               string         `json:"sourceTag"`
	SourceCommit            string         `json:"sourceCommit"`
	BuildDefinitionMode     string         `gorm:"not null;default:repository_dockerfile" json:"buildDefinitionMode"`
	BuildTemplateID         string         `gorm:"index;not null;default:''" json:"buildTemplateId"`
	BuildTemplateVersion    string         `gorm:"not null;default:''" json:"buildTemplateVersion"`
	BuildTemplateValues     string         `gorm:"type:text;not null;default:'{}'" json:"buildTemplateValues"`
	BuildTemplateDockerfile string         `gorm:"type:text;not null;default:''" json:"-"`
	BuildTemplateChecksum   string         `gorm:"not null;default:''" json:"buildTemplateChecksum"`
	DockerfilePath          string         `gorm:"not null;default:Dockerfile" json:"dockerfilePath"`
	BuildContext            string         `gorm:"not null;default:." json:"buildContext"`
	BuildDirectory          string         `json:"buildDirectory"`
	BuildArgs               string         `gorm:"type:text;not null;default:''" json:"buildArgs"`
	BuildEnvironmentID      string         `gorm:"index;not null;default:''" json:"buildEnvironmentId"`
	BuildCPURequest         string         `gorm:"not null;default:'1'" json:"buildCpuRequest"`
	BuildMemoryRequest      string         `gorm:"not null;default:'1Gi'" json:"buildMemoryRequest"`
	BuildTimeoutSeconds     int            `gorm:"not null;default:1800" json:"buildTimeoutSeconds"`
	TargetRegistryID        string         `gorm:"index" json:"targetRegistryId"`
	TargetRepository        string         `json:"targetRepository"`
	TargetTag               string         `json:"targetTag"`
	ImageRef                string         `json:"imageRef"`
	ImageDigest             string         `json:"imageDigest"`
	CacheConfig             string         `json:"cacheConfig"`
	CPUCoreSeconds          int64          `json:"cpuCoreSeconds"`
	MemoryMBSeconds         int64          `json:"memoryMbSeconds"`
	CreditCost              int64          `json:"creditCost"`
	StartedAt               *time.Time     `json:"startedAt"`
	FinishedAt              *time.Time     `json:"finishedAt"`
	CreatedBy               string         `gorm:"index" json:"createdBy"`
	TriggeredByName         string         `json:"triggeredByName"`
	TriggeredByEmail        string         `json:"triggeredByEmail"`
	SourceAuthorName        string         `json:"sourceAuthorName"`
	SourceAuthorEmail       string         `json:"sourceAuthorEmail"`
	CreatedAt               time.Time      `json:"createdAt"`
	UpdatedAt               time.Time      `json:"updatedAt"`
	DeletedAt               gorm.DeletedAt `gorm:"index" json:"-"`
}

const (
	BuildEnvironmentScopeGlobal      = "global"
	BuildEnvironmentScopeApplication = "application"
	BuildEnvironmentScopeDeployment  = "deployment"
	BuildEnvironmentGlobalRef        = "platform"
)

type BuildEnvironmentConfig struct {
	ID         string    `gorm:"primaryKey" json:"id"`
	Scope      string    `gorm:"uniqueIndex:idx_build_environment_scope_ref;not null" json:"scope"`
	ScopeRef   string    `gorm:"uniqueIndex:idx_build_environment_scope_ref;not null" json:"scopeRef"`
	Variables  string    `gorm:"type:text;not null;default:'{}'" json:"variables"`
	SecretRefs string    `gorm:"type:text;not null;default:'{}'" json:"-"`
	UpdatedBy  string    `gorm:"index;not null" json:"updatedBy"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type DeploymentTargetHookBinding struct {
	ID            string    `gorm:"primaryKey" json:"id"`
	ProjectID     string    `gorm:"index;not null" json:"projectId"`
	ApplicationID string    `gorm:"index;not null" json:"applicationId"`
	TargetID      string    `gorm:"uniqueIndex:idx_deployment_target_hook_bindings_target_hook;index;not null" json:"deploymentTargetId"`
	HookConfigID  string    `gorm:"uniqueIndex:idx_deployment_target_hook_bindings_target_hook;index;not null" json:"hookConfigId"`
	Phase         string    `gorm:"uniqueIndex:idx_deployment_target_hook_bindings_target_hook;index;not null" json:"phase"`
	RunOrder      int       `gorm:"not null;default:0" json:"runOrder"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type BuildVariableSet struct {
	ID         string         `gorm:"primaryKey" json:"id"`
	Name       string         `gorm:"not null" json:"name"`
	Scope      string         `gorm:"index;not null;default:global" json:"scope"`
	OwnerRef   string         `gorm:"index" json:"ownerRef"`
	ProjectIDs []string       `gorm:"-" json:"projectIds"`
	Variables  string         `gorm:"type:text" json:"variables"`
	SecretRefs string         `gorm:"type:text;not null;default:''" json:"-"`
	Enabled    bool           `gorm:"not null;default:true" json:"enabled"`
	CreatedBy  string         `gorm:"index" json:"createdBy"`
	CreatedAt  time.Time      `json:"createdAt"`
	UpdatedAt  time.Time      `json:"updatedAt"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
}

type BuildJob struct {
	ID              string         `gorm:"primaryKey" json:"id"`
	BuildRunID      string         `gorm:"index;not null" json:"buildRunId"`
	ProjectID       string         `gorm:"index;not null" json:"projectId"`
	Type            string         `gorm:"not null;default:build" json:"type"`
	Status          string         `gorm:"index;not null;default:queued" json:"status"`
	BuilderID       string         `gorm:"index" json:"builderId"`
	LeaseToken      string         `gorm:"index" json:"-"`
	LeaseUntil      *time.Time     `gorm:"index" json:"leaseUntil"`
	LastHeartbeatAt *time.Time     `gorm:"index" json:"lastHeartbeatAt"`
	ExecutorID      string         `json:"executorId"`
	ExecutorName    string         `json:"executorName"`
	Message         string         `json:"message"`
	LogRef          string         `json:"logRef"`
	Attempts        int            `gorm:"not null;default:0" json:"attempts"`
	StartedAt       *time.Time     `json:"startedAt"`
	FinishedAt      *time.Time     `json:"finishedAt"`
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       time.Time      `json:"updatedAt"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}

type BuildLog struct {
	ID         string    `gorm:"primaryKey" json:"id"`
	BuildRunID string    `gorm:"index;not null" json:"buildRunId"`
	BuildJobID string    `gorm:"uniqueIndex;not null" json:"buildJobId"`
	ProjectID  string    `gorm:"index;not null" json:"projectId"`
	Content    string    `gorm:"type:text" json:"content"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}
