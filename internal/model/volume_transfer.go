package model

import "time"

const (
	VolumeTransferDirectionImport = "import"
	VolumeTransferDirectionExport = "export"

	VolumeTransferFormatTarGZ  = "tar_gz"
	VolumeTransferFormatRawZST = "raw_zst"

	VolumeTransferConsistencySnapshot  = "snapshot"
	VolumeTransferConsistencyLive      = "live"
	VolumeTransferConsistencyUnmounted = "unmounted"

	VolumeTransferStateCreated   = "created"
	VolumeTransferStatePreparing = "preparing"
	VolumeTransferStateReady     = "ready"
	VolumeTransferStateStreaming = "streaming"
	VolumeTransferStateSucceeded = "succeeded"
	VolumeTransferStateFailed    = "failed"
	VolumeTransferStateCancelled = "cancelled"
	VolumeTransferStateExpired   = "expired"
)

// VolumeTransfer records direct import/export workflow history. Transfer data
// flows between the client and a runtime Pod and is never persisted here.
type VolumeTransfer struct {
	ID                          string     `gorm:"primaryKey" json:"id"`
	ProjectID                   string     `gorm:"index;not null" json:"projectId"`
	ProjectVolumeID             string     `gorm:"index;not null" json:"projectVolumeId"`
	Direction                   string     `gorm:"not null" json:"direction"`
	Format                      string     `gorm:"not null" json:"format"`
	ConsistencyMode             string     `gorm:"not null" json:"consistencyMode"`
	State                       string     `gorm:"index;not null;default:created" json:"state"`
	SourceFilename              string     `gorm:"not null;default:''" json:"sourceFilename,omitempty"`
	ExpectedBytes               int64      `gorm:"not null;default:0" json:"expectedBytes"`
	TransferredBytes            int64      `gorm:"not null;default:0" json:"transferredBytes"`
	ProcessedFiles              int64      `gorm:"not null;default:0" json:"processedFiles"`
	Phase                       string     `gorm:"not null;default:''" json:"phase,omitempty"`
	SHA256                      string     `gorm:"column:sha256;not null;default:''" json:"sha256,omitempty"`
	LogicalBytes                int64      `gorm:"not null;default:0" json:"logicalBytes"`
	DataSHA256                  string     `gorm:"column:data_sha256;not null;default:''" json:"dataSHA256,omitempty"`
	ActorID                     string     `gorm:"index;not null" json:"createdBy"`
	ExecutionCleanupCompletedAt *time.Time `json:"-"`
	ExecutionGeneration         int64      `gorm:"not null;default:0" json:"-"`
	CreationLeaseOwner          string     `gorm:"not null;default:''" json:"-"`
	CreationLeaseExpiresAt      *time.Time `json:"-"`
	JobCreatedAt                *time.Time `json:"-"`
	ExpiresAt                   time.Time  `gorm:"index;not null" json:"expiresAt"`
	StartedAt                   *time.Time `json:"startedAt,omitempty"`
	FinishedAt                  *time.Time `json:"finishedAt,omitempty"`
	LastErrorCode               string     `gorm:"not null;default:''" json:"lastErrorCode,omitempty"`
	LastErrorMessage            string     `gorm:"type:text;not null;default:''" json:"-"`
	CreatedAt                   time.Time  `json:"createdAt"`
	UpdatedAt                   time.Time  `json:"updatedAt"`
}
