package volumetransferapi

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"sort"
	"strings"
	"syscall"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/provider/volumestore"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/LiteyukiStudio/devops/internal/volume"
)

func (service *Service) HeadImport(ctx context.Context, projectID, transferID string, actor Actor) (offset, length, chunkSize int64, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume_transfer_api", "import.head")
	defer func() { end(err) }()
	transfer, err := service.getPublicTransfer(ctx, projectID, transferID, actor)
	if err != nil {
		return 0, 0, 0, err
	}
	if transfer.Direction != model.VolumeTransferDirectionImport {
		return 0, 0, 0, domainError(volume.CodeTransferStateConflict, "volume transfer is not an import", nil)
	}
	if transfer.State != model.VolumeTransferStateUploading {
		return 0, 0, 0, domainError(volume.CodeTransferStateConflict, "volume import does not accept uploads", nil)
	}
	if !transfer.ExpiresAt.After(service.now()) {
		return 0, 0, 0, domainError(volume.CodeTransferExpired, "volume import upload has expired", nil)
	}
	parts, err := service.allParts(ctx, transfer.ID)
	if err != nil {
		return 0, 0, 0, err
	}
	return partsOffset(parts), transfer.ExpectedBytes, service.ChunkSizeForTransfer(transfer), nil
}

func (service *Service) WriteImportPart(ctx context.Context, projectID, transferID string, actor Actor, offset int64, checksum string, body io.Reader, size int64) (nextOffset, chunkSize int64, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume_transfer_api", "import.write_part")
	defer func() { end(err) }()
	transfer, err := service.getPublicTransfer(ctx, projectID, transferID, actor)
	if err != nil {
		return 0, 0, err
	}
	if transfer.Direction != model.VolumeTransferDirectionImport || transfer.State != model.VolumeTransferStateUploading {
		return 0, 0, domainError(volume.CodeTransferStateConflict, "volume import does not accept uploads", nil)
	}
	if !transfer.ExpiresAt.After(service.now()) {
		return 0, 0, domainError(volume.CodeTransferExpired, "volume import upload has expired", nil)
	}
	return service.writePart(ctx, transfer, offset, checksum, body, size)
}

func (service *Service) CompleteImport(ctx context.Context, projectID, transferID string, actor Actor, contentLength int64, checksum string) (result model.VolumeTransfer, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume_transfer_api", "import.complete")
	defer func() { end(err) }()
	transfer, err := service.getPublicTransfer(ctx, projectID, transferID, actor)
	if err != nil {
		return model.VolumeTransfer{}, err
	}
	checksum = strings.ToLower(strings.TrimSpace(checksum))
	if !validSHA256(checksum) || contentLength < 1 {
		return model.VolumeTransfer{}, domainError(volume.CodeTransferChecksumInvalid, "volume import checksum is invalid", nil)
	}
	if transfer.State != model.VolumeTransferStateUploading {
		if (transfer.State == model.VolumeTransferStateQueued || transfer.State == model.VolumeTransferStateRunning || transfer.State == model.VolumeTransferStateSucceeded) && transfer.ExpectedBytes == contentLength && transfer.SHA256 == checksum {
			return transfer, nil
		}
		return model.VolumeTransfer{}, domainError(volume.CodeTransferStateConflict, "volume import upload state changed", nil)
	}
	if !transfer.ExpiresAt.After(service.now()) {
		return model.VolumeTransfer{}, domainError(volume.CodeTransferExpired, "volume import upload has expired", nil)
	}
	if transfer.ExpectedBytes != contentLength {
		return model.VolumeTransfer{}, domainError(volume.CodeTransferProgressInvalid, "volume import content length does not match", nil)
	}
	if transfer.SHA256 != "" && transfer.SHA256 != checksum {
		return model.VolumeTransfer{}, domainError(volume.CodeTransferChecksumMismatch, "volume import checksum does not match the declared checksum", nil)
	}
	parts, err := service.allParts(ctx, transfer.ID)
	if err != nil {
		return model.VolumeTransfer{}, err
	}
	if partsOffset(parts) != contentLength {
		return model.VolumeTransfer{}, domainError(volume.CodeTransferOffsetMismatch, "volume import upload is incomplete", nil)
	}
	completedParts := make([]volumestore.CompletedPart, 0, len(parts))
	for _, part := range parts {
		completedParts = append(completedParts, volumestore.CompletedPart{PartNumber: part.PartNumber, ETag: part.ETag})
	}
	if err := service.store.CompleteMultipart(ctx, transfer.ObjectKey, transfer.MultipartUploadID, completedParts); err != nil {
		// A completion request may time out after the object store committed it.
		// A successful authoritative Head allows the retry to continue safely.
		info, headErr := service.store.Head(ctx, transfer.ObjectKey)
		if headErr != nil || info.Size != contentLength {
			return model.VolumeTransfer{}, storeError("complete volume import content", err)
		}
	}
	if _, err := service.verifyStoredObjectSize(ctx, transfer, contentLength); err != nil {
		if volume.ErrorCode(err) == volume.CodeTransferChecksumMismatch {
			_ = service.store.Delete(ctx, transfer.ObjectKey)
			_, _ = service.volumes.FailVolumeTransferExecution(ctx, transfer.ProjectID, transfer.ID,
				volume.CodeTransferChecksumMismatch, "completed import object size does not match")
		}
		return model.VolumeTransfer{}, err
	}
	result, err = service.volumes.CompleteVolumeTransferUpload(ctx, projectID, transferID, contentLength, checksum)
	if volume.ErrorCode(err) == volume.CodeTransferStateConflict {
		current, getErr := service.volumes.GetVolumeTransfer(ctx, projectID, transferID)
		if getErr == nil && (current.State == model.VolumeTransferStateQueued || current.State == model.VolumeTransferStateRunning || current.State == model.VolumeTransferStateSucceeded) &&
			current.ExpectedBytes == contentLength && constantTimeTextEqual(current.SHA256, checksum) {
			return current, nil
		}
	}
	return result, err
}

