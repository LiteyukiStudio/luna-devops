package model

import (
	"time"

	"gorm.io/gorm"
)

type BuildProvider struct {
	ID        string         `gorm:"primaryKey" json:"id"`
	Name      string         `gorm:"not null" json:"name"`
	Type      string         `gorm:"not null;default:platform" json:"type"`
	Scope     string         `gorm:"index;not null;default:global" json:"scope"`
	OwnerRef  string         `gorm:"index" json:"ownerRef"`
	Config    string         `json:"config"`
	Enabled   bool           `gorm:"not null;default:true" json:"enabled"`
	CreatedBy string         `gorm:"index" json:"createdBy"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

type BuildRun struct {
	ID                  string         `gorm:"primaryKey" json:"id"`
	ProjectID           string         `gorm:"index;not null" json:"projectId"`
	ApplicationID       string         `gorm:"index" json:"applicationId"`
	BuildProviderID     string         `gorm:"index" json:"buildProviderId"`
	BuildLabels         string         `json:"buildLabels"`
	BuildVariableSetIDs string         `gorm:"type:text" json:"buildVariableSetIds"`
	Status              string         `gorm:"index;not null;default:queued" json:"status"`
	TriggerType         string         `gorm:"not null;default:manual" json:"triggerType"`
	SourceBranch        string         `json:"sourceBranch"`
	SourceTag           string         `json:"sourceTag"`
	SourceCommit        string         `json:"sourceCommit"`
	DockerfilePath      string         `gorm:"not null;default:Dockerfile" json:"dockerfilePath"`
	BuildContext        string         `gorm:"not null;default:." json:"buildContext"`
	BuildDirectory      string         `json:"buildDirectory"`
	TargetRegistryID    string         `gorm:"index" json:"targetRegistryId"`
	TargetRepository    string         `json:"targetRepository"`
	TargetTag           string         `json:"targetTag"`
	ImageRef            string         `json:"imageRef"`
	ImageDigest         string         `json:"imageDigest"`
	CacheConfig         string         `json:"cacheConfig"`
	CPUCoreSeconds      int64          `json:"cpuCoreSeconds"`
	MemoryMBSeconds     int64          `json:"memoryMbSeconds"`
	CreditCost          int64          `json:"creditCost"`
	StartedAt           *time.Time     `json:"startedAt"`
	FinishedAt          *time.Time     `json:"finishedAt"`
	CreatedBy           string         `gorm:"index" json:"createdBy"`
	CreatedAt           time.Time      `json:"createdAt"`
	UpdatedAt           time.Time      `json:"updatedAt"`
	DeletedAt           gorm.DeletedAt `gorm:"index" json:"-"`
}

type BuildVariableSet struct {
	ID        string         `gorm:"primaryKey" json:"id"`
	Name      string         `gorm:"not null" json:"name"`
	Scope     string         `gorm:"index;not null;default:global" json:"scope"`
	OwnerRef  string         `gorm:"index" json:"ownerRef"`
	Variables string         `gorm:"type:text" json:"variables"`
	Enabled   bool           `gorm:"not null;default:true" json:"enabled"`
	CreatedBy string         `gorm:"index" json:"createdBy"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

type BuildJob struct {
	ID         string         `gorm:"primaryKey" json:"id"`
	BuildRunID string         `gorm:"index;not null" json:"buildRunId"`
	ProjectID  string         `gorm:"index;not null" json:"projectId"`
	Type       string         `gorm:"not null;default:build" json:"type"`
	Status     string         `gorm:"index;not null;default:queued" json:"status"`
	BuilderID  string         `gorm:"index" json:"builderId"`
	LeaseUntil *time.Time     `gorm:"index" json:"leaseUntil"`
	Message    string         `json:"message"`
	LogRef     string         `json:"logRef"`
	Attempts   int            `gorm:"not null;default:0" json:"attempts"`
	StartedAt  *time.Time     `json:"startedAt"`
	FinishedAt *time.Time     `json:"finishedAt"`
	CreatedAt  time.Time      `json:"createdAt"`
	UpdatedAt  time.Time      `json:"updatedAt"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
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

type BuilderAgent struct {
	ID                 string     `gorm:"primaryKey" json:"id"`
	Name               string     `gorm:"not null" json:"name"`
	Labels             string     `json:"labels"`
	Scopes             string     `json:"scopes"`
	Executor           string     `json:"executor"`
	Status             string     `gorm:"index;not null;default:online" json:"status"`
	MaxConcurrency     int        `gorm:"not null;default:1" json:"maxConcurrency"`
	CurrentConcurrency int        `gorm:"not null;default:0" json:"currentConcurrency"`
	LastHeartbeatAt    *time.Time `json:"lastHeartbeatAt"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}
