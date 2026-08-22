package transferjob

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/LiteyukiStudio/devops/internal/volumetransfer"
	"go.opentelemetry.io/otel/attribute"
)

type Result struct {
	TransferredBytes int64  `json:"transferredBytes"`
	ProcessedFiles   int64  `json:"processedFiles"`
	SHA256           string `json:"sha256"`
	LogicalBytes     int64  `json:"logicalBytes"`
	DataSHA256       string `json:"dataSha256,omitempty"`
}

type Runner struct{ config Config }

func NewRunner(config Config) (*Runner, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &Runner{config: config}, nil
}

func (runner *Runner) RunStream(ctx context.Context, source io.Reader, destination io.Writer) (result Result, err error) {
	if runner == nil {
		return Result{}, newError(CodeJobFailed, nil)
	}
	ctx, end := telemetry.StartOperation(ctx, "volume", "transfer_stream."+runner.config.Direction,
		attribute.String("volume.transfer.direction", runner.config.Direction),
		attribute.String("volume.transfer.format", runner.config.Format),
	)
	startedAt := time.Now()
	defer func() {
		recordTransferMetrics(ctx, runner.config, result, startedAt, err)
		end(err)
	}()
	if runner.config.Direction == DirectionImport {
		if source == nil {
			return Result{}, newError(CodeStateConflict, nil)
		}
		return runner.runImport(ctx, source)
	}
	if destination == nil {
		return Result{}, newError(CodeStateConflict, nil)
	}
	return runner.runExport(ctx, destination)
}

func (runner *Runner) runImport(ctx context.Context, source io.Reader) (Result, error) {
	archiveDigest := sha256.New()
	limited := &boundedArchiveReader{reader: &contextReader{ctx: ctx, reader: source}, remaining: runner.config.ExpectedBytes}
	archiveReader := io.TeeReader(limited, archiveDigest)
	var processedFiles int64
	var rawResult rawDataResult
	var err error
	switch runner.config.VolumeMode {
	case ModeFilesystem:
		result, extractErr := volumetransfer.ExtractTarGzip(ctx, archiveReader, runner.config.DataPath, volumetransfer.ExtractLimits{
			MaxLogicalBytes: runner.config.CapacityBytes,
			MaxFiles:        runner.config.MaxFiles,
		})
		if extractErr != nil {
			return Result{}, mapArchiveError(extractErr)
		}
		processedFiles = int64(result.Entries)
	case ModeBlock:
		rawResult, err = importRawZST(ctx, archiveReader, runner.config.DataPath, runner.config.CapacityBytes)
		if err != nil {
			return Result{}, err
		}
	default:
		return Result{}, newError(CodeFormatUnsupported, nil)
	}
	if _, err := io.Copy(io.Discard, archiveReader); err != nil {
		return Result{}, newError(CodeJobFailed, err)
	}
	if limited.read != runner.config.ExpectedBytes {
		return Result{}, newError(CodeChecksumMismatch, nil)
	}
	var probe [1]byte
	if count, probeErr := source.Read(probe[:]); count != 0 || probeErr != nil && !errors.Is(probeErr, io.EOF) {
		return Result{}, newError(CodeChecksumMismatch, probeErr)
	}
	checksum := hex.EncodeToString(archiveDigest.Sum(nil))
	if !strings.EqualFold(runner.config.ExpectedSHA256, checksum) {
		return Result{}, newError(CodeChecksumMismatch, nil)
	}
	return Result{TransferredBytes: limited.read, ProcessedFiles: processedFiles, SHA256: checksum,
		LogicalBytes: rawResult.LogicalBytes, DataSHA256: rawResult.DataSHA256}, nil
}

func (runner *Runner) runExport(ctx context.Context, destination io.Writer) (Result, error) {
	archiveDigest := sha256.New()
	counter := &boundedArchiveWriter{writer: io.MultiWriter(destination, archiveDigest), remaining: runner.config.MaxArchiveBytes}
	var processedFiles int64
	var rawResult rawDataResult
	var err error
	switch runner.config.VolumeMode {
	case ModeFilesystem:
		manifest, exportErr := volumetransfer.WriteTarGzip(ctx, runner.config.DataPath, counter, volumetransfer.ExportOptions{
			ConsistencyMode: runner.config.ConsistencyMode,
			ExportedAt:      runner.config.ExportedAt,
			MaxLogicalBytes: runner.config.CapacityBytes,
			MaxFiles:        runner.config.MaxFiles,
		})
		if exportErr != nil {
			return Result{}, mapArchiveError(exportErr)
		}
		processedFiles = int64(manifest.FileCount)
	case ModeBlock:
		rawResult, err = exportRawZST(ctx, runner.config.DataPath, runner.config.CapacityBytes, counter)
		if err != nil {
			return Result{}, err
		}
	default:
		return Result{}, newError(CodeFormatUnsupported, nil)
	}
	return Result{TransferredBytes: counter.written, ProcessedFiles: processedFiles,
		SHA256: hex.EncodeToString(archiveDigest.Sum(nil)), LogicalBytes: rawResult.LogicalBytes,
		DataSHA256: rawResult.DataSHA256}, nil
}

type boundedArchiveReader struct {
	reader    io.Reader
	remaining int64
	read      int64
}

func (reader *boundedArchiveReader) Read(content []byte) (int, error) {
	if reader.remaining == 0 {
		return 0, io.EOF
	}
	if int64(len(content)) > reader.remaining {
		content = content[:reader.remaining]
	}
	count, err := reader.reader.Read(content)
	reader.read += int64(count)
	reader.remaining -= int64(count)
	return count, err
}

type boundedArchiveWriter struct {
	writer    io.Writer
	written   int64
	remaining int64
}

func (writer *boundedArchiveWriter) Write(content []byte) (int, error) {
	if writer.remaining == 0 {
		return 0, newError(CodeCapacityExceeded, nil)
	}
	originalLength := len(content)
	if int64(len(content)) > writer.remaining {
		content = content[:writer.remaining]
	}
	count, err := writer.writer.Write(content)
	writer.written += int64(count)
	writer.remaining -= int64(count)
	if err == nil && count == len(content) && len(content) < originalLength {
		err = newError(CodeCapacityExceeded, nil)
	}
	return count, err
}

func mapArchiveError(err error) error {
	var transferErr *Error
	if errors.As(err, &transferErr) {
		return transferErr
	}
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