func (service *Service) writePart(ctx context.Context, transfer model.VolumeTransfer, offset int64, checksum string, body io.Reader, size int64) (int64, int64, error) {
	chunkSize := service.ChunkSizeForTransfer(transfer)
	if offset < 0 || size < 1 || size > chunkSize || body == nil {
		return 0, chunkSize, domainError(volume.CodeInvalidInput, "volume transfer part is invalid", nil)
	}
	checksumHex, err := decodeChunkChecksum(checksum)
	if err != nil {
		return 0, chunkSize, err
	}
	if size > service.maxBytes || offset > service.maxBytes-size {
		return 0, chunkSize, domainError(volume.CodeTransferProgressInvalid, "volume transfer part exceeds the expected length", nil)
	}
	if transfer.Direction == model.VolumeTransferDirectionImport {
		remaining := transfer.ExpectedBytes - offset
		if remaining < 1 {
			return 0, chunkSize, domainError(volume.CodeTransferOffsetMismatch, "volume transfer part offset does not match the server offset", nil)
		}
		expectedSize := min(chunkSize, remaining)
		if size != expectedSize {
			return 0, chunkSize, domainError(volume.CodeTransferProgressInvalid, "volume transfer part does not use the required chunk size", nil)
		}
	}
	if err := service.volumes.PreflightVolumeTransferPart(ctx, transfer.ProjectID, transfer.ID, model.VolumeTransferPart{
		Offset: offset, Size: size, SHA256: checksumHex,
	}); err != nil {
		return 0, chunkSize, err
	}

	partFile, cleanup, err := service.spoolPart(ctx, body, size, checksumHex)
	if err != nil {
		return 0, chunkSize, err
	}
	defer cleanup()

	_, nextOffset, err := service.volumes.WriteVolumeTransferPart(ctx, transfer.ProjectID, transfer.ID, model.VolumeTransferPart{
		Offset: offset, Size: size, SHA256: checksumHex,
	}, func(writeCtx context.Context, partNumber int) (string, error) {
		if _, seekErr := partFile.Seek(0, io.SeekStart); seekErr != nil {
			return "", storeError("prepare volume transfer part", seekErr)
		}
		counted := &countingReader{reader: io.LimitReader(partFile, size)}
		etag, writeErr := service.store.WritePart(writeCtx, transfer.ObjectKey, transfer.MultipartUploadID, partNumber, counted, size)
		if writeErr != nil {
			return "", storeError("write volume transfer part", writeErr)
		}
		if counted.count != size {
			return "", storeError("write volume transfer part", io.ErrUnexpectedEOF)
		}
		return etag, nil
	})
	return nextOffset, chunkSize, err
}

