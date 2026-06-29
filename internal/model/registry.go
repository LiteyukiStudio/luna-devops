package model

import (
	"gorm.io/gorm"
	"time"
)

type ArtifactRegistry struct {
	ID                string         `gorm:"primaryKey" json:"id"`
	Name              string         `gorm:"not null" json:"name"`
	Provider          string         `gorm:"not null" json:"provider"`
	Endpoint          string         `gorm:"not null" json:"endpoint"`
	Namespace         string         `json:"namespace"`
	Scope             string         `gorm:"index;not null;default:global" json:"scope"`
	OwnerRef          string         `gorm:"index" json:"ownerRef"`
	ProjectIDs        []string       `gorm:"-" json:"projectIds"`
	DefaultProjectIDs []string       `gorm:"-" json:"defaultProjectIds"`
	CredentialRef     string         `json:"credentialRef"`
	IsDefault         bool           `gorm:"not null;default:false" json:"isDefault"`
	Capabilities      string         `json:"capabilities"`
	CreatedBy         string         `gorm:"index" json:"createdBy"`
	CreatedAt         time.Time      `json:"createdAt"`
	UpdatedAt         time.Time      `json:"updatedAt"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"-"`
}

type RegistryCredential struct {
	ID                 string         `gorm:"primaryKey" json:"id"`
	RegistryID         string         `gorm:"index;not null" json:"registryId"`
	Name               string         `gorm:"not null" json:"name"`
	Username           string         `json:"username"`
	PasswordRef        string         `json:"-"`
	TokenRef           string         `json:"-"`
	Scope              string         `gorm:"not null;default:push-pull" json:"scope"`
	AccessScope        string         `gorm:"not null;default:personal" json:"accessScope"`
	RepositoryTemplate string         `gorm:"type:text;not null;default:''" json:"repositoryTemplate"`
	TagTemplate        string         `gorm:"type:text;not null;default:''" json:"tagTemplate"`
	CreatedBy          string         `gorm:"index" json:"createdBy"`
	CreatedAt          time.Time      `json:"createdAt"`
	UpdatedAt          time.Time      `json:"updatedAt"`
	DeletedAt          gorm.DeletedAt `gorm:"index" json:"-"`
}

type ContainerImage struct {
	ID            string         `gorm:"primaryKey" json:"id"`
	ProjectID     string         `gorm:"index" json:"projectId"`
	ApplicationID string         `gorm:"index" json:"applicationId"`
	RegistryID    string         `gorm:"index;not null" json:"registryId"`
	Repository    string         `gorm:"not null" json:"repository"`
	Tag           string         `gorm:"not null" json:"tag"`
	Digest        string         `json:"digest"`
	ImageRef      string         `gorm:"not null" json:"imageRef"`
	SourceCommit  string         `json:"sourceCommit"`
	BuildRunID    string         `gorm:"index" json:"buildRunId"`
	SourceType    string         `gorm:"not null;default:manual-image" json:"sourceType"`
	ScanStatus    string         `gorm:"not null;default:unknown" json:"scanStatus"`
	CreatedBy     string         `gorm:"index" json:"createdBy"`
	CreatedAt     time.Time      `json:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}
