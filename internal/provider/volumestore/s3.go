package volumestore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/LiteyukiStudio/devops/internal/security"
	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/minio/minio-go/v7/pkg/encrypt"
	"github.com/minio/minio-go/v7/pkg/s3utils"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	defaultS3RequestTimeout = 30 * time.Second
	maxS3ObjectKeyLength    = 512
	maxS3MultipartParts     = 10_000
)

var (
	ErrInvalidConfiguration = errors.New("volume transfer store configuration is invalid")
	ErrInvalidObjectKey     = errors.New("volume transfer object key is invalid")
	ErrInvalidMultipart     = errors.New("volume transfer multipart request is invalid")
	ErrInvalidRange         = errors.New("volume transfer object range is invalid")

	safeObjectKeyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)
)

type S3Config struct {
	Endpoint                    string
	Region                      string
	Bucket                      string
	AccessKeyID                 string
	SecretAccessKey             string
	SessionToken                string
	AllowInsecureEndpoint       bool
	DisableServerSideEncryption bool
	RequestTimeout              time.Duration
	PathStyle                   bool
}

type s3Core interface {
	NewMultipartUpload(ctx context.Context, bucket, object string, opts minio.PutObjectOptions) (string, error)
	PutObjectPart(ctx context.Context, bucket, object, uploadID string, partID int, data io.Reader, size int64, opts minio.PutObjectPartOptions) (minio.ObjectPart, error)
	CompleteMultipartUpload(ctx context.Context, bucket, object, uploadID string, parts []minio.CompletePart, opts minio.PutObjectOptions) (minio.UploadInfo, error)
	AbortMultipartUpload(ctx context.Context, bucket, object, uploadID string) error
	GetObject(ctx context.Context, bucketName, objectName string, opts minio.GetObjectOptions) (io.ReadCloser, minio.ObjectInfo, http.Header, error)
	StatObject(ctx context.Context, bucketName, objectName string, opts minio.StatObjectOptions) (minio.ObjectInfo, error)
	RemoveObject(ctx context.Context, bucketName, objectName string, opts minio.RemoveObjectOptions) error
}

type S3Store struct {
	core        s3Core
	bucket      string
	putOptions  minio.PutObjectOptions
	partOptions minio.PutObjectPartOptions
}

func NewS3Store(config S3Config) (*S3Store, error) {
	endpoint, err := normalizeS3Endpoint(config.Endpoint)
	if err != nil {
		return nil, err
	}
	if err := s3utils.CheckValidBucketNameStrict(strings.TrimSpace(config.Bucket)); err != nil {
		return nil, fmt.Errorf("%w: bucket: %v", ErrInvalidConfiguration, err)
	}
	if strings.TrimSpace(config.AccessKeyID) == "" || strings.TrimSpace(config.SecretAccessKey) == "" {
		return nil, fmt.Errorf("%w: credentials are required", ErrInvalidConfiguration)
	}
	if endpoint.Scheme != "https" && !config.AllowInsecureEndpoint {
		return nil, fmt.Errorf("%w: insecure endpoint requires explicit opt-in", ErrInvalidConfiguration)
	}
	if _, err := security.AdminEgressPolicy().ValidateURL(endpoint.String()); err != nil {
		return nil, fmt.Errorf("%w: endpoint: %v", ErrInvalidConfiguration, err)
	}

	timeout := config.RequestTimeout
	if timeout <= 0 {
		timeout = defaultS3RequestTimeout
	}
	transport := instrumentS3Transport(cloneS3BaseTransport(timeout))

	bucketLookup := minio.BucketLookupAuto
	if config.PathStyle {
		bucketLookup = minio.BucketLookupPath
	}
	core, err := minio.NewCore(endpoint.Host, &minio.Options{
		Creds: credentials.NewStaticV4(
			strings.TrimSpace(config.AccessKeyID),
			strings.TrimSpace(config.SecretAccessKey),
			strings.TrimSpace(config.SessionToken),
		),
		Secure:       endpoint.Scheme == "https",
		Transport:    transport,
		Region:       strings.TrimSpace(config.Region),
		BucketLookup: bucketLookup,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: initialize S3 client: %v", ErrInvalidConfiguration, err)
	}

	return newS3Store(core, strings.TrimSpace(config.Bucket), !config.DisableServerSideEncryption), nil
}

type s3OriginalURLContextKey struct{}

type s3OriginalRequestTarget struct {
	url  *url.URL
	host string
}

type restoreS3RequestURLTransport struct {
	next http.RoundTripper
}

func (t restoreS3RequestURLTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	original, _ := request.Context().Value(s3OriginalURLContextKey{}).(s3OriginalRequestTarget)
	if original.url == nil {
		return t.next.RoundTrip(request)
	}
	actual := request.Clone(request.Context())
	actual.URL = cloneS3URL(original.url)
	actual.Host = original.host
	return t.next.RoundTrip(actual)
}

type sanitizeS3TelemetryTransport struct {
	next http.RoundTripper
}

func (t sanitizeS3TelemetryTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request == nil || request.URL == nil {
		return t.next.RoundTrip(request)
	}
	original := s3OriginalRequestTarget{url: cloneS3URL(request.URL), host: request.Host}
	ctx := context.WithValue(request.Context(), s3OriginalURLContextKey{}, original)
	sanitized := request.Clone(ctx)
	sanitized.URL = cloneS3URL(request.URL)
	sanitized.URL.Host = "s3.invalid"
	sanitized.Host = "s3.invalid"
	sanitized.URL.Path = "/object"
	sanitized.URL.RawPath = ""
	sanitized.URL.RawQuery = ""
	sanitized.URL.ForceQuery = false
	sanitized.URL.Fragment = ""
	sanitized.URL.User = nil
	return t.next.RoundTrip(sanitized)
}

func instrumentS3Transport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	traced := otelhttp.NewTransport(restoreS3RequestURLTransport{next: base})
	return sanitizeS3TelemetryTransport{next: traced}
}

