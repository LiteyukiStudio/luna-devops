package volumetransferapi

import (
	"io"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/volumetransfer"
)

const (
	MinimumChunkSize  = volumetransfer.MinimumChunkSize
	MaximumChunkSize  = volumetransfer.MaximumChunkSize
	MaxMultipartParts = volumetransfer.MaxMultipartParts

	DefaultChunkSize = MinimumChunkSize
)

// RequiredChunkSize returns the server-selected multipart size for a transfer
// of expectedBytes. Sizes are MiB-aligned so every supported 5 TiB transfer
// fits within S3's 10,000-part limit without accepting client-selected parts.
func RequiredChunkSize(expectedBytes int64) int64 {
	return volumetransfer.RequiredChunkSize(expectedBytes)
}

type Actor struct {
	UserID    string
	CanManage bool
}

type ImportRequest struct {
	ProjectID        string
	Namespace        string
	DisplayName      string
	ClusterID        string
	CapacityRequest  string
	CapacityBytes    int64
	StorageClassName string
	AccessMode       string
	VolumeMode       string
	Format           string
	Filename         string
	ContentLength    int64
	SHA256           string
	ActorID          string
	IdempotencyKey   string
}

type ImportResult struct {
	Volume   model.ProjectVolume
	Transfer model.VolumeTransfer
}

type ExportRequest struct {
	ProjectID      string
	VolumeID       string
	Format         string
	Consistency    string
	ActorID        string
	IdempotencyKey string
}

type DownloadBinding struct {
	UserID            string    `json:"userId"`
	SubjectID         string    `json:"subjectId"`
	AssertionID       string    `json:"assertionId,omitempty"`
	AssertionRequired bool      `json:"assertionRequired"`
	Deadline          time.Time `json:"deadline"`
}

type DownloadAuthorization struct {
	Ticket    string
	ExpiresAt time.Time
}

type DownloadCredential struct {
	Ticket  string
	Session string
}

type DownloadSession struct {
	Token     string
	ExpiresAt time.Time
}

type ContentInfo struct {
	Offset    int64
	Size      int64
	ChunkSize int64
	ETag      string
}

type Download struct {
	Body         io.ReadCloser
	Status       int
	ContentType  string
	Size         int64
	ETag         string
	ContentRange string
}

type Progress struct {
	ExpectedState    string
	TransferredBytes int64
	ProcessedFiles   int64
	Stage            string
}

type Completion struct {
	ExpectedState    string
	TransferredBytes int64
	SHA256           string
	LogicalBytes     int64
	DataSHA256       string
}

type BlockManifest struct {
	SchemaVersion   int       `json:"schemaVersion"`
	VolumeMode      string    `json:"volumeMode"`
	Format          string    `json:"format"`
	ExportedAt      time.Time `json:"exportedAt"`
	LogicalBytes    int64     `json:"logicalBytes"`
	FileCount       int64     `json:"fileCount"`
	DataSHA256      string    `json:"dataSHA256"`
	ConsistencyMode string    `json:"consistencyMode"`
}

type Failure struct {
	ExpectedState string
	ErrorCode     string
	Diagnostic    string
}
