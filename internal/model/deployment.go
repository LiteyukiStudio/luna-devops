package model

import (
	"time"

	"gorm.io/gorm"
)

type RuntimeCluster struct {
	ID            string         `gorm:"primaryKey" json:"id"`
	Name          string         `gorm:"not null" json:"name"`
	Type          string         `gorm:"not null;default:kubernetes" json:"type"`
	Endpoint      string         `json:"endpoint"`
	Scope         string         `gorm:"index;not null;default:global" json:"scope"`
	OwnerRef      string         `gorm:"index" json:"ownerRef"`
	KubeconfigRef string         `json:"-"`
	KubeconfigSet bool           `gorm:"-" json:"kubeconfigSet"`
	Kubeconfig    string         `gorm:"-" json:"kubeconfig,omitempty"`
	IsDefault     bool           `gorm:"not null;default:false" json:"isDefault"`
	Status        string         `gorm:"not null;default:unknown" json:"status"`
	LastCheckedAt *time.Time     `json:"lastCheckedAt"`
	CreatedBy     string         `gorm:"index" json:"createdBy"`
	CreatedAt     time.Time      `json:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
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
	ID             string         `gorm:"primaryKey" json:"id"`
	ProjectID      string         `gorm:"index;not null" json:"projectId"`
	ApplicationID  string         `gorm:"index;not null" json:"applicationId"`
	EnvironmentID  string         `gorm:"index;not null" json:"environmentId"`
	BuildRunID     string         `gorm:"index" json:"buildRunId"`
	ImageRef       string         `gorm:"not null" json:"imageRef"`
	Type           string         `gorm:"not null;default:deploy" json:"type"`
	Status         string         `gorm:"index;not null;default:pending" json:"status"`
	Revision       int            `gorm:"not null;default:1" json:"revision"`
	RollbackFromID string         `gorm:"index" json:"rollbackFromId"`
	Message        string         `json:"message"`
	StartedAt      *time.Time     `json:"startedAt"`
	FinishedAt     *time.Time     `json:"finishedAt"`
	CreatedBy      string         `gorm:"index" json:"createdBy"`
	CreatedAt      time.Time      `json:"createdAt"`
	UpdatedAt      time.Time      `json:"updatedAt"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}
