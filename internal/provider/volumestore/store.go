package volumestore

import (
	"context"
	"io"
	"time"
)

// Store persists temporary volume transfer archives. Implementations must not
// expose object keys, credentials, or backend URLs through API responses or
// telemetry.
type Store interface {
	CreateMultipart(ctx context.Context, key string) (uploadID string, err error)
	WritePart(ctx context.Context, key, uploadID string, partNumber int, body io.Reader, size int64) (etag string, err error)
	CompleteMultipart(ctx context.Context, key, uploadID string, parts []CompletedPart) error
	AbortMultipart(ctx context.Context, key, uploadID string) error
	Head(ctx context.Context, key string) (ObjectInfo, error)
	ReadRange(ctx context.Context, key string, offset, length int64) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}

type CompletedPart struct {
	PartNumber int
	ETag       string
}

type ObjectInfo struct {
	Size         int64
	ETag         string
	ContentType  string
	LastModified time.Time
}
