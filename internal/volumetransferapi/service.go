package volumetransferapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/provider/volumestore"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/LiteyukiStudio/devops/internal/volume"
)

const (
	defaultObjectTTL  = 24 * time.Hour
	defaultTicketTTL  = time.Minute
	defaultSessionTTL = 30 * time.Minute
	defaultOrphanAge  = 24 * time.Hour
	spoolPartPrefix   = "luna-volume-transfer-part-"
	maxPartCount      = 10_000
	maxLookupPages    = 100
)

type volumeDomain interface {
	CreateProjectVolume(context.Context, volume.CreateProjectVolumeInput) (volume.CreateProjectVolumeResult, error)
	GetProjectVolume(context.Context, string, string) (model.ProjectVolume, error)
	SetProjectVolumeLifecycle(context.Context, string, string, []string, string, string, string) (model.ProjectVolume, error)
	CreateVolumeTransfer(context.Context, volume.CreateVolumeTransferInput) (model.VolumeTransfer, error)
	RetryVolumeImportTransfer(context.Context, string, volume.CreateVolumeTransferInput) (model.VolumeTransfer, error)
	GetVolumeTransfer(context.Context, string, string) (model.VolumeTransfer, error)
	GetVolumeTransferForMaintenance(context.Context, string) (model.VolumeTransfer, error)
	ListVolumeTransfers(context.Context, string, volume.VolumeTransferListOptions) (volume.VolumeTransferListResult, error)
	CompleteVolumeTransferUpload(context.Context, string, string, int64, string) (model.VolumeTransfer, error)
	TransitionVolumeTransfer(context.Context, string, string, string, string, string) (model.VolumeTransfer, error)
	UpdateVolumeTransferProgress(context.Context, string, string, volume.TransferProgress) (model.VolumeTransfer, error)
	PreflightVolumeTransferPart(context.Context, string, string, model.VolumeTransferPart) error
	WriteVolumeTransferPart(context.Context, string, string, model.VolumeTransferPart, volume.TransferPartWriter) (model.VolumeTransferPart, int64, error)
	ListVolumeTransferParts(context.Context, string, int, int) ([]model.VolumeTransferPart, int64, error)
	ReportVolumeTransferCompletion(context.Context, string, string, volume.TransferCompletion) (model.VolumeTransfer, error)
	FailVolumeTransferExecution(context.Context, string, string, string, string) (model.VolumeTransfer, error)
}

type Options struct {
	ObjectTTL           time.Duration
	MaxBytes            int64
	TempDir             string
	SpoolMaxBytes       int64
	SpoolMinFreeBytes   int64
	SpoolOrphanAge      time.Duration
	TicketTTL           time.Duration
	SessionTTL          time.Duration
	Now                 func() time.Time
	SpoolAvailableBytes func(string) (int64, error)
}

type Service struct {
	volumes             volumeDomain
	store               volumestore.Store
	tickets             TicketStore
	objectTTL           time.Duration
	maxBytes            int64
	tempDir             string
	spoolBudget         *weightedSpoolBudget
	spoolInitErr        error
	spoolMinFreeBytes   int64
	spoolAvailableBytes func(string) (int64, error)
	ticketTTL           time.Duration
	sessionTTL          time.Duration
	now                 func() time.Time
}

