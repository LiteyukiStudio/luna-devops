package model

import "time"

const (
	RetainedVolumeStatusRetaining = "retaining"
	RetainedVolumeStatusRetained  = "retained"
	RetainedVolumeStatusReserved  = "reserved"
	RetainedVolumeStatusClaimed   = "claimed"
	RetainedVolumeStatusDeleting  = "deleting"
	RetainedVolumeStatusFailed    = "delete_failed"
)

// RetainedVolume maps the legacy table only for the explicit volume-center
// backfill command. API, worker, and provider runtime paths must not use it.
type RetainedVolume struct {
	ID                       string     `gorm:"primaryKey" json:"id"`
	ProjectID                string     `gorm:"index;not null" json:"projectId"`
	SourceApplicationID      string     `gorm:"index;not null" json:"sourceApplicationId"`
	SourceApplicationName    string     `gorm:"not null;default:''" json:"sourceApplicationName"`
	SourceDeploymentTargetID string     `gorm:"index;not null" json:"sourceDeploymentTargetId"`
	ClusterID                string     `gorm:"index;not null" json:"clusterId"`
	Namespace                string     `gorm:"not null" json:"namespace"`
	ClaimName                string     `gorm:"not null" json:"claimName"`
	VolumeName               string     `gorm:"not null;default:data" json:"volumeName"`
	MountPath                string     `gorm:"not null;default:/data" json:"mountPath"`
	Capacity                 string     `gorm:"not null;default:''" json:"capacity"`
	StorageClassName         string     `gorm:"not null;default:''" json:"storageClassName"`
	AccessMode               string     `gorm:"not null;default:''" json:"accessMode"`
	VolumeMode               string     `gorm:"not null;default:''" json:"volumeMode"`
	Status                   string     `gorm:"index;not null;default:retained" json:"status"`
	ClaimedByApplicationID   string     `gorm:"index;not null;default:''" json:"claimedByApplicationId"`
	ClaimedByTargetID        string     `gorm:"index;not null;default:''" json:"claimedByTargetId"`
	LastError                string     `gorm:"type:text;not null;default:''" json:"lastError"`
	RetainedAt               time.Time  `gorm:"not null" json:"retainedAt"`
	ClaimedAt                *time.Time `json:"claimedAt"`
	CreatedAt                time.Time  `json:"createdAt"`
	UpdatedAt                time.Time  `json:"updatedAt"`
}
