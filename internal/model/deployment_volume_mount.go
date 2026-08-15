package model

import (
	"time"

	"gorm.io/gorm"
)

const (
	DeploymentVolumeSourceProjectVolume = "project_volume"
	DeploymentVolumeSourceEmptyDir      = "empty_dir"

	DeploymentVolumeActivationReserved       = "reserved"
	DeploymentVolumeActivationActive         = "active"
	DeploymentVolumeActivationReleasePending = "release_pending"
	DeploymentVolumeActivationError          = "error"
)

// DeploymentVolumeMount is a desired deployment-to-volume relation. A
// release worker promotes reserved relations only after the workload has been
// observed using the expected PVC.
type DeploymentVolumeMount struct {
	ID                 string         `gorm:"primaryKey" json:"id"`
	ProjectID          string         `gorm:"index;not null" json:"projectId"`
	ApplicationID      string         `gorm:"index;not null" json:"applicationId"`
	DeploymentTargetID string         `gorm:"index;not null" json:"deploymentTargetId"`
	SourceType         string         `gorm:"not null" json:"sourceType"`
	ProjectVolumeID    *string        `gorm:"index" json:"projectVolumeId,omitempty"`
	LogicalName        string         `gorm:"not null" json:"logicalName"`
	MountPath          *string        `json:"mountPath,omitempty"`
	DevicePath         *string        `json:"devicePath,omitempty"`
	ReadOnly           bool           `gorm:"not null;default:false" json:"readOnly"`
	Exclusive          bool           `gorm:"not null;default:false" json:"exclusive"`
	ActivationState    string         `gorm:"index;not null;default:reserved" json:"activationState"`
	EmptyDirMedium     string         `gorm:"not null;default:''" json:"emptyDirMedium,omitempty"`
	EmptyDirSizeLimit  string         `gorm:"not null;default:''" json:"emptyDirSizeLimit,omitempty"`
	LastErrorCode      string         `gorm:"not null;default:''" json:"lastErrorCode,omitempty"`
	LastErrorMessage   string         `gorm:"type:text;not null;default:''" json:"-"`
	CreatedAt          time.Time      `json:"createdAt"`
	UpdatedAt          time.Time      `json:"updatedAt"`
	DeletedAt          gorm.DeletedAt `gorm:"index" json:"-"`
}
