package volumetransferapi

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"io"
	"regexp"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/provider/volumestore"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/LiteyukiStudio/devops/internal/volume"
)

var stableWorkerErrorCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(?:\.[a-z][a-z0-9_]*)+$`)

func (service *Service) InternalHead(ctx context.Context, transferID, rawToken string) (info ContentInfo, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume_transfer_api", "internal.head")
	defer func() { end(err) }()
	transfer, err := service.authenticateInternal(ctx, transferID, rawToken, model.VolumeTransferStateRunning)
	if err != nil {
		return ContentInfo{}, err
	}
	if transfer.Direction == model.VolumeTransferDirectionExport {
		parts, err := service.allParts(ctx, transfer.ID)
		if err != nil {
			return ContentInfo{}, err
		}
		return ContentInfo{
			Offset: partsOffset(parts), Size: transfer.ExpectedBytes,
			ChunkSize: service.ChunkSizeForTransfer(transfer),
		}, nil
	}
	object, err := service.store.Head(ctx, transfer.ObjectKey)
	if err != nil {
		return ContentInfo{}, storeError("read internal volume transfer content metadata", err)
	}
	if object.Size != transfer.ExpectedBytes {
		return ContentInfo{}, domainError(volume.CodeTransferChecksumMismatch, "volume transfer content length does not match", nil)
	}
	return ContentInfo{Size: object.Size, ChunkSize: service.ChunkSizeForTransfer(transfer), ETag: object.ETag}, nil
}

func (service *Service) InternalWritePart(ctx context.Context, transferID, rawToken string, offset int64, checksum string, body io.Reader, size int64) (nextOffset, chunkSize int64, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume_transfer_api", "internal.write_part")
	defer func() { end(err) }()
	transfer, err := service.authenticateInternal(ctx, transferID, rawToken, model.VolumeTransferStateRunning)
	if err != nil {
		return 0, 0, err
	}
	if transfer.Direction != model.VolumeTransferDirectionExport || transfer.MultipartUploadID == "" {
		return 0, 0, domainError(volume.CodeTransferStateConflict, "volume transfer does not accept exported content", nil)
	}
	return service.writePart(ctx, transfer, offset, checksum, body, size)
}

func (service *Service) InternalOpen(ctx context.Context, transferID, rawToken, rangeHeader string) (download Download, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume_transfer_api", "internal.open")
	defer func() { end(err) }()
	transfer, err := service.authenticateInternal(ctx, transferID, rawToken, model.VolumeTransferStateRunning)
	if err != nil {
		return Download{}, err
	}
	if transfer.Direction != model.VolumeTransferDirectionImport {
		return Download{}, domainError(volume.CodeTransferStateConflict, "volume transfer does not expose import content", nil)
	}
	return service.openStoredContent(ctx, transfer, rangeHeader)
}

func (service *Service) InternalProgress(ctx context.Context, transferID, rawToken string, progress Progress) (err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume_transfer_api", "internal.progress")
	defer func() { end(err) }()
	transfer, err := service.authenticateInternal(ctx, transferID, rawToken, model.VolumeTransferStateRunning)
	if err != nil {
		return err
	}
	if strings.TrimSpace(progress.ExpectedState) != model.VolumeTransferStateRunning {
		return domainError(volume.CodeTransferStateConflict, "volume transfer execution state changed", nil)
	}
	_, err = service.volumes.UpdateVolumeTransferProgress(ctx, transfer.ProjectID, transfer.ID, volume.TransferProgress{
		TransferredBytes: progress.TransferredBytes,
		ProcessedFiles:   progress.ProcessedFiles,
		Phase:            strings.TrimSpace(progress.Stage),
	})
	return err
}

func (service *Service) InternalComplete(ctx context.Context, transferID, rawToken string, completion Completion) (result model.VolumeTransfer, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume_transfer_api", "internal.complete")
	defer func() { end(err) }()
	completion.ExpectedState = strings.TrimSpace(completion.ExpectedState)
	completion.SHA256 = strings.ToLower(strings.TrimSpace(completion.SHA256))
	completion.DataSHA256 = strings.ToLower(strings.TrimSpace(completion.DataSHA256))
	if completion.ExpectedState != model.VolumeTransferStateRunning || completion.TransferredBytes < 1 ||
		!validSHA256(completion.SHA256) || completion.LogicalBytes < 0 ||
		(completion.LogicalBytes == 0) != (completion.DataSHA256 == "") ||
		(completion.DataSHA256 != "" && !validSHA256(completion.DataSHA256)) {
		return model.VolumeTransfer{}, domainError(volume.CodeTransferChecksumInvalid, "volume transfer completion metadata is invalid", nil)
	}
	transfer, err := service.authenticateInternal(ctx, transferID, rawToken,
		model.VolumeTransferStateRunning, model.VolumeTransferStateSucceeded)
	if err != nil {
		return model.VolumeTransfer{}, err
	}
	if transfer.State == model.VolumeTransferStateSucceeded {
		if transfer.TransferredBytes == completion.TransferredBytes && constantTimeTextEqual(transfer.SHA256, completion.SHA256) &&
			transfer.LogicalBytes == completion.LogicalBytes && constantTimeTextEqual(transfer.DataSHA256, completion.DataSHA256) {
			return transfer, nil
		}
		return model.VolumeTransfer{}, domainError(volume.CodeTransferStateConflict, "volume transfer completion differs from the committed result", nil)
	}
	if completion.TransferredBytes > service.maxBytes {
		return model.VolumeTransfer{}, domainError(volume.CodeTransferCapacityExceeded, "volume transfer content exceeds the configured limit", nil)
	}
	if transfer.Format == model.VolumeTransferFormatRawZST {
		if completion.LogicalBytes < 1 || !validSHA256(completion.DataSHA256) {
			return model.VolumeTransfer{}, domainError(volume.CodeTransferChecksumInvalid, "raw volume transfer data digest is required", nil)
		}
	} else if completion.LogicalBytes != 0 || completion.DataSHA256 != "" {
		return model.VolumeTransfer{}, domainError(volume.CodeTransferChecksumInvalid, "filesystem transfer cannot commit a raw data digest", nil)
	}

	if transfer.Direction == model.VolumeTransferDirectionExport {
		if err = service.completeInternalExport(ctx, transfer, completion); err != nil {
			return model.VolumeTransfer{}, err
		}
	} else {
		if transfer.ExpectedBytes != completion.TransferredBytes || !constantTimeTextEqual(transfer.SHA256, completion.SHA256) {
			return model.VolumeTransfer{}, domainError(volume.CodeTransferChecksumMismatch, "volume transfer result does not match the verified import object", nil)
		}
		// The import Job has just streamed the full object and computed SHA-256;
		// the callback compares that observed digest with the persisted upload
		// declaration above. Re-reading the object here would synchronously scan
		// large archives a second time.
		if _, err = service.verifyStoredObjectSize(ctx, transfer, completion.TransferredBytes); err != nil {
			return model.VolumeTransfer{}, err
		}
	}
	return service.volumes.ReportVolumeTransferCompletion(ctx, transfer.ProjectID, transfer.ID, volume.TransferCompletion{
		ExpectedState: completion.ExpectedState, TransferredBytes: completion.TransferredBytes,
		SHA256: completion.SHA256, LogicalBytes: completion.LogicalBytes, DataSHA256: completion.DataSHA256,
	})
}

func (service *Service) completeInternalExport(ctx context.Context, transfer model.VolumeTransfer, completion Completion) error {
	parts, err := service.allParts(ctx, transfer.ID)
	if err != nil {
		return err
	}
	if partsOffset(parts) != completion.TransferredBytes || len(parts) == 0 {
		return domainError(volume.CodeTransferProgressInvalid, "volume export upload is incomplete", nil)
	}
	completedParts := make([]volumestore.CompletedPart, 0, len(parts))
	for _, part := range parts {
		completedParts = append(completedParts, volumestore.CompletedPart{PartNumber: part.PartNumber, ETag: part.ETag})
	}
	if err := service.store.CompleteMultipart(ctx, transfer.ObjectKey, transfer.MultipartUploadID, completedParts); err != nil {
		// Object stores may commit a multipart upload before the API observes a
		// timeout. Authoritative size and full checksum verification below make
		// completion retries safe without trusting the dependency error text.
		info, headErr := service.store.Head(ctx, transfer.ObjectKey)
		if headErr != nil || info.Size != completion.TransferredBytes {
			return storeError("complete volume export content", err)
		}
	}
	// Export content is hashed by the trusted transfer Job while it is streamed
	// through per-part checksum validation. Completion only needs an authoritative
	// object-size check; clients verify the published full digest on download.
	_, err = service.verifyStoredObjectSize(ctx, transfer, completion.TransferredBytes)
	return err
}

func (service *Service) InternalFail(ctx context.Context, transferID, rawToken string, failure Failure) (result model.VolumeTransfer, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume_transfer_api", "internal.fail")
	defer func() { end(err) }()
	failure.ExpectedState = strings.TrimSpace(failure.ExpectedState)
	failure.ErrorCode = strings.TrimSpace(failure.ErrorCode)
	if failure.ExpectedState != model.VolumeTransferStateRunning || !validWorkerErrorCode(failure.ErrorCode) || len(failure.Diagnostic) > 4096 {
		return model.VolumeTransfer{}, domainError(volume.CodeInvalidInput, "volume transfer failure report is invalid", nil)
	}
	transfer, err := service.authenticateInternal(ctx, transferID, rawToken,
		model.VolumeTransferStateRunning, model.VolumeTransferStateFailed)
	if err != nil {
		return model.VolumeTransfer{}, err
	}
	if transfer.State == model.VolumeTransferStateFailed {
		if transfer.LastErrorCode == failure.ErrorCode {
			return transfer, nil
		}
		return model.VolumeTransfer{}, domainError(volume.CodeTransferStateConflict, "volume transfer failure differs from the committed result", nil)
	}
	if transfer.Direction == model.VolumeTransferDirectionExport && transfer.MultipartUploadID != "" {
		_ = service.store.AbortMultipart(ctx, transfer.ObjectKey, transfer.MultipartUploadID)
	}
	return service.volumes.FailVolumeTransferExecution(ctx, transfer.ProjectID, transfer.ID,
		failure.ErrorCode, strings.TrimSpace(failure.Diagnostic))
}

func (service *Service) authenticateInternal(ctx context.Context, transferID, rawToken string, allowedStates ...string) (model.VolumeTransfer, error) {
	if err := service.validate(); err != nil {
		return model.VolumeTransfer{}, err
	}
	transferID = strings.TrimSpace(transferID)
	rawToken = strings.TrimSpace(rawToken)
	if transferID == "" || len(rawToken) < 32 || len(rawToken) > 512 {
		return model.VolumeTransfer{}, domainError(volume.CodeTransferCallbackUnauthorized, "volume transfer callback authorization is invalid", nil)
	}
	transfer, err := service.volumes.GetVolumeTransferForMaintenance(ctx, transferID)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return model.VolumeTransfer{}, err
		}
		// Internal callback lookup intentionally does not distinguish an unknown
		// transfer from an invalid credential.
		return model.VolumeTransfer{}, domainError(volume.CodeTransferCallbackUnauthorized, "volume transfer callback authorization is invalid", nil)
	}
	if transfer.CallbackTokenExpiresAt == nil || !transfer.CallbackTokenExpiresAt.After(service.now().UTC()) ||
		!validSHA256(strings.ToLower(transfer.CallbackTokenHash)) {
		return model.VolumeTransfer{}, domainError(volume.CodeTransferCallbackUnauthorized, "volume transfer callback authorization is invalid", nil)
	}
	storedHash, decodeErr := hex.DecodeString(strings.ToLower(transfer.CallbackTokenHash))
	presentedHash := sha256.Sum256([]byte(rawToken))
	if decodeErr != nil || len(storedHash) != sha256.Size || subtle.ConstantTimeCompare(storedHash, presentedHash[:]) != 1 {
		return model.VolumeTransfer{}, domainError(volume.CodeTransferCallbackUnauthorized, "volume transfer callback authorization is invalid", nil)
	}
	for _, state := range allowedStates {
		if transfer.State == state {
			return transfer, nil
		}
	}
	return model.VolumeTransfer{}, domainError(volume.CodeTransferStateConflict, "volume transfer execution state changed", nil)
}

func validWorkerErrorCode(code string) bool {
	return len(code) <= 128 && stableWorkerErrorCodePattern.MatchString(code) &&
		(strings.HasPrefix(code, "volume_transfer.") || strings.HasPrefix(code, "volume."))
}