func NewService(volumes volumeDomain, store volumestore.Store, tickets TicketStore, options Options) *Service {
	if options.ObjectTTL <= 0 {
		options.ObjectTTL = defaultObjectTTL
	}
	if options.TicketTTL <= 0 {
		options.TicketTTL = defaultTicketTTL
	}
	if options.SessionTTL <= 0 || options.SessionTTL > defaultSessionTTL {
		options.SessionTTL = defaultSessionTTL
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if strings.TrimSpace(options.TempDir) == "" {
		options.TempDir = filepath.Join(os.TempDir(), "luna-devops-volume-transfer-spool")
	}
	if options.SpoolOrphanAge <= 0 {
		options.SpoolOrphanAge = defaultOrphanAge
	}
	if options.SpoolMaxBytes <= 0 {
		options.SpoolMaxBytes = 2 * RequiredChunkSize(options.MaxBytes)
	}
	if options.SpoolMinFreeBytes <= 0 {
		options.SpoolMinFreeBytes = MinimumChunkSize
	}
	if options.SpoolAvailableBytes == nil {
		options.SpoolAvailableBytes = availableFilesystemBytes
	}
	service := &Service{
		volumes: volumes, store: store, tickets: tickets,
		objectTTL: options.ObjectTTL, maxBytes: options.MaxBytes,
		tempDir: options.TempDir, spoolBudget: &weightedSpoolBudget{limit: options.SpoolMaxBytes},
		spoolMinFreeBytes: options.SpoolMinFreeBytes, spoolAvailableBytes: options.SpoolAvailableBytes,
		ticketTTL:  options.TicketTTL,
		sessionTTL: options.SessionTTL, now: options.Now,
	}
	service.spoolInitErr = service.initializeSpool(options.SpoolOrphanAge)
	return service
}

type weightedSpoolBudget struct {
	mu    sync.Mutex
	limit int64
	used  int64
}

func (budget *weightedSpoolBudget) tryAcquire(size int64) bool {
	if budget == nil || size < 1 {
		return false
	}
	budget.mu.Lock()
	defer budget.mu.Unlock()
	if size > budget.limit || budget.used > budget.limit-size {
		return false
	}
	budget.used += size
	return true
}

func (budget *weightedSpoolBudget) release(size int64) {
	if budget == nil || size < 1 {
		return
	}
	budget.mu.Lock()
	budget.used -= size
	if budget.used < 0 {
		budget.used = 0
	}
	budget.mu.Unlock()
}

func (service *Service) initializeSpool(orphanAge time.Duration) error {
	info, err := os.Lstat(service.tempDir)
	if errors.Is(err, os.ErrNotExist) {
		if err = os.MkdirAll(service.tempDir, 0o700); err != nil {
			return err
		}
		info, err = os.Lstat(service.tempDir)
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("volume transfer spool path is not a directory")
	}
	if err := os.Chmod(service.tempDir, 0o700); err != nil {
		return err
	}
	entries, err := os.ReadDir(service.tempDir)
	if err != nil {
		return err
	}
	cutoff := service.now().Add(-orphanAge)
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), spoolPartPrefix) || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		entryInfo, infoErr := entry.Info()
		if infoErr != nil || !entryInfo.Mode().IsRegular() || !entryInfo.ModTime().Before(cutoff) {
			continue
		}
		if removeErr := os.Remove(filepath.Join(service.tempDir, entry.Name())); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return removeErr
		}
	}
	return nil
}

func availableFilesystemBytes(path string) (int64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	if stat.Bsize <= 0 {
		return 0, errors.New("volume transfer spool filesystem reported an invalid block size")
	}
	if stat.Bavail > uint64(^uint64(0)>>1)/uint64(stat.Bsize) {
		return int64(^uint64(0) >> 1), nil
	}
	return int64(stat.Bavail) * int64(stat.Bsize), nil
}

// ChunkSizeForTransfer returns the exact part size clients must use. Export
// size is not known until the trusted Job finishes, so exports are sized from
// the configured transfer ceiling; imports use their declared content length.
func (service *Service) ChunkSizeForTransfer(transfer model.VolumeTransfer) int64 {
	expectedBytes := transfer.ExpectedBytes
	if transfer.Direction == model.VolumeTransferDirectionExport && service != nil && service.maxBytes > 0 {
		expectedBytes = service.maxBytes
	}
	return RequiredChunkSize(expectedBytes)
}

