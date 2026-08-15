package transferjob

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/LiteyukiStudio/devops/internal/volumetransfer"
	"go.opentelemetry.io/otel/attribute"
)

const failureReportTimeout = 5 * time.Second

type Result struct {
	TransferredBytes int64
	ProcessedFiles   int64
	SHA256           string
	LogicalBytes     int64
	DataSHA256       string
}

type Runner struct {
	config Config
	client *Client
	stage  string
}

func NewRunner(config Config, httpClient *http.Client) (*Runner, error) {
	client, err := NewClient(config, httpClient)
	if err != nil {
		return nil, err
	}
	return &Runner{config: config, client: client, stage: "starting"}, nil
}

func (runner *Runner) Close() error {
	if runner == nil || runner.client == nil {
		return nil
	}
	return runner.client.Close()
}

func (runner *Runner) Run(ctx context.Context) (result Result, err error) {
	if runner == nil || runner.client == nil {
		return Result{}, newError(CodeJobFailed, nil)
	}
	ctx, end := telemetry.StartOperation(ctx, "volume", "transfer."+runner.config.Direction,
		attribute.String("volume.transfer.direction", runner.config.Direction),
		attribute.String("volume.transfer.format", runner.config.Format),
	)
	startedAt := time.Now()
	defer func() {
		if err != nil {
			runner.reportFailure(ctx, err)
		}
		recordTransferMetrics(ctx, runner.config, result, startedAt, err)
		end(err)
	}()
	if err = runner.progress(ctx, "starting", 0, 0); err != nil {
		return Result{}, err
	}
	if runner.config.Direction == DirectionImport {
		result, err = runner.runImport(ctx)
	} else {
		result, err = runner.runExport(ctx)
	}
	return result, err
}

func (runner *Runner) runImport(ctx context.Context) (Result, error) {
	info, err := runner.client.HeadContent(ctx)
	if err != nil {
		return Result{}, err
	}
	if info.ChunkSize != runner.config.ChunkSize {
		return Result{}, newError(CodeStateConflict, nil)
	}
	if info.Offset != info.Size || info.Size < 1 || runner.config.ExpectedBytes > 0 && info.Size != runner.config.ExpectedBytes {
		return Result{}, newError(CodeStateConflict, nil)
	}
	body, err := runner.client.OpenContent(ctx, 0)
	if err != nil {
		return Result{}, err
	}
	defer body.Close()

	archiveDigest := sha256.New()
	progressReader := NewProgressReader(ctx, body, 8*1024*1024, func(callbackCtx context.Context, transferred int64) error {
		return runner.progress(callbackCtx, "downloading", transferred, 0)
	})
	archiveReader := io.TeeReader(progressReader, archiveDigest)
	var processedFiles int64
	var rawResult rawDataResult
	switch runner.config.VolumeMode {
	case ModeFilesystem:
		runner.stage = "extracting"
		result, extractErr := volumetransfer.ExtractTarGzip(ctx, archiveReader, runner.config.DataPath, volumetransfer.ExtractLimits{
			MaxLogicalBytes: runner.config.CapacityBytes,
			MaxFiles:        runner.config.MaxFiles,
		})
		if extractErr != nil {
			return Result{}, mapArchiveError(extractErr)
		}
		processedFiles = int64(result.Entries)
	case ModeBlock:
		runner.stage = "reading_device"
		rawResult, err = importRawZST(ctx, archiveReader, runner.config.DataPath, runner.config.CapacityBytes)
		if err != nil {
			return Result{}, err
		}
	default:
		return Result{}, newError(CodeFormatUnsupported, nil)
	}
	if _, err := io.Copy(io.Discard, archiveReader); err != nil {
		return Result{}, newError(CodeCallbackUnavailable, err)
	}
	if err := progressReader.Flush(); err != nil {
		return Result{}, err
	}
	transferred := progressReader.BytesRead()
	if transferred != info.Size {
		return Result{}, newError(CodeChecksumMismatch, nil)
	}
	checksum := hex.EncodeToString(archiveDigest.Sum(nil))
	if runner.config.ExpectedSHA256 != "" && !strings.EqualFold(runner.config.ExpectedSHA256, checksum) {
		return Result{}, newError(CodeChecksumMismatch, nil)
	}
	if err := runner.progress(ctx, "verifying", transferred, processedFiles); err != nil {
		return Result{}, err
	}
	if err := runner.client.Complete(ctx, CompleteInput{
		ExpectedState: "running", TransferredBytes: transferred, SHA256: checksum,
		LogicalBytes: rawResult.LogicalBytes, DataSHA256: rawResult.DataSHA256,
	}); err != nil {
		return Result{}, err
	}
	runner.stage = "completed"
	return Result{
		TransferredBytes: transferred, ProcessedFiles: processedFiles, SHA256: checksum,
		LogicalBytes: rawResult.LogicalBytes, DataSHA256: rawResult.DataSHA256,
	}, nil
}

