package volumemigration

import (
	"context"
	"errors"

	"github.com/LiteyukiStudio/devops/internal/model"
)

const (
	DefaultPageSize = 100
	MaxPageSize     = 100

	ModeDryRun = "dry_run"
	ModeApply  = "apply"

	OutcomePlanned   = "planned"
	OutcomeApplied   = "applied"
	OutcomeUnchanged = "unchanged"

	RepairLegacyRetainedStateUnsupported = "volume_migration.legacy_retained_state_unsupported"
	RepairLegacyDataVolumesInvalid       = "volume_migration.legacy_data_volumes_invalid"
	RepairLegacyMountInvalid             = "volume_migration.legacy_mount_invalid"
	RepairLegacyBlockDevicePathMissing   = "volume_migration.legacy_block_device_path_missing"
	RepairRuntimeClusterUnavailable      = "volume_migration.runtime_cluster_unavailable"
	RepairClaimNotFound                  = "volume_migration.claim_not_found"
	RepairClaimObservationUnavailable    = "volume_migration.claim_observation_unavailable"
	RepairClaimOwnershipConflict         = "volume_migration.claim_ownership_conflict"
	RepairClaimLabelsMismatch            = "volume_migration.claim_labels_mismatch"
	RepairClaimSpecUnsupported           = "volume_migration.claim_spec_unsupported"
	RepairWorkloadObservationUnavailable = "volume_migration.workload_observation_unavailable"
	RepairWorkloadMountMismatch          = "volume_migration.workload_mount_mismatch"
	RepairRetainedVolumeMissing          = "volume_migration.retained_volume_missing"
	RepairProjectVolumeMissing           = "volume_migration.project_volume_missing"
	RepairProjectVolumeConflict          = "volume_migration.project_volume_conflict"
	RepairDeploymentMountConflict        = "volume_migration.deployment_mount_conflict"
)

var (
	ErrInvalidOptions          = errors.New("volume migration options are invalid")
	ErrRuntimeCluster          = errors.New("volume migration runtime cluster is unavailable")
	ErrClaimNotFound           = errors.New("volume migration claim was not found")
	ErrClaimObservation        = errors.New("volume migration claim observation is unavailable")
	ErrClaimOwnership          = errors.New("volume migration claim ownership conflicts")
	ErrWorkloadNotFound        = errors.New("volume migration workload was not found")
	ErrWorkloadObservation     = errors.New("volume migration workload observation is unavailable")
	ErrProjectVolumeConflict   = errors.New("volume migration project volume conflicts with existing data")
	ErrDeploymentMountConflict = errors.New("volume migration deployment mount conflicts with existing data")
)

type Options struct {
	Apply     bool
	PageSize  int
	ProjectID string
}

func (options Options) Mode() string {
	if options.Apply {
		return ModeApply
	}
	return ModeDryRun
}

type RepairItem struct {
	ID           string `json:"id"`
	Code         string `json:"code"`
	ResourceKind string `json:"resourceKind"`
	SourceID     string `json:"sourceId"`
	ProjectID    string `json:"projectId"`
	ClusterID    string `json:"clusterId,omitempty"`
	Namespace    string `json:"namespace,omitempty"`
	ClaimName    string `json:"claimName,omitempty"`
}

type Reconciliation struct {
	Projects                  int64 `json:"projects"`
	DeploymentTargets         int64 `json:"deploymentTargets"`
	SourceRetainedVolumes     int64 `json:"sourceRetainedVolumes"`
	SourcePersistentMounts    int64 `json:"sourcePersistentMounts"`
	SourceEmptyDirMounts      int64 `json:"sourceEmptyDirMounts"`
	ExpectedProjectVolumes    int64 `json:"expectedProjectVolumes"`
	VerifiedProjectVolumes    int64 `json:"verifiedProjectVolumes"`
	PlannedProjectVolumes     int64 `json:"plannedProjectVolumes"`
	AppliedProjectVolumes     int64 `json:"appliedProjectVolumes"`
	UnchangedProjectVolumes   int64 `json:"unchangedProjectVolumes"`
	ExpectedDeploymentMounts  int64 `json:"expectedDeploymentMounts"`
	VerifiedDeploymentMounts  int64 `json:"verifiedDeploymentMounts"`
	PlannedDeploymentMounts   int64 `json:"plannedDeploymentMounts"`
	AppliedDeploymentMounts   int64 `json:"appliedDeploymentMounts"`
	UnchangedDeploymentMounts int64 `json:"unchangedDeploymentMounts"`
	ObservedCapacityBytes     int64 `json:"observedCapacityBytes"`
	VerifiedCapacityBytes     int64 `json:"verifiedCapacityBytes"`
	RepairItems               int64 `json:"repairItems"`
	PlanBalanced              bool  `json:"planBalanced"`
	DatabaseBalanced          bool  `json:"databaseBalanced"`
	ReadyForSwitch            bool  `json:"readyForSwitch"`
}

type Report struct {
	SchemaVersion  int            `json:"schemaVersion"`
	Mode           string         `json:"mode"`
	PageSize       int            `json:"pageSize"`
	ProjectFilter  string         `json:"projectFilter,omitempty"`
	Reconciliation Reconciliation `json:"reconciliation"`
	Repairs        []RepairItem   `json:"repairs"`
}

type ClaimInspectionInput struct {
	ProjectID       string
	ProjectVolumeID string
	ClusterID       string
	Namespace       string
	ClaimName       string
}

type ClaimObservation struct {
	Exists               bool
	CapacityRequest      string
	CapacityBytes        int64
	StorageClassName     string
	AccessModes          []string
	VolumeMode           string
	ManagedBy            string
	OwnerProjectID       string
	OwnerProjectVolumeID string
	ActivePodReferences  int
}

type WorkloadInspectionInput struct {
	ClusterID    string
	Namespace    string
	Name         string
	WorkloadType string
}

type WorkloadAttachment struct {
	ClaimName  string
	MountPath  string
	DevicePath string
	ReadOnly   bool
	EmptyDir   bool
}

// Inspector is the only Kubernetes boundary used by the backfill service.
// Implementations must continue the supplied context and return observations,
// never kubeconfig material or credentials.
type Inspector interface {
	InspectClaim(context.Context, ClaimInspectionInput) (ClaimObservation, error)
	InspectWorkload(context.Context, WorkloadInspectionInput) (map[string]WorkloadAttachment, error)
}

type SyncResult struct {
	Outcome       string
	CapacityBytes int64
}

// Repository owns pagination and idempotent synchronization. All list methods
// use one-based pages and a page size that has already been limited to 100.
type Repository interface {
	ListProjects(context.Context, int, int, string) ([]model.Project, error)
	ListRetainedVolumes(context.Context, string, int, int) ([]model.RetainedVolume, error)
	ListDeploymentTargets(context.Context, string, int, int) ([]model.DeploymentTarget, error)
	ResolveRuntimeClusterID(context.Context, string) (string, error)
	GetApplication(context.Context, string, string) (model.Application, error)
	GetProjectVolume(context.Context, string, string) (model.ProjectVolume, error)
	SyncProjectVolume(context.Context, model.ProjectVolume, bool) (SyncResult, error)
	SyncDeploymentVolumeMount(context.Context, model.DeploymentVolumeMount, bool) (SyncResult, error)
}
