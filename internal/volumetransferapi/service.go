package volumetransferapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/LiteyukiStudio/devops/internal/volume"
)

const (
	defaultTicketTTL         = time.Minute
	defaultHeartbeatInterval = 20 * time.Second
	defaultMaxStreamDuration = 7 * 24 * time.Hour
)

type volumeDomain interface {
	CreateProjectVolume(context.Context, volume.CreateProjectVolumeInput) (volume.CreateProjectVolumeResult, error)
	GetProjectVolume(context.Context, string, string) (model.ProjectVolume, error)
	SetProjectVolumeLifecycle(context.Context, string, string, []string, string, string, string) (model.ProjectVolume, error)
	CreateVolumeTransfer(context.Context, volume.CreateVolumeTransferInput) (model.VolumeTransfer, error)
	GetVolumeTransfer(context.Context, string, string) (model.VolumeTransfer, error)
	ClaimVolumeTransferStream(context.Context, string, string, string) (model.VolumeTransfer, error)
	CompleteVolumeTransferStream(context.Context, string, string, volume.TransferCompletion) (model.VolumeTransfer, error)
	FailVolumeTransferExecution(context.Context, string, string, string, string) (model.VolumeTransfer, error)
	UpdateVolumeTransferProgress(context.Context, string, string, volume.TransferProgress) (model.VolumeTransfer, error)
}

type Options struct {
	MaxBytes          int64
	ReadyTTL          time.Duration
	TicketTTL         time.Duration
	HeartbeatInterval time.Duration
	MaxStreamDuration time.Duration
	Now               func() time.Time
}

type Service struct {
	volumes           volumeDomain
	runtime           RuntimeStreamer
	tickets           TicketStore
	maxBytes          int64
	readyTTL          time.Duration
	ticketTTL         time.Duration
	heartbeatInterval time.Duration
	maxStreamDuration time.Duration
	now               func() time.Time
}

