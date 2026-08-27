package volumetransferapi

import (
	"context"
	"io"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
)

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
	UserID    string    `json:"userId"`
	SubjectID string    `json:"subjectId"`
	Deadline  time.Time `json:"deadline"`
}

type DownloadAuthorization struct {
	Ticket    string
	ExpiresAt time.Time
}

type Download struct {
	Body        io.ReadCloser
	ContentType string
}

type StreamResult struct {
	TransferredBytes int64
	ProcessedFiles   int64
	SHA256           string
	LogicalBytes     int64
	DataSHA256       string
}

type ExportStream interface {
	io.ReadCloser
	Wait() (StreamResult, error)
}

// RuntimeStreamer connects the API request body/response directly to the
// deterministic transfer Pod prepared by the Worker.
type RuntimeStreamer interface {
	OpenVolumeTransferImport(context.Context, model.ProjectVolume, model.VolumeTransfer, io.Reader) (StreamResult, error)
	OpenVolumeTransferExport(context.Context, model.ProjectVolume, model.VolumeTransfer) (ExportStream, error)
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