func (service *Service) CreateImport(ctx context.Context, request ImportRequest) (result ImportResult, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume_transfer_api", "import.create")
	defer func() { end(err) }()
	if err = service.validate(); err != nil {
		return ImportResult{}, err
	}
	request = normalizeImportRequest(request)
	if err = service.validateImportRequest(request); err != nil {
		return ImportResult{}, err
	}
	volumeResult, err := service.volumes.CreateProjectVolume(ctx, volume.CreateProjectVolumeInput{
		ProjectID: request.ProjectID, DisplayName: request.DisplayName, ClusterID: request.ClusterID,
		Namespace: request.Namespace, OwnershipMode: model.ProjectVolumeOwnershipManaged,
		SourceKind: model.ProjectVolumeSourceArchiveImport, CapacityRequest: request.CapacityRequest,
		CapacityBytes: request.CapacityBytes, StorageClassName: request.StorageClassName,
		AccessMode: request.AccessMode, VolumeMode: request.VolumeMode, ActorID: request.ActorID,
		IdempotencyKey: request.IdempotencyKey,
	})
	if err != nil {
		return ImportResult{}, err
	}
	objectKey := requestObjectKey(model.VolumeTransferDirectionImport, request.ProjectID, volumeResult.Volume.ID, request.IdempotencyKey)
	if volumeResult.Replayed {
		if existing, found, findErr := service.transferByObjectKey(ctx, request.ProjectID, volumeResult.Volume.ID, model.VolumeTransferDirectionImport, objectKey); findErr != nil {
			return ImportResult{}, findErr
		} else if found {
			return ImportResult{Volume: volumeResult.Volume, Transfer: existing}, nil
		}
		if volumeResult.Volume.LifecycleState == model.ProjectVolumeLifecycleError {
			volumeResult.Volume, err = service.volumes.SetProjectVolumeLifecycle(ctx, request.ProjectID, volumeResult.Volume.ID,
				[]string{model.ProjectVolumeLifecycleError}, model.ProjectVolumeLifecycleProvisioning, "", "")
			if err != nil {
				return ImportResult{}, err
			}
		}
	}

	uploadID, err := service.store.CreateMultipart(ctx, objectKey)
	if err != nil {
		_, _ = service.volumes.SetProjectVolumeLifecycle(ctx, request.ProjectID, volumeResult.Volume.ID,
			[]string{model.ProjectVolumeLifecycleProvisioning}, model.ProjectVolumeLifecycleError,
			volume.CodeTransferStoreUnavailable, "initialize volume import content failed")
		return ImportResult{}, storeError("initialize volume import content", err)
	}
	transfer, err := service.volumes.CreateVolumeTransfer(ctx, volume.CreateVolumeTransferInput{
		ProjectID: request.ProjectID, ProjectVolumeID: volumeResult.Volume.ID,
		Direction: model.VolumeTransferDirectionImport, Format: request.Format,
		ConsistencyMode: model.VolumeTransferConsistencyUnmounted, ObjectKey: objectKey,
		MultipartUploadID: uploadID, SourceFilename: request.Filename,
		ExpectedBytes: request.ContentLength, SHA256: request.SHA256, ActorID: request.ActorID,
		ExpiresAt: service.now().UTC().Add(service.objectTTL), StartUploading: true,
	})
	if err != nil {
		_ = service.store.AbortMultipart(ctx, objectKey, uploadID)
		if existing, found, findErr := service.activeTransfer(ctx, request.ProjectID, volumeResult.Volume.ID, model.VolumeTransferDirectionImport); findErr == nil && found {
			return ImportResult{Volume: volumeResult.Volume, Transfer: existing}, nil
		}
		_, _ = service.volumes.SetProjectVolumeLifecycle(ctx, request.ProjectID, volumeResult.Volume.ID,
			[]string{model.ProjectVolumeLifecycleProvisioning}, model.ProjectVolumeLifecycleError,
			volume.ErrorCode(err), "create volume import transfer failed")
		return ImportResult{}, err
	}
	return ImportResult{Volume: volumeResult.Volume, Transfer: transfer}, nil
}

