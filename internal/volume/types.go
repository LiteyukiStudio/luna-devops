package volume

import (
	"context"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
)

const (
	DefaultPageSize = 20
	MaxPageSize     = 100

	OperationProvision = "provision"
	OperationExpand    = "expand"
	OperationDelete    = "delete"
	OperationImport    = "import"
	OperationExport    = "export"
	OperationCleanup   = "cleanup"
)

type ProjectVolumeListOptions struct {
	Page           int
	PageSize       int
	SortBy         string
	SortOrder      string
	Search         string
	Availability   string
	LifecycleState string
	ClusterID      string
	SourceKind     string
	OwnershipMode  string
	VolumeMode     string
}

type ProjectVolumeListResult struct {
	Items      []model.ProjectVolume `json:"items"`
	Page       int                   `json:"page"`
	PageSize   int                   `json:"pageSize"`
	SortBy     string                `json:"sortBy"`
	SortOrder  string                `json:"sortOrder"`
	Total      int64                 `json:"total"`
	TotalPages int                   `json:"totalPages"`
}

type ProjectVolumeDetail struct {
	Volume             model.ProjectVolume           `json:"volume"`
	Bindings           []model.DeploymentVolumeMount `json:"bindings"`
	BindingPage        int                           `json:"bindingPage"`
	BindingPageSize    int                           `json:"bindingPageSize"`
	BindingTotal       int64                         `json:"bindingTotal"`
	BindingTotalPages  int                           `json:"bindingTotalPages"`
	Transfers          []model.VolumeTransfer        `json:"transfers"`
	TransferPage       int                           `json:"transferPage"`
	TransferPageSize   int                           `json:"transferPageSize"`
	TransferTotal      int64                         `json:"transferTotal"`
	TransferTotalPages int                           `json:"transferTotalPages"`
}

type ProjectVolumeDeletionPreview struct {
	Volume                model.ProjectVolume           `json:"volume"`
	BlockingBindings      []model.DeploymentVolumeMount `json:"blockingBindings"`
	BlockingBindingCount  int64                         `json:"blockingBindingCount"`
	BlockingTransfers     []model.VolumeTransfer        `json:"blockingTransfers"`
	BlockingTransferCount int64                         `json:"blockingTransferCount"`
	RequiredDataAction    string                        `json:"requiredDataAction"`
	CanDelete             bool                          `json:"canDelete"`
}

type CreateProjectVolumeInput struct {
	ProjectID                string
	DisplayName              string
	ClusterID                string
	Namespace                string
	ClaimName                string
	OwnershipMode            string
	SourceKind               string
	SourceSnapshotName       string
	CapacityRequest          string
	CapacityBytes            int64
	StorageClassName         string
	AccessMode               string
	VolumeMode               string
	SourceApplicationID      string
	SourceApplicationName    string
	SourceDeploymentTargetID string
	ActorID                  string
	IdempotencyKey           string
}

type CreateProjectVolumeResult struct {
	Volume   model.ProjectVolume
	Replayed bool
}

type UpdateProjectVolumeInput struct {
	ActorID         string
	DisplayName     *string
	CapacityRequest *string
	CapacityBytes   *int64
}

type DeleteProjectVolumeInput struct {
	ProjectID        string
	VolumeID         string
	ActorID          string
	ExpectedRevision int64
	DataAction       string
}

type ReserveMountInput struct {
	ProjectID          string
	ApplicationID      string
	DeploymentTargetID string
	SourceType         string
	ProjectVolumeID    string
	LogicalName        string
	MountPath          string
	DevicePath         string
	ReadOnly           bool
	EmptyDirMedium     string
	EmptyDirSizeLimit  string
}

type VolumeTransferListOptions struct {
	Page      int
	PageSize  int
	SortBy    string
	SortOrder string
	Direction string
	State     string
	VolumeID  string
	CreatedBy string
}

type VolumeTransferListResult struct {
	Items      []model.VolumeTransfer `json:"items"`
	Page       int                    `json:"page"`
	PageSize   int                    `json:"pageSize"`
	SortBy     string                 `json:"sortBy"`
	SortOrder  string                 `json:"sortOrder"`
	Total      int64                  `json:"total"`
	TotalPages int                    `json:"totalPages"`
}

type CreateVolumeTransferInput struct {
	ProjectID       string
	ProjectVolumeID string
	Direction       string
	Format          string
	ConsistencyMode string
	SourceFilename  string
	ExpectedBytes   int64
	ActorID         string
	ExpiresAt       time.Time
	// IdempotencyKey is hashed into an internal transfer identity and is never
	// persisted or exposed. It is used to make one transfer request idempotent.
	IdempotencyKey string
}

type TransferProgress struct {
	TransferredBytes int64
	ProcessedFiles   int64
	Phase            string
}

// TransferCompletion contains only execution results observed by the transfer
// Job. DataSHA256 hashes the uncompressed Block device bytes and is never
// accepted from a public import request.
type TransferCompletion struct {
	ExpectedState    string
	TransferredBytes int64
	ProcessedFiles   int64
	SHA256           string
	LogicalBytes     int64
	DataSHA256       string
}

type MaintenanceScanOptions struct {
	Cutoff time.Time
	Limit  int
}

type VolumeOperation struct {
	Kind       string
	ProjectID  string
	VolumeID   string
	TransferID string
	ActorID    string
}

// OperationDispatcher is implemented by the API/Worker task adapter. The
// domain does not import Asynq or provider packages, so task context can retain
// its parent trace without reversing dependency direction.
type OperationDispatcher interface {
	DispatchVolumeOperation(context.Context, VolumeOperation) error
}

type ExistingClaimInspectionInput struct {
	ProjectID string
	VolumeID  string
	ClusterID string
	Namespace string
	ClaimName string
}

type ExistingClaimInspection struct {
	CapacityRequest     string
	CapacityBytes       int64
	StorageClassName    string
	AccessMode          string
	VolumeMode          string
	ManagedBy           string
	OwnerProjectID      string
	OwnerVolumeID       string
	ActivePodReferences int
}

// ExistingClaimInspector is an API-side adapter over the cluster provider.
// Implementations must build their Kubernetes client from trusted cluster and
// Secret Store state; request data never contains kubeconfig material.
type ExistingClaimInspector interface {
	InspectExistingClaim(context.Context, ExistingClaimInspectionInput) (ExistingClaimInspection, error)
}
