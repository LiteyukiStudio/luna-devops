package model

import (
	"time"

	"gorm.io/gorm"
)

const (
	ProjectVolumeOwnershipManaged    = "managed"
	ProjectVolumeOwnershipReferenced = "referenced"

	ProjectVolumeSourceBlank           = "blank"
	ProjectVolumeSourceManaged         = "managed"
	ProjectVolumeSourceRetained        = "retained"
	ProjectVolumeSourceArchiveImport   = "archive_import"
	ProjectVolumeSourceSnapshotRestore = "snapshot_restore"
	ProjectVolumeSourceExistingClaim   = "existing_claim"

	ProjectVolumeLifecycleProvisioning = "provisioning"
	ProjectVolumeLifecycleReady        = "ready"
	ProjectVolumeLifecycleDeleting     = "deleting"
	ProjectVolumeLifecycleError        = "error"

	ProjectVolumeAvailabilityAvailable   = "available"
	ProjectVolumeAvailabilityReserved    = "reserved"
	ProjectVolumeAvailabilityInUse       = "in_use"
	ProjectVolumeAvailabilityUnavailable = "unavailable"

	ProjectVolumeAccessReadWriteOnce    = "ReadWriteOnce"
	ProjectVolumeAccessReadWriteOncePod = "ReadWriteOncePod"
	ProjectVolumeAccessReadOnlyMany     = "ReadOnlyMany"
	ProjectVolumeAccessReadWriteMany    = "ReadWriteMany"

	ProjectVolumeModeFilesystem = "Filesystem"
	ProjectVolumeModeBlock      = "Block"
)

type ProjectVolumeBindingSummary struct {
	Reserved int64 `json:"reserved"`
	Active   int64 `json:"active"`
}

// ProjectVolume is the project-scoped desired state and durable identity for a
// Kubernetes PersistentVolumeClaim. Runtime phase and capacity observations
// are intentionally not persisted here.
type ProjectVolume struct {
	ID                       string                      `gorm:"primaryKey" json:"id"`
	ProjectID                string                      `gorm:"index;not null" json:"projectId"`
	DisplayName              string                      `gorm:"not null" json:"displayName"`
	ClusterID                string                      `gorm:"index;not null" json:"clusterId"`
	Namespace                string                      `gorm:"not null" json:"namespace"`
	ClaimName                string                      `gorm:"not null" json:"claimName"`
	OwnershipMode            string                      `gorm:"not null" json:"ownershipMode"`
	SourceKind               string                      `gorm:"not null" json:"sourceKind"`
	SourceSnapshotName       string                      `gorm:"not null;default:''" json:"sourceSnapshotName,omitempty"`
	LifecycleState           string                      `gorm:"index;not null;default:provisioning" json:"lifecycleState"`
	PendingOperation         string                      `gorm:"not null;default:provision" json:"pendingOperation,omitempty"`
	CapacityRequest          string                      `gorm:"not null" json:"capacity"`
	CapacityBytes            int64                       `gorm:"not null" json:"capacityBytes"`
	StorageClassName         string                      `gorm:"not null;default:''" json:"storageClassName"`
	AccessMode               string                      `gorm:"not null" json:"accessMode"`
	VolumeMode               string                      `gorm:"not null" json:"volumeMode"`
	SourceApplicationID      *string                     `gorm:"index" json:"sourceApplicationId,omitempty"`
	SourceApplicationName    string                      `gorm:"not null;default:''" json:"sourceApplicationName,omitempty"`
	SourceDeploymentTargetID *string                     `gorm:"index" json:"sourceDeploymentTargetId,omitempty"`
	CreatedBy                string                      `gorm:"index;not null" json:"createdBy"`
	Revision                 int64                       `gorm:"not null;default:1" json:"revision"`
	IdempotencyKeyHash       string                      `gorm:"column:idempotency_key_hash;not null;default:''" json:"-"`
	IdempotencyRequestHash   string                      `gorm:"column:idempotency_request_hash;not null;default:''" json:"-"`
	LastErrorCode            string                      `gorm:"not null;default:''" json:"lastErrorCode,omitempty"`
	LastErrorMessage         string                      `gorm:"type:text;not null;default:''" json:"-"`
	Availability             string                      `gorm:"-" json:"availability"`
	BindingSummary           ProjectVolumeBindingSummary `gorm:"-" json:"bindingSummary"`
	CreatedAt                time.Time                   `json:"createdAt"`
	UpdatedAt                time.Time                   `json:"updatedAt"`
	DeletedAt                gorm.DeletedAt              `gorm:"index" json:"-"`
}