func (service *Service) CreateExport(ctx context.Context, request ExportRequest) (result model.VolumeTransfer, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume_transfer_api", "export.create")
	defer func() { end(err) }()
	if err = service.validate(); err != nil {
		return model.VolumeTransfer{}, err
	}
	request = normalizeExportRequest(request)
	if request.ProjectID == "" || request.VolumeID == "" || request.ActorID == "" || len(request.IdempotencyKey) < 8 || len(request.IdempotencyKey) > 160 {
		return model.VolumeTransfer{}, domainError(volume.CodeInvalidInput, "volume export request is invalid", nil)
	}
	projectVolume, err := service.volumes.GetProjectVolume(ctx, request.ProjectID, request.VolumeID)
	if err != nil {
		return model.VolumeTransfer{}, err
	}
	consistency, err := exportConsistency(projectVolume, request.Consistency)
	if err != nil {
		return model.VolumeTransfer{}, err
	}
	if err := validateFormatForVolume(projectVolume.VolumeMode, request.Format); err != nil {
		return model.VolumeTransfer{}, err
	}
	objectKey := requestObjectKey(model.VolumeTransferDirectionExport, request.ProjectID, request.VolumeID, request.IdempotencyKey)
	if existing, found, findErr := service.transferByObjectKey(ctx, request.ProjectID, request.VolumeID, model.VolumeTransferDirectionExport, objectKey); findErr != nil {
		return model.VolumeTransfer{}, findErr
	} else if found {
		return existing, nil
	}
	if _, found, findErr := service.activeTransfer(ctx, request.ProjectID, request.VolumeID, model.VolumeTransferDirectionExport); findErr != nil {
		return model.VolumeTransfer{}, findErr
	} else if found {
		return model.VolumeTransfer{}, domainError(volume.CodeInUse, "project volume already has an active export", nil)
	}

	uploadID, err := service.store.CreateMultipart(ctx, objectKey)
	if err != nil {
		return model.VolumeTransfer{}, storeError("initialize volume export content", err)
	}
	result, err = service.volumes.CreateVolumeTransfer(ctx, volume.CreateVolumeTransferInput{
		ProjectID: request.ProjectID, ProjectVolumeID: request.VolumeID,
		Direction: model.VolumeTransferDirectionExport, Format: request.Format,
		ConsistencyMode: consistency, ObjectKey: objectKey, MultipartUploadID: uploadID,
		ActorID: request.ActorID, ExpiresAt: service.now().UTC().Add(service.objectTTL),
	})
	if err != nil {
		_ = service.store.AbortMultipart(ctx, objectKey, uploadID)
		if existing, found, findErr := service.transferByObjectKey(ctx, request.ProjectID, request.VolumeID, model.VolumeTransferDirectionExport, objectKey); findErr == nil && found {
			return existing, nil
		}
		return model.VolumeTransfer{}, err
	}
	return result, nil
}