func (runner *Runner) runExport(ctx context.Context) (Result, error) {
	info, err := runner.client.HeadContent(ctx)
	if err != nil {
		return Result{}, err
	}
	if info.ChunkSize != runner.config.ChunkSize {
		return Result{}, newError(CodeStateConflict, nil)
	}
	archiveDigest := sha256.New()
	uploader, err := NewChunkUploader(ctx, runner.client, info.Offset, info.ChunkSize, func(callbackCtx context.Context, transferred int64) error {
		return runner.progress(callbackCtx, "uploading", transferred, 0)
	})
	if err != nil {
		return Result{}, err
	}
	destination := io.MultiWriter(archiveDigest, uploader)
	var processedFiles int64
	var rawResult rawDataResult
	switch runner.config.VolumeMode {
	case ModeFilesystem:
		runner.stage = "archiving"
		manifest, exportErr := volumetransfer.WriteTarGzip(ctx, runner.config.DataPath, destination, volumetransfer.ExportOptions{
			ConsistencyMode: runner.config.ConsistencyMode,
			ExportedAt:      runner.config.ExportedAt,
			MaxLogicalBytes: runner.config.CapacityBytes,
			MaxFiles:        runner.config.MaxFiles,
		})
		if exportErr != nil {
			_ = uploader.Close()
			return Result{}, mapArchiveError(exportErr)
		}
		processedFiles = int64(manifest.FileCount)
	case ModeBlock:
		runner.stage = "reading_device"
		rawResult, err = exportRawZST(ctx, runner.config.DataPath, runner.config.CapacityBytes, destination)
		if err != nil {
			_ = uploader.Close()
			return Result{}, err
		}
	default:
		_ = uploader.Close()
		return Result{}, newError(CodeFormatUnsupported, nil)
	}
	if err := uploader.Close(); err != nil {
		return Result{}, err
	}
	if uploader.UploadedOffset() != uploader.GeneratedBytes() {
		return Result{}, newError(CodeStateConflict, nil)
	}
	checksum := hex.EncodeToString(archiveDigest.Sum(nil))
	transferred := uploader.GeneratedBytes()
	if err := runner.progress(ctx, "verifying", transferred, processedFiles); err != nil {
		return Result{}, err
	}
	if err := runner.client.Complete(ctx, CompleteInput{
		ExpectedState: "running", TransferredBytes: transferred, SHA256: checksum,
		LogicalBytes: rawResult.LogicalBytes, DataSHA256: rawResult.DataSHA256,
	}); err != nil {
		return Result{}, err
	}
	runner.stage = "completed"
	return Result{
		TransferredBytes: transferred, ProcessedFiles: processedFiles, SHA256: checksum,
		LogicalBytes: rawResult.LogicalBytes, DataSHA256: rawResult.DataSHA256,
	}, nil
}

func (runner *Runner) progress(ctx context.Context, stage string, transferred, files int64) error {
	runner.stage = stage
	return runner.client.Progress(ctx, ProgressInput{
		ExpectedState: "running", TransferredBytes: transferred, ProcessedFiles: files, Stage: stage,
	})
}

func (runner *Runner) reportFailure(ctx context.Context, operationErr error) {
	if errors.Is(operationErr, context.Canceled) {
		return
	}
	code := ErrorCode(operationErr)
	var remote *RemoteError
	if errors.As(operationErr, &remote) && (remote.Code == CodeStateConflict || remote.Status == http.StatusUnauthorized || remote.Status == http.StatusForbidden) {
		return
	}
	// A signal-cancelled operation cannot use its original context to report a
	// terminal failure. Preserve trace values while detaching cancellation only
	// for this bounded final callback; the data operation itself stays cancelled.
	reportCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), failureReportTimeout)
	defer cancel()
	_ = runner.client.Fail(reportCtx, FailInput{
		ExpectedState: "running",
		ErrorCode:     code,
		Diagnostic:    "volume transfer job failed during " + stableFailureStage(runner.stage),
	})
}

func stableFailureStage(stage string) string {
	if stableStage(stage) {
		return stage
	}
	return "starting"
}

func mapArchiveError(err error) error {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case errors.Is(err, volumetransfer.ErrArchiveCapacityExceeded):
		return newError(CodeCapacityExceeded, err)
	case errors.Is(err, volumetransfer.ErrArchiveChecksumMismatch):
		return newError(CodeChecksumMismatch, err)
	case errors.Is(err, volumetransfer.ErrArchiveUnsafe), errors.Is(err, volumetransfer.ErrArchiveFileLimit):
		return newError(CodeArchiveUnsafe, err)
	default:
		return newError(CodeJobFailed, err)
	}
}
