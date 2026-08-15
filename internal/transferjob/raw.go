package transferjob

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/klauspost/compress/zstd"
)

type rawDataResult struct {
	LogicalBytes int64
	DataSHA256   string
}

func importRawZST(ctx context.Context, source io.Reader, devicePath string, capacityBytes int64) (rawDataResult, error) {
	decoder, err := zstd.NewReader(source, zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxMemory(256*1024*1024))
	if err != nil {
		return rawDataResult{}, newError(CodeArchiveUnsafe, err)
	}
	defer decoder.Close()
	device, err := os.OpenFile(devicePath, os.O_WRONLY, 0)
	if err != nil {
		return rawDataResult{}, newError(CodeJobFailed, err)
	}
	defer device.Close()
	decoded := &contextReader{ctx: ctx, reader: decoder}
	digest := sha256.New()
	written, err := io.Copy(io.MultiWriter(device, digest), io.LimitReader(decoded, capacityBytes))
	if err != nil {
		return rawDataResult{}, newError(CodeArchiveUnsafe, err)
	}
	var probe [1]byte
	count, probeErr := decoded.Read(probe[:])
	if count > 0 {
		return rawDataResult{}, newError(CodeCapacityExceeded, nil)
	}
	if probeErr != nil && !errors.Is(probeErr, io.EOF) {
		return rawDataResult{}, newError(CodeArchiveUnsafe, probeErr)
	}
	if err := device.Sync(); err != nil {
		return rawDataResult{}, newError(CodeJobFailed, err)
	}
	return rawDataResult{LogicalBytes: written, DataSHA256: hex.EncodeToString(digest.Sum(nil))}, nil
}

func exportRawZST(ctx context.Context, devicePath string, capacityBytes int64, destination io.Writer) (rawDataResult, error) {
	device, err := os.Open(devicePath)
	if err != nil {
		return rawDataResult{}, newError(CodeJobFailed, err)
	}
	defer device.Close()
	encoder, err := zstd.NewWriter(destination, zstd.WithEncoderConcurrency(1), zstd.WithEncoderLevel(zstd.SpeedDefault))
	if err != nil {
		return rawDataResult{}, newError(CodeJobFailed, err)
	}
	digest := sha256.New()
	written, copyErr := io.CopyN(io.MultiWriter(encoder, digest), &contextReader{ctx: ctx, reader: device}, capacityBytes)
	closeErr := encoder.Close()
	if copyErr != nil || written != capacityBytes {
		return rawDataResult{}, newError(CodeJobFailed, fmt.Errorf("block read failed"))
	}
	if closeErr != nil {
		return rawDataResult{}, newError(CodeJobFailed, closeErr)
	}
	return rawDataResult{LogicalBytes: written, DataSHA256: hex.EncodeToString(digest.Sum(nil))}, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader *contextReader) Read(content []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(content)
}
