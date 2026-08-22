package volume

import "errors"

const (
	CodeInvalidInput            = "volume.invalid_input"
	CodeNotFound                = "volume.not_found"
	CodeClusterUnavailable      = "volume.cluster_unavailable"
	CodeClaimNotFound           = "volume.claim_not_found"
	CodeOwnershipConflict       = "volume.ownership_conflict"
	CodeIncompatibleCluster     = "volume.incompatible_cluster"
	CodeBindingConflict         = "volume.binding_conflict"
	CodeInUse                   = "volume.in_use"
	CodeCapacityShrinkForbidden = "volume.capacity_shrink_forbidden"
	CodeRevisionConflict        = "volume.revision_conflict"
	CodeStateConflict           = "volume.state_conflict"
	CodeIdempotencyConflict     = "volume.idempotency_conflict"
	CodeNameConflict            = "volume.name_conflict"
	CodeClaimConflict           = "volume.claim_conflict"
	CodeClaimSpecConflict       = "volume.claim_spec_conflict"
	CodeExpansionUnsupported    = "volume.expansion_unsupported"
	CodeSnapshotUnsupported     = "volume.snapshot_unsupported"
	CodeSnapshotNotFound        = "volume.snapshot_not_found"
	CodeSnapshotRequired        = "volume.snapshot_required"
	CodeDeletionPending         = "volume.deletion_pending"
	CodeTaskEnqueueFailed       = "volume.task_enqueue_failed"
	CodeQuotaExceeded           = "volume.quota_exceeded"
	CodeQuotaUnavailable        = "volume.quota_unavailable"

	CodeTransferNotFound             = "volume_transfer.not_found"
	CodeTransferUnavailable          = "volume_transfer.unavailable"
	CodeTransferStateConflict        = "volume_transfer.state_conflict"
	CodeTransferFormatMismatch       = "volume_transfer.format_mismatch"
	CodeTransferProgressInvalid      = "volume_transfer.progress_invalid"
	CodeTransferChecksumInvalid      = "volume_transfer.checksum_invalid"
	CodeTransferChecksumMismatch     = "volume_transfer.checksum_mismatch"
	CodeTransferArchiveUnsafe        = "volume_transfer.archive_unsafe"
	CodeTransferCapacityExceeded     = "volume_transfer.capacity_exceeded"
	CodeTransferExpired              = "volume_transfer.expired"
	CodeTransferDownloadUnauthorized = "volume_transfer.download_unauthorized"
	CodeTransferFormatUnsupported    = "volume_transfer.format_unsupported"
	CodeTransferJobFailed            = "volume_transfer.job_failed"
	CodePaginationSortByInvalid      = "pagination.sort_by_invalid"
	CodePaginationOrderInvalid       = "pagination.sort_order_invalid"
)

var (
	ErrExistingClaimNotFound          = errors.New("existing project volume claim was not found")
	ErrExistingClaimOwnershipConflict = errors.New("existing project volume claim ownership conflicts")
	ErrExistingClaimSpecConflict      = errors.New("existing project volume claim specification conflicts")
)

// DomainError exposes a stable machine-readable code while retaining an
// optional internal cause for development diagnostics. Error intentionally
// returns only the public message; raw dependency errors must not leak from
// production API responses.
type DomainError struct {
	Code    string
	Message string
	Cause   error
}

func (err *DomainError) Error() string {
	if err == nil || err.Message == "" {
		return "volume operation failed"
	}
	return err.Message
}

func (err *DomainError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

func newDomainError(code, message string, cause ...error) error {
	var wrapped error
	if len(cause) > 0 {
		wrapped = cause[0]
	}
	return &DomainError{Code: code, Message: message, Cause: wrapped}
}

func ErrorCode(err error) string {
	var domainErr *DomainError
	if errors.As(err, &domainErr) {
		return domainErr.Code
	}
	return ""
}