func (service *Service) spoolPart(ctx context.Context, body io.Reader, size int64, checksumHex string) (*os.File, func(), error) {
	if service.spoolInitErr != nil {
		return nil, nil, domainError(volume.CodeTransferSpoolUnavailable, "volume transfer spool is unavailable", service.spoolInitErr)
	}
	if !service.spoolBudget.tryAcquire(size) {
		return nil, nil, domainError(volume.CodeTransferSpoolBusy, "volume transfer spool byte budget is busy", nil)
	}
	releaseBudget := true
	defer func() {
		if releaseBudget {
			service.spoolBudget.release(size)
		}
	}()
	available, err := service.spoolAvailableBytes(service.tempDir)
	if err != nil {
		return nil, nil, domainError(volume.CodeTransferSpoolUnavailable, "volume transfer spool capacity is unavailable", err)
	}
	if available < size || available-size < service.spoolMinFreeBytes {
		return nil, nil, domainError(volume.CodeTransferSpoolInsufficient, "volume transfer spool has insufficient free space", nil)
	}
	partFile, err := os.CreateTemp(service.tempDir, spoolPartPrefix+"*")
	if err != nil {
		if errors.Is(err, syscall.ENOSPC) || errors.Is(err, syscall.EDQUOT) {
			return nil, nil, domainError(volume.CodeTransferSpoolInsufficient, "create temporary volume transfer part failed", err)
		}
		return nil, nil, domainError(volume.CodeTransferSpoolUnavailable, "create temporary volume transfer part failed", err)
	}
	cleanup := func() {
		_ = partFile.Close()
		_ = os.Remove(partFile.Name())
		service.spoolBudget.release(size)
	}
	if err := partFile.Chmod(0o600); err != nil {
		cleanup()
		return nil, nil, storeError("protect temporary volume transfer part", err)
	}
	if closer, ok := body.(io.Closer); ok {
		stop := context.AfterFunc(ctx, func() { _ = closer.Close() })
		defer stop()
	}
	hasher := sha256.New()
	reader := &contextReader{ctx: ctx, reader: io.LimitReader(body, size+1)}
	written, copyErr := io.CopyBuffer(io.MultiWriter(partFile, hasher), reader, make([]byte, 1024*1024))
	if copyErr != nil {
		cleanup()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, nil, ctxErr
		}
		return nil, nil, domainError(volume.CodeTransferProgressInvalid, "read volume transfer part failed", copyErr)
	}
	if err := ctx.Err(); err != nil {
		cleanup()
		return nil, nil, err
	}
	if written != size {
		cleanup()
		return nil, nil, domainError(volume.CodeTransferProgressInvalid, "volume transfer part length does not match", nil)
	}
	if actualChecksum := hex.EncodeToString(hasher.Sum(nil)); actualChecksum != checksumHex {
		cleanup()
		return nil, nil, domainError(volume.CodeTransferChunkChecksumMismatch, "volume transfer chunk checksum does not match", nil)
	}
	if _, err := partFile.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return nil, nil, storeError("prepare temporary volume transfer part", err)
	}
	releaseBudget = false
	return partFile, cleanup, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader *contextReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	count, err := reader.reader.Read(buffer)
	if ctxErr := reader.ctx.Err(); ctxErr != nil {
		return count, ctxErr
	}
	return count, err
}

func (service *Service) allParts(ctx context.Context, transferID string) ([]model.VolumeTransferPart, error) {
	parts := make([]model.VolumeTransferPart, 0)
	for page := 1; page <= maxPartCount/volume.MaxPageSize; page++ {
		items, total, err := service.volumes.ListVolumeTransferParts(ctx, transferID, page, volume.MaxPageSize)
		if err != nil {
			return nil, err
		}
		parts = append(parts, items...)
		if int64(len(parts)) >= total {
			sort.Slice(parts, func(i, j int) bool { return parts[i].PartNumber < parts[j].PartNumber })
			return parts, nil
		}
	}
	return nil, domainError(volume.CodeTransferProgressInvalid, "volume transfer contains too many parts", nil)
}

func partsOffset(parts []model.VolumeTransferPart) int64 {
	var offset int64
	for _, part := range parts {
		if end := part.Offset + part.Size; end > offset {
			offset = end
		}
	}
	return offset
}

func decodeChunkChecksum(value string) (string, error) {
	raw, err := base64.StdEncoding.Strict().DecodeString(strings.TrimSpace(value))
	if err != nil || len(raw) != sha256.Size {
		return "", domainError(volume.CodeTransferChecksumInvalid, "volume transfer chunk checksum is invalid", nil)
	}
	return hex.EncodeToString(raw), nil
}

type countingReader struct {
	reader io.Reader
	count  int64
}

func (reader *countingReader) Read(buffer []byte) (int, error) {
	count, err := reader.reader.Read(buffer)
	reader.count += int64(count)
	return count, err
}

func (service *Service) getPublicTransfer(ctx context.Context, projectID, transferID string, actor Actor) (model.VolumeTransfer, error) {
	if err := service.validate(); err != nil {
		return model.VolumeTransfer{}, err
	}
	transfer, err := service.volumes.GetVolumeTransfer(ctx, strings.TrimSpace(projectID), strings.TrimSpace(transferID))
	if err != nil {
		return model.VolumeTransfer{}, err
	}
	if actor.UserID == "" || (transfer.ActorID != actor.UserID && !actor.CanManage) {
		return model.VolumeTransfer{}, domainError(volume.CodeTransferNotFound, "volume transfer was not found", nil)
	}
	return transfer, nil
}
