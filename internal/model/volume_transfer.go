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
	VolumeTransferStateUploading = "uploading"
	VolumeTransferStateQueued    = "queued"
	VolumeTransferStateRunning   = "running"
	VolumeTransferStateSucceeded = "succeeded"
	VolumeTransferStateFailed    = "failed"
	VolumeTransferStateCancelled = "cancelled"
	VolumeTransferStateExpired   = "expired"

	VolumeTransferPartStateReserved  = "reserved"
	VolumeTransferPartStateCompleted = "completed"
)

// VolumeTransfer records import/export workflow history. Object-store
// references and callback credentials are server-internal and never serialized.
type VolumeTransfer struct {
	ID                          string     `gorm:"primaryKey" json:"id"`
	ProjectID                   string     `gorm:"index;not null" json:"projectId"`
	ProjectVolumeID             string     `gorm:"index;not null" json:"projectVolumeId"`
	Direction                   string     `gorm:"not null" json:"direction"`
	Format                      string     `gorm:"not null" json:"format"`
	ConsistencyMode             string     `gorm:"not null" json:"consistencyMode"`
	State                       string     `gorm:"index;not null;default:created" json:"state"`
	ObjectKey                   string     `gorm:"not null" json:"-"`
	ObjectOwned                 bool       `gorm:"not null;default:true" json:"-"`
	ObjectCleanupStartedAt      *time.Time `json:"-"`
	ObjectCleanupLeaseToken     string     `gorm:"not null;default:''" json:"-"`
	ObjectCleanupLeaseExpiresAt *time.Time `json:"-"`
	MultipartUploadID           string     `gorm:"not null;default:''" json:"-"`
	SourceFilename              string     `gorm:"not null;default:''" json:"sourceFilename,omitempty"`
	ExpectedBytes               int64      `gorm:"not null;default:0" json:"expectedBytes"`
	TransferredBytes            int64      `gorm:"not null;default:0" json:"transferredBytes"`
	ProcessedFiles              int64      `gorm:"not null;default:0" json:"processedFiles"`
	Phase                       string     `gorm:"not null;default:''" json:"phase,omitempty"`
	SHA256                      string     `gorm:"column:sha256;not null;default:''" json:"sha256,omitempty"`
	LogicalBytes                int64      `gorm:"not null;default:0" json:"logicalBytes"`
	DataSHA256                  string     `gorm:"column:data_sha256;not null;default:''" json:"dataSHA256,omitempty"`
	ActorID                     string     `gorm:"index;not null" json:"createdBy"`
	CallbackTokenHash           string     `gorm:"not null;default:''" json:"-"`
	CallbackTokenExpiresAt      *time.Time `json:"-"`
	CompletionReportedAt        *time.Time `json:"-"`
	JobSucceededAt              *time.Time `json:"-"`
	ExecutionCleanupCompletedAt *time.Time `json:"-"`
	ExecutionGeneration         int64      `gorm:"not null;default:0" json:"-"`
	CreationLeaseOwner          string     `gorm:"not null;default:''" json:"-"`
	CreationLeaseExpiresAt      *time.Time `json:"-"`
	JobCreatedAt                *time.Time `json:"-"`
	ExpiresAt                   time.Time  `gorm:"index;not null" json:"expiresAt"`
	StartedAt                   *time.Time `json:"startedAt,omitempty"`
	FinishedAt                  *time.Time `json:"finishedAt,omitempty"`
	ObjectDeletedAt             *time.Time `json:"-"`
	LastErrorCode               string     `gorm:"not null;default:''" json:"lastErrorCode,omitempty"`
	LastErrorMessage            string     `gorm:"type:text;not null;default:''" json:"-"`
	CreatedAt                   time.Time  `json:"createdAt"`
	UpdatedAt                   time.Time  `json:"updatedAt"`
}

type VolumeTransferPart struct {
	TransferID     string     `gorm:"primaryKey" json:"transferId"`
	PartNumber     int        `gorm:"primaryKey" json:"partNumber"`
	Offset         int64      `gorm:"column:byte_offset;not null" json:"offset"`
	Size           int64      `gorm:"not null" json:"size"`
	ETag           string     `gorm:"column:etag;not null" json:"etag"`
	SHA256         string     `gorm:"column:sha256;not null" json:"sha256"`
	State          string     `gorm:"not null;default:completed" json:"-"`
	LeaseToken     string     `gorm:"not null;default:''" json:"-"`
	LeaseExpiresAt *time.Time `json:"-"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}