func NewService(volumes volumeDomain, runtime RuntimeStreamer, tickets TicketStore, options Options) *Service {
	if options.ReadyTTL <= 0 {
		options.ReadyTTL = 2 * time.Hour
	}
	if options.TicketTTL <= 0 {
		options.TicketTTL = defaultTicketTTL
	}
	if options.HeartbeatInterval <= 0 {
		options.HeartbeatInterval = defaultHeartbeatInterval
	}
	if options.MaxStreamDuration <= 0 {
		options.MaxStreamDuration = defaultMaxStreamDuration
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	return &Service{
		volumes: volumes, runtime: runtime, tickets: tickets, maxBytes: options.MaxBytes,
		readyTTL: options.ReadyTTL, ticketTTL: options.TicketTTL, heartbeatInterval: options.HeartbeatInterval,
		maxStreamDuration: options.MaxStreamDuration, now: options.Now,
	}
}

func (service *Service) CreateImport(ctx context.Context, request ImportRequest) (result ImportResult, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume_transfer_api", "import.create")
	defer func() { end(err) }()
	if err = service.validate(); err != nil {
		return ImportResult{}, err
	}
	request = normalizeImportRequest(request)
	if request.ProjectID == "" || request.ActorID == "" || request.ContentLength < 1 ||
		(service.maxBytes > 0 && request.ContentLength > service.maxBytes) {
		return ImportResult{}, domainError(volume.CodeInvalidInput, "volume import metadata is invalid", nil)
	}
	volumeResult, err := service.volumes.CreateProjectVolume(ctx, volume.CreateProjectVolumeInput{
		ProjectID: request.ProjectID, DisplayName: request.DisplayName, ClusterID: request.ClusterID, Namespace: request.Namespace,
		OwnershipMode: model.ProjectVolumeOwnershipManaged, SourceKind: model.ProjectVolumeSourceArchiveImport,
		CapacityRequest: request.CapacityRequest, CapacityBytes: request.CapacityBytes, StorageClassName: request.StorageClassName,
		AccessMode: request.AccessMode, VolumeMode: request.VolumeMode, ActorID: request.ActorID, IdempotencyKey: request.IdempotencyKey,
	})
	if err != nil {
		return ImportResult{}, err
	}
	transfer, err := service.volumes.CreateVolumeTransfer(ctx, volume.CreateVolumeTransferInput{
		ProjectID: request.ProjectID, ProjectVolumeID: volumeResult.Volume.ID, Direction: model.VolumeTransferDirectionImport,
		Format: request.Format, ConsistencyMode: model.VolumeTransferConsistencyUnmounted, SourceFilename: request.Filename,
		ExpectedBytes: request.ContentLength, ActorID: request.ActorID,
		ExpiresAt: service.now().UTC().Add(service.readyTTL), IdempotencyKey: request.IdempotencyKey,
	})
	if err != nil {
		_, _ = service.volumes.SetProjectVolumeLifecycle(ctx, request.ProjectID, volumeResult.Volume.ID,
			[]string{model.ProjectVolumeLifecycleProvisioning}, model.ProjectVolumeLifecycleError, volume.ErrorCode(err), "create direct import transfer failed")
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
	request.ProjectID = strings.TrimSpace(request.ProjectID)
	request.VolumeID = strings.TrimSpace(request.VolumeID)
	request.ActorID = strings.TrimSpace(request.ActorID)
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	projectVolume, err := service.volumes.GetProjectVolume(ctx, request.ProjectID, request.VolumeID)
	if err != nil {
		return model.VolumeTransfer{}, err
	}
	consistency, err := exportConsistency(projectVolume, strings.TrimSpace(request.Consistency))
	if err != nil {
		return model.VolumeTransfer{}, err
	}
	if err = validateFormatForVolume(projectVolume.VolumeMode, strings.TrimSpace(request.Format)); err != nil {
		return model.VolumeTransfer{}, err
	}
	return service.volumes.CreateVolumeTransfer(ctx, volume.CreateVolumeTransferInput{
		ProjectID: request.ProjectID, ProjectVolumeID: request.VolumeID, Direction: model.VolumeTransferDirectionExport,
		Format: strings.TrimSpace(request.Format), ConsistencyMode: consistency, ActorID: request.ActorID,
		ExpiresAt: service.now().UTC().Add(service.readyTTL), IdempotencyKey: request.IdempotencyKey,
	})
}

func (service *Service) RetryTransfer(ctx context.Context, actor Actor, original model.VolumeTransfer, idempotencyKey string) (model.VolumeTransfer, error) {
	if err := service.authorizeActor(actor, original); err != nil {
		return model.VolumeTransfer{}, err
	}
	if !volume.IsVolumeTransferTerminal(original.State) {
		return model.VolumeTransfer{}, domainError(volume.CodeTransferStateConflict, "only terminal transfers can be retried", nil)
	}
	if original.Direction == model.VolumeTransferDirectionImport {
		return model.VolumeTransfer{}, domainError(volume.CodeTransferStateConflict, "volume imports cannot be retried because the destination may contain partial data", nil)
	}
	return service.volumes.CreateVolumeTransfer(ctx, volume.CreateVolumeTransferInput{
		ProjectID: original.ProjectID, ProjectVolumeID: original.ProjectVolumeID, Direction: original.Direction,
		Format: original.Format, ConsistencyMode: original.ConsistencyMode, SourceFilename: original.SourceFilename,
		ExpectedBytes: original.ExpectedBytes, ActorID: actor.UserID,
		ExpiresAt: service.now().UTC().Add(service.readyTTL), IdempotencyKey: strings.TrimSpace(idempotencyKey),
	})
}

func (service *Service) StreamImport(ctx context.Context, projectID, transferID string, actor Actor, body io.Reader, contentLength int64) (result model.VolumeTransfer, err error) {
	ctx, end := telemetry.StartOperation(ctx, "volume_transfer_api", "import.stream")
	defer func() { end(err) }()
	if err = service.validate(); err != nil {
		return model.VolumeTransfer{}, err
	}
	transfer, err := service.volumes.GetVolumeTransfer(ctx, strings.TrimSpace(projectID), strings.TrimSpace(transferID))
	if err != nil {
		return model.VolumeTransfer{}, err
	}
	if err = service.authorizeActor(actor, transfer); err != nil {
		return model.VolumeTransfer{}, err
	}
	if transfer.State == model.VolumeTransferStateReady && !transfer.ExpiresAt.After(service.now()) {
		return model.VolumeTransfer{}, domainError(volume.CodeTransferExpired, "volume import session expired", nil)
	}
	if transfer.Direction != model.VolumeTransferDirectionImport || contentLength != transfer.ExpectedBytes || contentLength < 1 {
		return model.VolumeTransfer{}, domainError(volume.CodeInvalidInput, "volume import length does not match the prepared transfer", nil)
	}
	claimed, err := service.volumes.ClaimVolumeTransferStream(ctx, transfer.ProjectID, transfer.ID, model.VolumeTransferDirectionImport)
	if err != nil {
		return model.VolumeTransfer{}, err
	}
	projectVolume, err := service.volumes.GetProjectVolume(ctx, transfer.ProjectID, transfer.ProjectVolumeID)
	if err != nil {
		return model.VolumeTransfer{}, service.fail(ctx, claimed, volume.CodeClusterUnavailable, "resolve import volume", err)
	}
	hasher := sha256.New()
	counted := &countingReader{reader: io.TeeReader(body, hasher)}
	limited := &io.LimitedReader{R: counted, N: contentLength + 1}
	streamCtx, cancelStream := context.WithTimeout(ctx, service.maxStreamDuration)
	stopBodyWatch := watchReaderCancellation(streamCtx, body)
	defer func() {
		stopBodyWatch()
		cancelStream()
	}()
	stopHeartbeat := service.startHeartbeat(streamCtx, claimed, func() (int64, int64) { return counted.bytes.Load(), 0 })
	streamResult, streamErr := service.runtime.OpenVolumeTransferImport(streamCtx, projectVolume, claimed, limited)
	stopHeartbeat()
	if streamErr != nil {
		return model.VolumeTransfer{}, service.fail(streamCtx, claimed, streamErrorCode(streamErr), "direct import stream failed", streamErr)
	}
	actualSHA := hex.EncodeToString(hasher.Sum(nil))
	if limited.N != 1 || streamResult.TransferredBytes != contentLength || !strings.EqualFold(streamResult.SHA256, actualSHA) {
		return model.VolumeTransfer{}, service.fail(ctx, claimed, volume.CodeTransferChecksumMismatch, "direct import checksum mismatch", nil)
	}
	result, err = service.volumes.CompleteVolumeTransferStream(ctx, transfer.ProjectID, transfer.ID, volume.TransferCompletion{
		ExpectedState: model.VolumeTransferStateStreaming, TransferredBytes: streamResult.TransferredBytes,
		ProcessedFiles: streamResult.ProcessedFiles, SHA256: actualSHA, LogicalBytes: streamResult.LogicalBytes, DataSHA256: streamResult.DataSHA256,
	})
	if err != nil {
		return model.VolumeTransfer{}, service.fail(ctx, claimed, completionErrorCode(err), "commit direct import completion", err)
	}
	return result, nil
}

func (service *Service) AuthorizeDownload(ctx context.Context, actor Actor, transfer model.VolumeTransfer, binding DownloadBinding) (DownloadAuthorization, error) {
	if err := service.validate(); err != nil {
		return DownloadAuthorization{}, err
	}
	current, err := service.volumes.GetVolumeTransfer(ctx, transfer.ProjectID, transfer.ID)
	if err != nil {
		return DownloadAuthorization{}, err
	}
	if err = service.authorizeActor(actor, current); err != nil {
		return DownloadAuthorization{}, err
	}
	if current.State == model.VolumeTransferStateReady && !current.ExpiresAt.After(service.now()) {
		return DownloadAuthorization{}, domainError(volume.CodeTransferExpired, "volume export session expired", nil)
	}
	manifestReady := current.State == model.VolumeTransferStateSucceeded && current.Format == model.VolumeTransferFormatRawZST
	if current.Direction != model.VolumeTransferDirectionExport || (current.State != model.VolumeTransferStateReady && !manifestReady) {
		return DownloadAuthorization{}, domainError(volume.CodeTransferStateConflict, "volume export is not ready", nil)
	}
	if binding.UserID != actor.UserID || strings.TrimSpace(binding.SubjectID) == "" || !binding.Deadline.After(service.now()) {
		return DownloadAuthorization{}, domainError(volume.CodeTransferDownloadUnauthorized, "download binding is invalid", nil)
	}
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return DownloadAuthorization{}, domainError(volume.CodeTransferDownloadUnauthorized, "download authorization is unavailable", err)
	}
	ticket := hex.EncodeToString(raw)
	expiresAt := service.now().UTC().Add(service.ticketTTL)
	if binding.Deadline.Before(expiresAt) {
		expiresAt = binding.Deadline.UTC()
	}
	payload, _ := json.Marshal(ticketPayload{TransferID: current.ID, ProjectID: current.ProjectID, Binding: binding, ExpiresAt: expiresAt})
	if err = service.tickets.Put(ctx, ticketKey(ticket), payload, expiresAt.Sub(service.now())); err != nil {
		return DownloadAuthorization{}, domainError(volume.CodeTransferDownloadUnauthorized, "download authorization is unavailable", err)
	}
	return DownloadAuthorization{Ticket: ticket, ExpiresAt: expiresAt}, nil
}

func (service *Service) OpenDownload(ctx context.Context, actor Actor, transfer model.VolumeTransfer, ticket string, binding DownloadBinding) (Download, error) {
	if err := service.validate(); err != nil {
		return Download{}, err
	}
	current, err := service.volumes.GetVolumeTransfer(ctx, transfer.ProjectID, transfer.ID)
	if err != nil {
		return Download{}, err
	}
	if err = service.authorizeActor(actor, current); err != nil {
		return Download{}, err
	}
	if current.Direction != model.VolumeTransferDirectionExport || current.State != model.VolumeTransferStateReady {
		return Download{}, domainError(volume.CodeTransferStateConflict, "volume export is not ready", nil)
	}
	if !current.ExpiresAt.After(service.now()) {
		return Download{}, domainError(volume.CodeTransferExpired, "volume export session expired", nil)
	}
	transfer = current
	if err := service.consumeTicket(ctx, actor, transfer, strings.TrimSpace(ticket), binding); err != nil {
		return Download{}, err
	}
	claimed, err := service.volumes.ClaimVolumeTransferStream(ctx, transfer.ProjectID, transfer.ID, model.VolumeTransferDirectionExport)
	if err != nil {
		return Download{}, err
	}
	projectVolume, err := service.volumes.GetProjectVolume(ctx, transfer.ProjectID, transfer.ProjectVolumeID)
	if err != nil {
		return Download{}, service.fail(ctx, claimed, volume.CodeClusterUnavailable, "resolve export volume", err)
	}
	streamCtx, cancelStream := context.WithTimeout(ctx, service.maxStreamDuration)
	stream, err := service.runtime.OpenVolumeTransferExport(streamCtx, projectVolume, claimed)
	if err != nil {
		cancelStream()
		return Download{}, service.fail(streamCtx, claimed, streamErrorCode(err), "open direct export stream", err)
	}
	contentType := "application/gzip"
	if transfer.Format == model.VolumeTransferFormatRawZST {
		contentType = "application/zstd"
	}
	body := &finalizingExportReader{
		stream: stream, service: service, transfer: claimed, ctx: streamCtx,
		cancelStream: cancelStream, finalized: make(chan struct{}),
	}
	body.stopHeartbeat = service.startHeartbeat(streamCtx, claimed, func() (int64, int64) { return body.transferred.Load(), 0 })
	body.watchCancellation()
	return Download{Body: body, ContentType: contentType}, nil
}

type finalizingExportReader struct {
	mu            sync.Mutex
	stream        ExportStream
	service       *Service
	transfer      model.VolumeTransfer
	ctx           context.Context
	done          bool
	finalErr      error
	transferred   atomic.Int64
	stopHeartbeat func()
	cancelStream  context.CancelFunc
	finalized     chan struct{}
}

func (reader *finalizingExportReader) Read(buffer []byte) (int, error) {
	n, err := reader.stream.Read(buffer)
	reader.transferred.Add(int64(n))
	if errors.Is(err, io.EOF) {
		if finalErr := reader.finish(true, nil); finalErr != nil {
			err = finalErr
		}
	} else if err != nil {
		_ = reader.finish(false, err)
	}
	return n, err
}

func (reader *finalizingExportReader) Close() error {
	finalErr := reader.finish(false, context.Canceled)
	return errors.Join(finalErr, reader.stream.Close())
}

func (reader *finalizingExportReader) finish(reachedEOF bool, readErr error) error {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	if reader.done {
		return reader.finalErr
	}
	reader.done = true
	if reader.finalized != nil {
		close(reader.finalized)
	}
	if reader.cancelStream != nil {
		defer reader.cancelStream()
	}
	if reader.stopHeartbeat != nil {
		reader.stopHeartbeat()
	}
	if !reachedEOF {
		reader.finalErr = reader.service.fail(reader.ctx, reader.transfer, streamErrorCode(readErr), "direct export stream interrupted", readErr)
		return reader.finalErr
	}
	result, err := reader.stream.Wait()
	if err != nil {
		reader.finalErr = reader.service.fail(reader.ctx, reader.transfer, streamErrorCode(err), "direct export helper failed", err)
		return reader.finalErr
	}
	_, err = reader.service.volumes.CompleteVolumeTransferStream(reader.ctx, reader.transfer.ProjectID, reader.transfer.ID, volume.TransferCompletion{
		ExpectedState: model.VolumeTransferStateStreaming, TransferredBytes: result.TransferredBytes,
		ProcessedFiles: result.ProcessedFiles, SHA256: result.SHA256, LogicalBytes: result.LogicalBytes, DataSHA256: result.DataSHA256,
	})
	if err != nil {
		reader.finalErr = reader.service.fail(reader.ctx, reader.transfer, volume.CodeTransferJobFailed, "commit direct export completion", err)
	}
	return reader.finalErr
}

func (reader *finalizingExportReader) watchCancellation() {
	go func() {
		select {
		case <-reader.ctx.Done():
			_ = reader.stream.Close()
			_ = reader.finish(false, reader.ctx.Err())
		case <-reader.finalized:
		}
	}()
}

type ticketPayload struct {
	TransferID string          `json:"transferId"`
	ProjectID  string          `json:"projectId"`
	Binding    DownloadBinding `json:"binding"`
	ExpiresAt  time.Time       `json:"expiresAt"`
}

func (service *Service) consumeTicket(ctx context.Context, actor Actor, transfer model.VolumeTransfer, ticket string, binding DownloadBinding) error {
	if ticket == "" || service.tickets == nil {
		return domainError(volume.CodeTransferDownloadUnauthorized, "download authorization is invalid", nil)
	}
	raw, ok, err := service.tickets.Consume(ctx, ticketKey(ticket))
	if err != nil || !ok {
		return domainError(volume.CodeTransferDownloadUnauthorized, "download authorization is invalid", err)
	}
	var payload ticketPayload
	if json.Unmarshal(raw, &payload) != nil || payload.TransferID != transfer.ID || payload.ProjectID != transfer.ProjectID ||
		payload.Binding.UserID != actor.UserID || payload.Binding.UserID != binding.UserID || payload.Binding.SubjectID != binding.SubjectID ||
		!payload.ExpiresAt.After(service.now()) {
		return domainError(volume.CodeTransferDownloadUnauthorized, "download authorization is invalid", nil)
	}
	return service.authorizeActor(actor, transfer)
}

func (service *Service) authorizeActor(actor Actor, transfer model.VolumeTransfer) error {
	if strings.TrimSpace(actor.UserID) == "" || (!actor.CanManage && transfer.ActorID != actor.UserID) {
		return domainError(volume.CodeTransferDownloadUnauthorized, "volume transfer is not available to this actor", nil)
	}
	return nil
}

func (service *Service) fail(ctx context.Context, transfer model.VolumeTransfer, code, message string, cause error) error {
	failCtx := ctx
	cancel := func() {}
	if ctx.Err() != nil {
		failCtx, cancel = context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	}
	defer cancel()
	_, failErr := service.volumes.FailVolumeTransferExecution(failCtx, transfer.ProjectID, transfer.ID, code, message)
	if failErr != nil {
		return errors.Join(domainError(code, message, cause), failErr)
	}
	return domainError(code, message, cause)
}

func (service *Service) validate() error {
	if service == nil || service.volumes == nil || service.runtime == nil || service.tickets == nil {
		return domainError(volume.CodeClusterUnavailable, "direct volume transfer service is unavailable", nil)
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
	request.ActorID = strings.TrimSpace(request.ActorID)
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	return request
}

func ticketKey(ticket string) string {
	hash := sha256.Sum256([]byte(ticket))
	return "volume-transfer:download:" + hex.EncodeToString(hash[:])
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func streamErrorCode(err error) string {
	type transferErrorCoder interface{ TransferErrorCode() string }
	var coded transferErrorCoder
	if errors.As(err, &coded) {
		switch code := strings.TrimSpace(coded.TransferErrorCode()); code {
		case volume.CodeTransferArchiveUnsafe, volume.CodeTransferCapacityExceeded,
			volume.CodeTransferChecksumMismatch, volume.CodeTransferStateConflict,
			volume.CodeTransferFormatUnsupported, volume.CodeTransferJobFailed:
			return code
		}
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return volume.CodeTransferJobFailed
	}
	return volume.CodeClusterUnavailable
}

func completionErrorCode(err error) string {
	switch code := volume.ErrorCode(err); code {
	case volume.CodeTransferChecksumInvalid, volume.CodeTransferChecksumMismatch,
		volume.CodeTransferCapacityExceeded, volume.CodeTransferStateConflict:
		return code
	default:
		return volume.CodeTransferJobFailed
	}
}

type countingReader struct {
	reader io.Reader
	bytes  atomic.Int64
}

func (reader *countingReader) Read(buffer []byte) (int, error) {
	n, err := reader.reader.Read(buffer)
	reader.bytes.Add(int64(n))
	return n, err
}

func watchReaderCancellation(ctx context.Context, reader io.Reader) func() {
	closer, ok := reader.(io.Closer)
	if !ok {
		return func() {}
	}
	stopped := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		select {
		case <-ctx.Done():
			_ = closer.Close()
		case <-stopped:
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			close(stopped)
			<-done
		})
	}
}

func (service *Service) startHeartbeat(ctx context.Context, transfer model.VolumeTransfer, progress func() (int64, int64)) func() {
	heartbeatCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(service.heartbeatInterval)
		defer ticker.Stop()
		lastTransferredBytes, lastProcessedFiles := int64(-1), int64(-1)
		for {
			select {
			case <-heartbeatCtx.Done():
				return
			case <-ticker.C:
				transferredBytes, processedFiles := progress()
				if transferredBytes == lastTransferredBytes && processedFiles == lastProcessedFiles {
					continue
				}
				_, err := service.volumes.UpdateVolumeTransferProgress(heartbeatCtx, transfer.ProjectID, transfer.ID, volume.TransferProgress{
					TransferredBytes: transferredBytes, ProcessedFiles: processedFiles, Phase: "streaming",
				})
				if err == nil {
					lastTransferredBytes, lastProcessedFiles = transferredBytes, processedFiles
				}
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			cancel()
			<-done
		})
	}
}

func domainError(code, message string, cause error) error {
	return &volume.DomainError{Code: code, Message: message, Cause: cause}
}

func validateFormatForVolume(volumeMode, format string) error {
	if (volumeMode == model.ProjectVolumeModeFilesystem && format == model.VolumeTransferFormatTarGZ) ||
		(volumeMode == model.ProjectVolumeModeBlock && format == model.VolumeTransferFormatRawZST) {
		return nil
	}
	return domainError(volume.CodeTransferFormatMismatch, "volume transfer format does not match volume mode", nil)
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
		if projectVolume.VolumeMode != model.ProjectVolumeModeFilesystem {
			return "", domainError(volume.CodeTransferStateConflict, "block volumes cannot use live export consistency", nil)
		}
		return requested, nil
	default:
		return "", domainError(volume.CodeInvalidInput, fmt.Sprintf("unsupported export consistency %q", requested), nil)
	}
}