func (service *Service) RetryTransfer(ctx context.Context, actor Actor, original model.VolumeTransfer, idempotencyKey string) (result model.VolumeTransfer, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume_transfer_api", "transfer.retry")
	defer func() { end(err) }()
	if err = service.validate(); err != nil {
		return model.VolumeTransfer{}, err
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if actor.UserID == "" || (original.ActorID != actor.UserID && !actor.CanManage) || len(idempotencyKey) < 8 || len(idempotencyKey) > 160 {
		return model.VolumeTransfer{}, domainError(volume.CodeInvalidInput, "volume transfer retry request is invalid", nil)
	}
	if original.ExpiresAt.Before(service.now()) || original.ObjectDeletedAt != nil {
		return model.VolumeTransfer{}, domainError(volume.CodeTransferExpired, "volume transfer content has expired", nil)
	}
	if !volume.IsVolumeTransferTerminal(original.State) {
		return model.VolumeTransfer{}, domainError(volume.CodeTransferStateConflict, "only terminal volume transfers can be retried", nil)
	}
	if existing, found, findErr := service.activeTransfer(ctx, original.ProjectID, original.ProjectVolumeID, original.Direction); findErr != nil {
		return model.VolumeTransfer{}, findErr
	} else if found {
		return existing, nil
	}

	if original.Direction == model.VolumeTransferDirectionExport {
		return service.CreateExport(ctx, ExportRequest{
			ProjectID: original.ProjectID, VolumeID: original.ProjectVolumeID, Format: original.Format,
			Consistency: original.ConsistencyMode, ActorID: actor.UserID, IdempotencyKey: idempotencyKey,
		})
	}
	if original.SHA256 == "" || original.ExpectedBytes < 1 {
		return model.VolumeTransfer{}, domainError(volume.CodeTransferChecksumInvalid, "verified import content is unavailable", nil)
	}
	// The retry path only checks object existence and size. The import Worker
	// streams the archive, computes its full SHA-256, and compares it with the
	// persisted digest before reporting success; doing the same multi-terabyte
	// read in an API request would duplicate work and violate the async boundary.
	if _, err := service.verifyStoredObjectSize(ctx, original, original.ExpectedBytes); err != nil {
		return model.VolumeTransfer{}, err
	}
	return service.volumes.RetryVolumeImportTransfer(ctx, original.ID, volume.CreateVolumeTransferInput{
		ProjectID: original.ProjectID, ProjectVolumeID: original.ProjectVolumeID,
		Direction: model.VolumeTransferDirectionImport, Format: original.Format,
		ConsistencyMode: original.ConsistencyMode, ObjectKey: original.ObjectKey,
		ExpectedBytes: original.ExpectedBytes, SHA256: original.SHA256,
		SourceFilename: original.SourceFilename, ActorID: actor.UserID,
		ExpiresAt: service.now().UTC().Add(service.objectTTL), IdempotencyKey: idempotencyKey, VerifiedObject: true,
	})
}

func (service *Service) activeTransfer(ctx context.Context, projectID, volumeID, direction string) (model.VolumeTransfer, bool, error) {
	result, err := service.volumes.ListVolumeTransfers(ctx, projectID, volume.VolumeTransferListOptions{
		Page: 1, PageSize: volume.MaxPageSize, SortBy: "createdAt", SortOrder: "desc", Direction: direction, VolumeID: volumeID,
	})
	if err != nil {
		return model.VolumeTransfer{}, false, err
	}
	for _, item := range result.Items {
		if !volume.IsVolumeTransferTerminal(item.State) {
			return item, true, nil
		}
	}
	return model.VolumeTransfer{}, false, nil
}

func (service *Service) transferByObjectKey(ctx context.Context, projectID, volumeID, direction, objectKey string) (model.VolumeTransfer, bool, error) {
	for page := 1; page <= maxLookupPages; page++ {
		result, err := service.volumes.ListVolumeTransfers(ctx, projectID, volume.VolumeTransferListOptions{
			Page: page, PageSize: volume.MaxPageSize, SortBy: "createdAt", SortOrder: "desc", Direction: direction, VolumeID: volumeID,
		})
		if err != nil {
			return model.VolumeTransfer{}, false, err
		}
		for _, item := range result.Items {
			if constantTimeTextEqual(item.ObjectKey, objectKey) {
				return item, true, nil
			}
		}
		if page >= result.TotalPages || len(result.Items) == 0 {
			return model.VolumeTransfer{}, false, nil
		}
	}
	return model.VolumeTransfer{}, false, domainError(volume.CodeTransferStoreUnavailable, "volume transfer idempotency history is unavailable", nil)
}

func (service *Service) validate() error {
	if service == nil || service.volumes == nil || service.store == nil || service.tickets == nil || service.maxBytes < 1 {
		return domainError(volume.CodeTransferStoreUnavailable, "volume transfer store is unavailable", nil)
	}
	return nil
}

func normalizeImportRequest(request ImportRequest) ImportRequest {
	request.ProjectID = strings.TrimSpace(request.ProjectID)
	request.Namespace = strings.TrimSpace(request.Namespace)
	request.DisplayName = strings.TrimSpace(request.DisplayName)
	request.ClusterID = strings.TrimSpace(request.ClusterID)
	request.CapacityRequest = strings.TrimSpace(request.CapacityRequest)
	request.StorageClassName = strings.TrimSpace(request.StorageClassName)
	request.AccessMode = strings.TrimSpace(request.AccessMode)
	request.VolumeMode = strings.TrimSpace(request.VolumeMode)
	request.Format = strings.TrimSpace(request.Format)
	request.Filename = strings.TrimSpace(request.Filename)
	request.SHA256 = strings.ToLower(strings.TrimSpace(request.SHA256))
	request.ActorID = strings.TrimSpace(request.ActorID)
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	return request
}

func (service *Service) validateImportRequest(request ImportRequest) error {
	if request.ProjectID == "" || request.Namespace == "" || request.ClusterID == "" || request.ActorID == "" || request.ContentLength < 1 || request.ContentLength > service.maxBytes || request.CapacityBytes < request.ContentLength || len(request.IdempotencyKey) < 8 || len(request.IdempotencyKey) > 160 {
		return domainError(volume.CodeInvalidInput, "volume import request is invalid", nil)
	}
	if request.SHA256 != "" && !validSHA256(request.SHA256) {
		return domainError(volume.CodeTransferChecksumInvalid, "volume import checksum is invalid", nil)
	}
	return validateFormatForVolume(request.VolumeMode, request.Format)
}

func normalizeExportRequest(request ExportRequest) ExportRequest {
	request.ProjectID = strings.TrimSpace(request.ProjectID)
	request.VolumeID = strings.TrimSpace(request.VolumeID)
	request.Format = strings.TrimSpace(request.Format)
	request.Consistency = strings.TrimSpace(request.Consistency)
	request.ActorID = strings.TrimSpace(request.ActorID)
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	return request
}

func exportConsistency(projectVolume model.ProjectVolume, requested string) (string, error) {
	switch requested {
	case "auto":
		if projectVolume.BindingSummary.Active == 0 && projectVolume.BindingSummary.Reserved == 0 {
			return model.VolumeTransferConsistencyUnmounted, nil
		}
		return model.VolumeTransferConsistencySnapshot, nil
	case model.VolumeTransferConsistencyUnmounted:
		if projectVolume.BindingSummary.Active != 0 || projectVolume.BindingSummary.Reserved != 0 {
			return "", domainError(volume.CodeTransferStateConflict, "mounted project volumes cannot use unmounted export consistency", nil)
		}
		return requested, nil
	case model.VolumeTransferConsistencySnapshot:
		return requested, nil
	case model.VolumeTransferConsistencyLive:
		if projectVolume.VolumeMode == model.ProjectVolumeModeBlock {
			return "", domainError(volume.CodeTransferStateConflict, "block volumes cannot use live export consistency", nil)
		}
		return requested, nil
	default:
		return "", domainError(volume.CodeInvalidInput, "volume export consistency is invalid", nil)
	}
}

func validateFormatForVolume(volumeMode, format string) error {
	if volumeMode == model.ProjectVolumeModeFilesystem && format == model.VolumeTransferFormatTarGZ {
		return nil
	}
	if volumeMode == model.ProjectVolumeModeBlock && format == model.VolumeTransferFormatRawZST {
		return nil
	}
	return domainError(volume.CodeTransferFormatMismatch, "volume transfer format does not match the volume mode", nil)
}

func requestObjectKey(direction, projectID, volumeID, idempotencyKey string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{direction, projectID, volumeID, idempotencyKey}, "\x00")))
	return "transfers/vobj_" + hex.EncodeToString(sum[:])
}

func domainError(code, message string, cause error) error {
	return &volume.DomainError{Code: code, Message: message, Cause: cause}
}

func storeError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return domainError(volume.CodeTransferStoreUnavailable, operation+" failed", err)
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		if !strings.ContainsRune("0123456789abcdef", char) {
			return false
		}
	}
	return true
}
