package transferjob

import (
	"context"
	"crypto/sha256"
	"errors"
	"hash"
	"io"
	"os"
)

type ProgressReporter func(context.Context, int64) error

type ChunkUploader struct {
	ctx           context.Context
	client        *Client
	partFile      *os.File
	partPath      string
	partHash      hash.Hash
	partSize      int64
	chunkSize     int64
	remoteOffset  int64
	skipRemaining int64
	generated     int64
	report        ProgressReporter
	closed        bool
	err           error
}

func NewChunkUploader(ctx context.Context, client *Client, remoteOffset int64, chunkSize int64, report ProgressReporter) (*ChunkUploader, error) {
	if client == nil || remoteOffset < 0 || chunkSize < 1 || chunkSize > maximumChunkSize {
		return nil, newError(CodeStateConflict, nil)
	}
	partFile, err := os.CreateTemp("", "luna-volume-export-part-*")
	if err != nil {
		return nil, newError(CodeJobFailed, err)
	}
	if err := partFile.Chmod(0o600); err != nil {
		path := partFile.Name()
		_ = partFile.Close()
		_ = os.Remove(path)
		return nil, newError(CodeJobFailed, err)
	}
	return &ChunkUploader{
		ctx: ctx, client: client, partFile: partFile, partPath: partFile.Name(), partHash: sha256.New(), chunkSize: chunkSize,
		remoteOffset: remoteOffset, skipRemaining: remoteOffset, report: report,
	}, nil
}

func (uploader *ChunkUploader) Write(content []byte) (int, error) {
	if uploader.closed {
		return 0, newError(CodeStateConflict, nil)
	}
	if uploader.err != nil {
		return 0, uploader.err
	}
	if err := uploader.ctx.Err(); err != nil {
		uploader.err = err
		return 0, err
	}
	originalLength := len(content)
	accepted := 0
	uploader.generated += int64(originalLength)
	if uploader.skipRemaining > 0 {
		skip := int64(len(content))
		if skip > uploader.skipRemaining {
			skip = uploader.skipRemaining
		}
		uploader.skipRemaining -= skip
		content = content[skip:]
		accepted += int(skip)
	}
	for len(content) > 0 {
		available := uploader.chunkSize - uploader.partSize
		writeSize := int64(len(content))
		if available < writeSize {
			writeSize = available
		}
		written, err := uploader.partFile.Write(content[:int(writeSize)])
		if err == nil && written != int(writeSize) {
			err = io.ErrShortWrite
		}
		if err != nil {
			uploader.err = newError(CodeJobFailed, err)
			return accepted + written, uploader.err
		}
		_, _ = uploader.partHash.Write(content[:written])
		uploader.partSize += int64(written)
		content = content[written:]
		accepted += written
		if uploader.partSize == uploader.chunkSize {
			if err := uploader.flush(); err != nil {
				uploader.err = err
				return accepted, err
			}
		}
	}
	return originalLength, nil
}

func (uploader *ChunkUploader) Close() error {
	if uploader.closed {
		return uploader.err
	}
	uploader.closed = true
	if uploader.err == nil && uploader.skipRemaining > 0 {
		uploader.err = newError(CodeStateConflict, nil)
	}
	if uploader.err == nil && uploader.partSize > 0 {
		uploader.err = uploader.flush()
	}
	if uploader.partFile != nil {
		if closeErr := uploader.partFile.Close(); uploader.err == nil && closeErr != nil {
			uploader.err = newError(CodeJobFailed, closeErr)
		}
		if removeErr := os.Remove(uploader.partPath); uploader.err == nil && removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			uploader.err = newError(CodeJobFailed, removeErr)
		}
		uploader.partFile = nil
	}
	return uploader.err
}

func (uploader *ChunkUploader) GeneratedBytes() int64 { return uploader.generated }
func (uploader *ChunkUploader) UploadedOffset() int64 { return uploader.remoteOffset }

func (uploader *ChunkUploader) flush() error {
	if uploader.partSize == 0 {
		return nil
	}
	if _, err := uploader.partFile.Seek(0, io.SeekStart); err != nil {
		return newError(CodeJobFailed, err)
	}
	digest := uploader.partHash.Sum(nil)
	next, err := uploader.client.WritePartStream(
		uploader.ctx,
		uploader.remoteOffset,
		io.LimitReader(uploader.partFile, uploader.partSize),
		uploader.partSize,
		digest,
	)
	if err != nil {
		return err
	}
	uploader.remoteOffset = next
	if uploader.report != nil {
		if err := uploader.report(uploader.ctx, uploader.remoteOffset); err != nil {
			return err
		}
	}
	if _, err := uploader.partFile.Seek(0, io.SeekStart); err != nil {
		return newError(CodeJobFailed, err)
	}
	if err := uploader.partFile.Truncate(0); err != nil {
		return newError(CodeJobFailed, err)
	}
	uploader.partHash.Reset()
	uploader.partSize = 0
	return nil
}

type ProgressReader struct {
	ctx       context.Context
	reader    io.Reader
	report    ProgressReporter
	interval  int64
	read      int64
	next      int64
	lastError error
}

func NewProgressReader(ctx context.Context, reader io.Reader, interval int64, report ProgressReporter) *ProgressReader {
	if interval < 1 {
		interval = 8 * 1024 * 1024
	}
	return &ProgressReader{ctx: ctx, reader: reader, report: report, interval: interval, next: interval}
}

func (reader *ProgressReader) Read(content []byte) (int, error) {
	if reader.lastError != nil {
		return 0, reader.lastError
	}
	if err := reader.ctx.Err(); err != nil {
		reader.lastError = err
		return 0, err
	}
	count, err := reader.reader.Read(content)
	reader.read += int64(count)
	if reader.report != nil && reader.read >= reader.next {
		if reportErr := reader.report(reader.ctx, reader.read); reportErr != nil {
			reader.lastError = reportErr
			return count, errors.Join(err, reportErr)
		}
		reader.next = reader.read + reader.interval
	}
	return count, err
}

func (reader *ProgressReader) Flush() error {
	if reader.lastError != nil {
		return reader.lastError
	}
	if reader.report != nil {
		reader.lastError = reader.report(reader.ctx, reader.read)
	}
	return reader.lastError
}

func (reader *ProgressReader) BytesRead() int64 { return reader.read }