func cloneS3BaseTransport(timeout time.Duration) http.RoundTripper {
	if transport, ok := http.DefaultTransport.(*http.Transport); ok && transport != nil {
		cloned := transport.Clone()
		cloned.Proxy = nil
		cloned.ResponseHeaderTimeout = timeout
		cloned.TLSHandshakeTimeout = min(timeout, 10*time.Second)
		return cloned
	}
	if http.DefaultTransport != nil {
		return http.DefaultTransport
	}
	return &http.Transport{
		Proxy:                 nil,
		ResponseHeaderTimeout: timeout,
		TLSHandshakeTimeout:   min(timeout, 10*time.Second),
	}
}

func cloneS3URL(value *url.URL) *url.URL {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func newS3Store(core s3Core, bucket string, serverSideEncryption bool) *S3Store {
	store := &S3Store{core: core, bucket: bucket}
	if serverSideEncryption {
		sse := encrypt.NewSSE()
		store.putOptions.ServerSideEncryption = sse
		store.partOptions.SSE = sse
	}
	return store
}

func (s *S3Store) CreateMultipart(ctx context.Context, key string) (uploadID string, err error) {
	ctx, end := startS3Operation(ctx, "multipart.create")
	defer func() { end(err) }()
	if err = validateObjectKey(key); err != nil {
		return "", err
	}
	uploadID, err = s.core.NewMultipartUpload(ctx, s.bucket, key, s.putOptions)
	return uploadID, err
}

func (s *S3Store) WritePart(ctx context.Context, key, uploadID string, partNumber int, body io.Reader, size int64) (etag string, err error) {
	ctx, end := startS3Operation(ctx, "multipart.write_part")
	defer func() { end(err) }()
	if err = validateObjectKey(key); err != nil {
		return "", err
	}
	if strings.TrimSpace(uploadID) == "" || partNumber < 1 || partNumber > maxS3MultipartParts || body == nil || size <= 0 {
		return "", ErrInvalidMultipart
	}
	part, err := s.core.PutObjectPart(ctx, s.bucket, key, uploadID, partNumber, body, size, s.partOptions)
	if err != nil {
		return "", err
	}
	return strings.Trim(part.ETag, `"`), nil
}

func (s *S3Store) CompleteMultipart(ctx context.Context, key, uploadID string, parts []CompletedPart) (err error) {
	ctx, end := startS3Operation(ctx, "multipart.complete")
	defer func() { end(err) }()
	if err = validateObjectKey(key); err != nil {
		return err
	}
	completed, err := normalizeCompletedParts(uploadID, parts)
	if err != nil {
		return err
	}
	_, err = s.core.CompleteMultipartUpload(ctx, s.bucket, key, uploadID, completed, s.putOptions)
	return err
}

func (s *S3Store) AbortMultipart(ctx context.Context, key, uploadID string) (err error) {
	ctx, end := startS3Operation(ctx, "multipart.abort")
	defer func() { end(err) }()
	if err = validateObjectKey(key); err != nil {
		return err
	}
	if strings.TrimSpace(uploadID) == "" {
		return ErrInvalidMultipart
	}
	err = s.core.AbortMultipartUpload(ctx, s.bucket, key, uploadID)
	if minio.ToErrorResponse(err).Code == minio.NoSuchUpload {
		// Abort is a crash-retry cleanup boundary. A missing upload means a
		// previous attempt already aborted or completed it, so the desired
		// terminal state has been reached.
		return nil
	}
	return err
}

func (s *S3Store) Head(ctx context.Context, key string) (object ObjectInfo, err error) {
	ctx, end := startS3Operation(ctx, "object.head")
	defer func() { end(err) }()
	if err = validateObjectKey(key); err != nil {
		return ObjectInfo{}, err
	}
	info, err := s.core.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return ObjectInfo{}, err
	}
	return ObjectInfo{
		Size:         info.Size,
		ETag:         strings.Trim(info.ETag, `"`),
		ContentType:  info.ContentType,
		LastModified: info.LastModified,
	}, nil
}

