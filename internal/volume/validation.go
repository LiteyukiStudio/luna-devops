package volume

import (
	"math"
	"path"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/LiteyukiStudio/devops/internal/model"
)

var (
	logicalNamePattern = regexp.MustCompile(`^[a-z0-9](?:[-a-z0-9]*[a-z0-9])?$`)
	sha256Pattern      = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

func normalizeProjectVolumeListOptions(options ProjectVolumeListOptions) (ProjectVolumeListOptions, error) {
	if options.Page < 1 {
		options.Page = 1
	}
	if options.PageSize < 1 {
		options.PageSize = DefaultPageSize
	}
	if options.PageSize > MaxPageSize {
		options.PageSize = MaxPageSize
	}
	options.Search = strings.TrimSpace(options.Search)
	options.Availability = strings.TrimSpace(options.Availability)
	options.LifecycleState = strings.TrimSpace(options.LifecycleState)
	options.ClusterID = strings.TrimSpace(options.ClusterID)
	options.SourceKind = strings.TrimSpace(options.SourceKind)
	options.OwnershipMode = strings.TrimSpace(options.OwnershipMode)
	options.VolumeMode = strings.TrimSpace(options.VolumeMode)
	options.SortBy = strings.TrimSpace(options.SortBy)
	if options.SortBy == "" {
		options.SortBy = "createdAt"
	}
	if _, ok := projectVolumeSortColumns[options.SortBy]; !ok {
		return ProjectVolumeListOptions{}, newDomainError(CodePaginationSortByInvalid, "unsupported project volume sort field")
	}
	options.SortOrder = strings.ToLower(strings.TrimSpace(options.SortOrder))
	if options.SortOrder == "" {
		options.SortOrder = "desc"
	}
	if options.SortOrder != "asc" && options.SortOrder != "desc" {
		return ProjectVolumeListOptions{}, newDomainError(CodePaginationOrderInvalid, "sort order must be asc or desc")
	}
	if options.Availability != "" && !oneOf(options.Availability,
		model.ProjectVolumeAvailabilityAvailable, model.ProjectVolumeAvailabilityReserved,
		model.ProjectVolumeAvailabilityInUse, model.ProjectVolumeAvailabilityUnavailable) {
		return ProjectVolumeListOptions{}, newDomainError(CodeInvalidInput, "project volume availability filter is invalid")
	}
	if options.LifecycleState != "" && !validLifecycleState(options.LifecycleState) {
		return ProjectVolumeListOptions{}, newDomainError(CodeInvalidInput, "project volume lifecycle filter is invalid")
	}
	if options.SourceKind != "" && !validSourceKind(options.SourceKind) {
		return ProjectVolumeListOptions{}, newDomainError(CodeInvalidInput, "project volume source filter is invalid")
	}
	if options.OwnershipMode != "" && !validOwnershipMode(options.OwnershipMode) {
		return ProjectVolumeListOptions{}, newDomainError(CodeInvalidInput, "project volume ownership filter is invalid")
	}
	if options.VolumeMode != "" && !validVolumeMode(options.VolumeMode) {
		return ProjectVolumeListOptions{}, newDomainError(CodeInvalidInput, "project volume mode filter is invalid")
	}
	return options, nil
}

func normalizeCreateProjectVolumeInput(input CreateProjectVolumeInput) CreateProjectVolumeInput {
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.ClusterID = strings.TrimSpace(input.ClusterID)
	input.Namespace = strings.TrimSpace(input.Namespace)
	input.ClaimName = strings.TrimSpace(input.ClaimName)
	input.OwnershipMode = strings.TrimSpace(input.OwnershipMode)
	input.SourceKind = strings.TrimSpace(input.SourceKind)
	input.SourceSnapshotName = strings.TrimSpace(input.SourceSnapshotName)
	input.CapacityRequest = strings.TrimSpace(input.CapacityRequest)
	input.StorageClassName = strings.TrimSpace(input.StorageClassName)
	input.AccessMode = strings.TrimSpace(input.AccessMode)
	input.VolumeMode = strings.TrimSpace(input.VolumeMode)
	input.SourceApplicationID = strings.TrimSpace(input.SourceApplicationID)
	input.SourceApplicationName = strings.TrimSpace(input.SourceApplicationName)
	input.SourceDeploymentTargetID = strings.TrimSpace(input.SourceDeploymentTargetID)
	input.ActorID = strings.TrimSpace(input.ActorID)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.OwnershipMode == "" {
		input.OwnershipMode = model.ProjectVolumeOwnershipManaged
	}
	return input
}

func validateCreateProjectVolumeInput(input CreateProjectVolumeInput) error {
	if input.ProjectID == "" || input.ClusterID == "" || input.Namespace == "" || input.ActorID == "" {
		return newDomainError(CodeInvalidInput, "project, cluster, namespace, and actor are required")
	}
	if !validDisplayName(input.DisplayName) {
		return newDomainError(CodeInvalidInput, "project volume display name is invalid")
	}
	if len(input.IdempotencyKey) < 8 || len(input.IdempotencyKey) > 160 {
		return newDomainError(CodeInvalidInput, "idempotency key must contain 8 to 160 characters")
	}
	if !validOwnershipMode(input.OwnershipMode) || !validSourceKind(input.SourceKind) {
		return newDomainError(CodeInvalidInput, "project volume ownership or source is invalid")
	}
	if input.OwnershipMode == model.ProjectVolumeOwnershipReferenced && input.SourceKind != model.ProjectVolumeSourceExistingClaim {
		return newDomainError(CodeInvalidInput, "referenced project volumes must use an existing claim")
	}
	if input.SourceKind == model.ProjectVolumeSourceExistingClaim && input.ClaimName == "" {
		return newDomainError(CodeInvalidInput, "existing claim name is required")
	}
	if input.SourceKind == model.ProjectVolumeSourceSnapshotRestore && input.SourceSnapshotName == "" {
		return newDomainError(CodeInvalidInput, "volume snapshot name is required")
	}
	if input.SourceKind != model.ProjectVolumeSourceSnapshotRestore && input.SourceSnapshotName != "" {
		return newDomainError(CodeInvalidInput, "snapshot name is only valid for snapshot restores")
	}
	if input.SourceKind != model.ProjectVolumeSourceExistingClaim && input.OwnershipMode != model.ProjectVolumeOwnershipManaged {
		return newDomainError(CodeInvalidInput, "platform-created project volumes must be managed")
	}
	if input.SourceKind != model.ProjectVolumeSourceExistingClaim && input.ClaimName != "" {
		return newDomainError(CodeInvalidInput, "claim name is generated for platform-created project volumes")
	}
	if input.SourceKind != model.ProjectVolumeSourceExistingClaim && (input.CapacityBytes <= 0 || input.CapacityRequest == "") {
		return newDomainError(CodeInvalidInput, "project volume capacity is required")
	}
	if input.SourceKind != model.ProjectVolumeSourceExistingClaim && input.StorageClassName == "" {
		return newDomainError(CodeInvalidInput, "storage class is required")
	}
	if input.SourceKind != model.ProjectVolumeSourceExistingClaim && (!validAccessMode(input.AccessMode) || !validVolumeMode(input.VolumeMode)) {
		return newDomainError(CodeInvalidInput, "project volume access mode or volume mode is invalid")
	}
	return nil
}

func normalizeReserveMountInput(input ReserveMountInput) ReserveMountInput {
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	input.ApplicationID = strings.TrimSpace(input.ApplicationID)
	input.DeploymentTargetID = strings.TrimSpace(input.DeploymentTargetID)
	input.SourceType = strings.TrimSpace(input.SourceType)
	input.ProjectVolumeID = strings.TrimSpace(input.ProjectVolumeID)
	input.LogicalName = strings.TrimSpace(input.LogicalName)
	input.MountPath = cleanAbsolutePath(input.MountPath)
	input.DevicePath = cleanAbsolutePath(input.DevicePath)
	input.EmptyDirMedium = strings.TrimSpace(input.EmptyDirMedium)
	input.EmptyDirSizeLimit = strings.TrimSpace(input.EmptyDirSizeLimit)
	return input
}

func validateReserveMountInput(input ReserveMountInput) error {
	if input.ProjectID == "" || input.ApplicationID == "" || input.DeploymentTargetID == "" {
		return newDomainError(CodeInvalidInput, "project, application, and deployment target are required")
	}
	if len(input.LogicalName) > 63 || !logicalNamePattern.MatchString(input.LogicalName) {
		return newDomainError(CodeInvalidInput, "deployment volume logical name is invalid")
	}
	switch input.SourceType {
	case model.DeploymentVolumeSourceProjectVolume:
		if input.ProjectVolumeID == "" || (input.MountPath == "") == (input.DevicePath == "") {
			return newDomainError(CodeInvalidInput, "project volume mounts require a volume and exactly one mount or device path")
		}
		if input.EmptyDirMedium != "" || input.EmptyDirSizeLimit != "" {
			return newDomainError(CodeInvalidInput, "project volume mounts cannot contain emptyDir settings")
		}
	case model.DeploymentVolumeSourceEmptyDir:
		if input.ProjectVolumeID != "" || input.MountPath == "" || input.DevicePath != "" {
			return newDomainError(CodeInvalidInput, "emptyDir mounts require only a mount path")
		}
		if input.EmptyDirMedium != "" && input.EmptyDirMedium != "Memory" {
			return newDomainError(CodeInvalidInput, "emptyDir medium must be empty or Memory")
		}
	default:
		return newDomainError(CodeInvalidInput, "deployment volume source type is invalid")
	}
	return nil
}

func applyVolumeMountPolicy(mount *model.DeploymentVolumeMount, volume model.ProjectVolume) error {
	if volume.VolumeMode == model.ProjectVolumeModeFilesystem && mount.MountPath == nil {
		return newDomainError(CodeBindingConflict, "filesystem project volumes require mountPath")
	}
	if volume.VolumeMode == model.ProjectVolumeModeBlock && mount.DevicePath == nil {
		return newDomainError(CodeBindingConflict, "block project volumes require devicePath")
	}
	switch volume.AccessMode {
	case model.ProjectVolumeAccessReadWriteOnce, model.ProjectVolumeAccessReadWriteOncePod:
		mount.Exclusive = true
	case model.ProjectVolumeAccessReadOnlyMany:
		if !mount.ReadOnly {
			return newDomainError(CodeBindingConflict, "ReadOnlyMany project volumes require readOnly mounts")
		}
		mount.Exclusive = false
	case model.ProjectVolumeAccessReadWriteMany:
		mount.Exclusive = false
	default:
		return newDomainError(CodeBindingConflict, "project volume access mode is unsupported")
	}
	return nil
}

func mountConflicts(existing []model.DeploymentVolumeMount, input ReserveMountInput) bool {
	for _, mount := range existing {
		if mount.LogicalName == input.LogicalName {
			return true
		}
		if input.MountPath != "" && mount.MountPath != nil && pathsOverlap(*mount.MountPath, input.MountPath) {
			return true
		}
		if input.DevicePath != "" && mount.DevicePath != nil && *mount.DevicePath == input.DevicePath {
			return true
		}
	}
	return false
}

func normalizeVolumeTransferListOptions(options VolumeTransferListOptions) (VolumeTransferListOptions, error) {
	if options.Page < 1 {
		options.Page = 1
	}
	if options.PageSize < 1 {
		options.PageSize = DefaultPageSize
	}
	if options.PageSize > MaxPageSize {
		options.PageSize = MaxPageSize
	}
	options.SortBy = strings.TrimSpace(options.SortBy)
	if options.SortBy == "" {
		options.SortBy = "createdAt"
	}
	if _, ok := volumeTransferSortColumns[options.SortBy]; !ok {
		return VolumeTransferListOptions{}, newDomainError(CodePaginationSortByInvalid, "unsupported volume transfer sort field")
	}
	options.SortOrder = strings.ToLower(strings.TrimSpace(options.SortOrder))
	if options.SortOrder == "" {
		options.SortOrder = "desc"
	}
	if options.SortOrder != "asc" && options.SortOrder != "desc" {
		return VolumeTransferListOptions{}, newDomainError(CodePaginationOrderInvalid, "sort order must be asc or desc")
	}
	options.Direction = strings.TrimSpace(options.Direction)
	options.State = strings.TrimSpace(options.State)
	options.VolumeID = strings.TrimSpace(options.VolumeID)
	options.CreatedBy = strings.TrimSpace(options.CreatedBy)
	if options.Direction != "" && !validTransferDirection(options.Direction) {
		return VolumeTransferListOptions{}, newDomainError(CodeInvalidInput, "volume transfer direction filter is invalid")
	}
	if options.State != "" && !validTransferState(options.State) {
		return VolumeTransferListOptions{}, newDomainError(CodeInvalidInput, "volume transfer state filter is invalid")
	}
	return options, nil
}

func normalizeCreateVolumeTransferInput(input CreateVolumeTransferInput) CreateVolumeTransferInput {
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	input.ProjectVolumeID = strings.TrimSpace(input.ProjectVolumeID)
	input.Direction = strings.TrimSpace(input.Direction)
	input.Format = strings.TrimSpace(input.Format)
	input.ConsistencyMode = strings.TrimSpace(input.ConsistencyMode)
	input.ObjectKey = strings.TrimSpace(input.ObjectKey)
	input.MultipartUploadID = strings.TrimSpace(input.MultipartUploadID)
	input.SourceFilename = strings.TrimSpace(input.SourceFilename)
	input.SHA256 = strings.ToLower(strings.TrimSpace(input.SHA256))
	input.ActorID = strings.TrimSpace(input.ActorID)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	return input
}

func validateCreateVolumeTransferInput(input CreateVolumeTransferInput) error {
	if input.ProjectID == "" || input.ProjectVolumeID == "" || input.ActorID == "" {
		return newDomainError(CodeInvalidInput, "project, project volume, and actor are required")
	}
	if !validTransferDirection(input.Direction) || !oneOf(input.Format, model.VolumeTransferFormatTarGZ, model.VolumeTransferFormatRawZST) {
		return newDomainError(CodeInvalidInput, "volume transfer direction or format is invalid")
	}
	if !oneOf(input.ConsistencyMode, model.VolumeTransferConsistencySnapshot, model.VolumeTransferConsistencyLive, model.VolumeTransferConsistencyUnmounted) {
		return newDomainError(CodeInvalidInput, "volume transfer consistency mode is invalid")
	}
	if input.Direction == model.VolumeTransferDirectionImport && !input.StartUploading && !input.VerifiedObject {
		return newDomainError(CodeInvalidInput, "external volume imports must begin in uploading state")
	}
	if input.Direction == model.VolumeTransferDirectionExport && (input.StartUploading || input.VerifiedObject) {
		return newDomainError(CodeInvalidInput, "volume exports cannot begin in uploading state")
	}
	if input.Direction == model.VolumeTransferDirectionImport && input.ExpectedBytes <= 0 {
		return newDomainError(CodeInvalidInput, "volume import content length must be positive")
	}
	if input.ExpectedBytes < 0 || !input.ExpiresAt.After(timeNowUTC()) {
		return newDomainError(CodeInvalidInput, "volume transfer size or expiry is invalid")
	}
	if input.StartUploading && input.MultipartUploadID == "" {
		return newDomainError(CodeInvalidInput, "uploading volume transfers require a multipart upload")
	}
	if input.VerifiedObject && (input.ObjectKey == "" || input.SHA256 == "" || !validSHA256(input.SHA256)) {
		return newDomainError(CodeInvalidInput, "verified volume transfer objects require a valid object reference and checksum")
	}
	if input.SHA256 != "" && !validSHA256(input.SHA256) {
		return newDomainError(CodeTransferChecksumInvalid, "volume transfer checksum is invalid")
	}
	if input.IdempotencyKey != "" && (len(input.IdempotencyKey) < 8 || len(input.IdempotencyKey) > 160) {
		return newDomainError(CodeInvalidInput, "volume transfer idempotency key must contain 8 to 160 characters")
	}
	if len(input.SourceFilename) > 255 || len(input.ObjectKey) > 1024 || len(input.MultipartUploadID) > 1024 {
		return newDomainError(CodeInvalidInput, "volume transfer metadata is too long")
	}
	return nil
}

func validateVolumeForTransfer(volume model.ProjectVolume, input CreateVolumeTransferInput) error {
	if input.Direction == model.VolumeTransferDirectionImport {
		if volume.SourceKind != model.ProjectVolumeSourceArchiveImport || volume.LifecycleState != model.ProjectVolumeLifecycleProvisioning {
			return newDomainError(CodeTransferStateConflict, "volume import requires a provisioning archive-import volume")
		}
	} else if volume.LifecycleState != model.ProjectVolumeLifecycleReady {
		return newDomainError(CodeTransferStateConflict, "volume export requires a ready project volume")
	}
	if volume.VolumeMode == model.ProjectVolumeModeFilesystem && input.Format != model.VolumeTransferFormatTarGZ {
		return newDomainError(CodeTransferFormatMismatch, "filesystem project volumes use tar_gz transfers")
	}
	if volume.VolumeMode == model.ProjectVolumeModeBlock && input.Format != model.VolumeTransferFormatRawZST {
		return newDomainError(CodeTransferFormatMismatch, "block project volumes use raw_zst transfers")
	}
	return nil
}

func validDisplayName(value string) bool {
	length := utf8.RuneCountInString(strings.TrimSpace(value))
	return length >= 1 && length <= 128
}

func validOwnershipMode(value string) bool {
	return oneOf(value, model.ProjectVolumeOwnershipManaged, model.ProjectVolumeOwnershipReferenced)
}

func validSourceKind(value string) bool {
	return oneOf(value, model.ProjectVolumeSourceBlank, model.ProjectVolumeSourceManaged, model.ProjectVolumeSourceRetained,
		model.ProjectVolumeSourceArchiveImport, model.ProjectVolumeSourceSnapshotRestore, model.ProjectVolumeSourceExistingClaim)
}

func validLifecycleState(value string) bool {
	return oneOf(value, model.ProjectVolumeLifecycleProvisioning, model.ProjectVolumeLifecycleReady,
		model.ProjectVolumeLifecycleDeleting, model.ProjectVolumeLifecycleError)
}

func validAccessMode(value string) bool {
	return oneOf(value, model.ProjectVolumeAccessReadWriteOnce, model.ProjectVolumeAccessReadWriteOncePod,
		model.ProjectVolumeAccessReadOnlyMany, model.ProjectVolumeAccessReadWriteMany)
}

func validVolumeMode(value string) bool {
	return oneOf(value, model.ProjectVolumeModeFilesystem, model.ProjectVolumeModeBlock)
}

func validTransferDirection(value string) bool {
	return oneOf(value, model.VolumeTransferDirectionImport, model.VolumeTransferDirectionExport)
}

func validTransferState(value string) bool {
	return oneOf(value, model.VolumeTransferStateCreated, model.VolumeTransferStateUploading, model.VolumeTransferStateQueued,
		model.VolumeTransferStateRunning, model.VolumeTransferStateSucceeded, model.VolumeTransferStateFailed,
		model.VolumeTransferStateCancelled, model.VolumeTransferStateExpired)
}

func validSHA256(value string) bool {
	return sha256Pattern.MatchString(value)
}

func cleanAbsolutePath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || !strings.HasPrefix(value, "/") {
		return ""
	}
	cleaned := path.Clean(value)
	if cleaned == "/" || cleaned == "." {
		return ""
	}
	return cleaned
}

func pathsOverlap(left, right string) bool {
	left = path.Clean(left)
	right = path.Clean(right)
	return left == right || strings.HasPrefix(left, right+"/") || strings.HasPrefix(right, left+"/")
}

func safeTransferPartEnd(offset, size int64) (int64, bool) {
	if offset < 0 || size <= 0 || offset > math.MaxInt64-size {
		return 0, false
	}
	return offset + size, true
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

var timeNowUTC = func() time.Time { return time.Now().UTC() }