func (s *S3Store) ReadRange(ctx context.Context, key string, offset, length int64) (body io.ReadCloser, err error) {
	ctx, end := startS3Operation(ctx, "object.read_range")
	defer func() { end(err) }()
	if err = validateObjectKey(key); err != nil {
		return nil, err
	}
	if offset < 0 || length <= 0 || offset > math.MaxInt64-(length-1) {
		return nil, ErrInvalidRange
	}
	opts := minio.GetObjectOptions{}
	if err := opts.SetRange(offset, offset+length-1); err != nil {
		return nil, ErrInvalidRange
	}
	body, _, _, err = s.core.GetObject(ctx, s.bucket, key, opts)
	return body, err
}

func (s *S3Store) Delete(ctx context.Context, key string) (err error) {
	ctx, end := startS3Operation(ctx, "object.delete")
	defer func() { end(err) }()
	if err = validateObjectKey(key); err != nil {
		return err
	}
	return s.core.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}

func normalizeS3Endpoint(raw string) (*url.URL, error) {
	endpoint, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "https" && endpoint.Scheme != "http") {
		return nil, fmt.Errorf("%w: endpoint must be an HTTP(S) origin", ErrInvalidConfiguration)
	}
	if endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" || (endpoint.Path != "" && endpoint.Path != "/") {
		return nil, fmt.Errorf("%w: endpoint must not contain credentials, path, query, or fragment", ErrInvalidConfiguration)
	}
	endpoint.Path = ""
	return endpoint, nil
}

func validateObjectKey(key string) error {
	if key == "" || len(key) > maxS3ObjectKeyLength || strings.TrimSpace(key) != key || !safeObjectKeyPattern.MatchString(key) {
		return ErrInvalidObjectKey
	}
	for _, segment := range strings.Split(key, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return ErrInvalidObjectKey
		}
	}
	return nil
}

func normalizeCompletedParts(uploadID string, parts []CompletedPart) ([]minio.CompletePart, error) {
	if strings.TrimSpace(uploadID) == "" || len(parts) == 0 || len(parts) > maxS3MultipartParts {
		return nil, ErrInvalidMultipart
	}
	copyParts := append([]CompletedPart(nil), parts...)
	sort.Slice(copyParts, func(i, j int) bool { return copyParts[i].PartNumber < copyParts[j].PartNumber })
	output := make([]minio.CompletePart, 0, len(copyParts))
	previous := 0
	for _, part := range copyParts {
		etag := strings.Trim(strings.TrimSpace(part.ETag), `"`)
		if part.PartNumber < 1 || part.PartNumber > maxS3MultipartParts || part.PartNumber == previous || etag == "" {
			return nil, ErrInvalidMultipart
		}
		previous = part.PartNumber
		output = append(output, minio.CompletePart{PartNumber: part.PartNumber, ETag: etag})
	}
	return output, nil
}

func startS3Operation(ctx context.Context, operation string) (context.Context, telemetry.OperationEnd) {
	return telemetry.StartOperationWithKind(ctx, "volumestore", operation, trace.SpanKindClient,
		attribute.String("storage.system", "s3"),
	)
}
