package volumestore

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type testContextKey struct{}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

type fakeS3Core struct {
	ctxValue       any
	createOptions  minio.PutObjectOptions
	partOptions    minio.PutObjectPartOptions
	completedParts []minio.CompletePart
	getOptions     minio.GetObjectOptions
	removed        bool
	abortErr       error
}

func (f *fakeS3Core) capture(ctx context.Context) {
	f.ctxValue = ctx.Value(testContextKey{})
}

func (f *fakeS3Core) NewMultipartUpload(ctx context.Context, _, _ string, opts minio.PutObjectOptions) (string, error) {
	f.capture(ctx)
	f.createOptions = opts
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return "upload-1", nil
}

func (f *fakeS3Core) PutObjectPart(ctx context.Context, _, _, _ string, _ int, _ io.Reader, _ int64, opts minio.PutObjectPartOptions) (minio.ObjectPart, error) {
	f.capture(ctx)
	f.partOptions = opts
	return minio.ObjectPart{ETag: `"etag-1"`}, nil
}

func (f *fakeS3Core) CompleteMultipartUpload(ctx context.Context, _, _, _ string, parts []minio.CompletePart, _ minio.PutObjectOptions) (minio.UploadInfo, error) {
	f.capture(ctx)
	f.completedParts = append([]minio.CompletePart(nil), parts...)
	return minio.UploadInfo{}, nil
}

func (f *fakeS3Core) AbortMultipartUpload(ctx context.Context, _, _, _ string) error {
	f.capture(ctx)
	return f.abortErr
}

func (f *fakeS3Core) GetObject(ctx context.Context, _, _ string, opts minio.GetObjectOptions) (io.ReadCloser, minio.ObjectInfo, http.Header, error) {
	f.capture(ctx)
	f.getOptions = opts
	return io.NopCloser(strings.NewReader("payload")), minio.ObjectInfo{}, nil, nil
}

func (f *fakeS3Core) StatObject(ctx context.Context, _, _ string, _ minio.StatObjectOptions) (minio.ObjectInfo, error) {
	f.capture(ctx)
	return minio.ObjectInfo{Size: 7, ETag: `"etag"`, ContentType: "application/octet-stream", LastModified: time.Unix(123, 0)}, nil
}

func (f *fakeS3Core) RemoveObject(ctx context.Context, _, _ string, _ minio.RemoveObjectOptions) error {
	f.capture(ctx)
	f.removed = true
	return nil
}

func TestS3StoreMultipartAndRange(t *testing.T) {
	core := &fakeS3Core{}
	store := newS3Store(core, "volume-transfers", true)
	ctx := context.WithValue(context.Background(), testContextKey{}, "preserved")

	uploadID, err := store.CreateMultipart(ctx, "transfers/vtx_demo/archive")
	if err != nil || uploadID != "upload-1" {
		t.Fatalf("CreateMultipart() = %q, %v", uploadID, err)
	}
	if core.ctxValue != "preserved" {
		t.Fatalf("caller context was not propagated: %#v", core.ctxValue)
	}
	if core.createOptions.ServerSideEncryption == nil {
		t.Fatal("multipart creation must request server-side encryption")
	}

	etag, err := store.WritePart(ctx, "transfers/vtx_demo/archive", uploadID, 1, bytes.NewBufferString("payload"), 7)
	if err != nil || etag != "etag-1" {
		t.Fatalf("WritePart() = %q, %v", etag, err)
	}
	if core.partOptions.SSE == nil {
		t.Fatal("multipart part must request server-side encryption")
	}

	err = store.CompleteMultipart(ctx, "transfers/vtx_demo/archive", uploadID, []CompletedPart{
		{PartNumber: 2, ETag: `"etag-2"`},
		{PartNumber: 1, ETag: "etag-1"},
	})
	if err != nil {
		t.Fatalf("CompleteMultipart() error = %v", err)
	}
	if got := core.completedParts; len(got) != 2 || got[0].PartNumber != 1 || got[0].ETag != "etag-1" || got[1].PartNumber != 2 {
		t.Fatalf("completed parts = %#v", got)
	}

	body, err := store.ReadRange(ctx, "transfers/vtx_demo/archive", 4, 3)
	if err != nil {
		t.Fatalf("ReadRange() error = %v", err)
	}
	_ = body.Close()
	if got := core.getOptions.Header().Get("Range"); got != "bytes=4-6" {
		t.Fatalf("Range header = %q", got)
	}

	info, err := store.Head(ctx, "transfers/vtx_demo/archive")
	if err != nil || info.Size != 7 || info.ETag != "etag" {
		t.Fatalf("Head() = %#v, %v", info, err)
	}
	if err := store.Delete(ctx, "transfers/vtx_demo/archive"); err != nil || !core.removed {
		t.Fatalf("Delete() removed=%t error=%v", core.removed, err)
	}
}

func TestS3StoreRejectsUnsafeKeysAndMultipartMetadata(t *testing.T) {
	store := newS3Store(&fakeS3Core{}, "volume-transfers", false)
	for _, key := range []string{"", "/absolute", "transfers//archive", "transfers/../archive", " transfer"} {
		if _, err := store.CreateMultipart(context.Background(), key); !errors.Is(err, ErrInvalidObjectKey) {
			t.Fatalf("key %q error = %v", key, err)
		}
	}
	if err := store.CompleteMultipart(context.Background(), "transfers/vtx_demo/archive", "upload", []CompletedPart{
		{PartNumber: 1, ETag: "etag"},
		{PartNumber: 1, ETag: "etag-again"},
	}); !errors.Is(err, ErrInvalidMultipart) {
		t.Fatalf("duplicate part error = %v", err)
	}
	if _, err := store.ReadRange(context.Background(), "transfers/vtx_demo/archive", -1, 1); !errors.Is(err, ErrInvalidRange) {
		t.Fatalf("invalid range error = %v", err)
	}
}

func TestS3StoreAbortMultipartTreatsMissingUploadAsAlreadyClean(t *testing.T) {
	core := &fakeS3Core{abortErr: minio.ErrorResponse{Code: minio.NoSuchUpload}}
	store := newS3Store(core, "volume-transfers", false)
	ctx := context.WithValue(context.Background(), testContextKey{}, "preserved")

	if err := store.AbortMultipart(ctx, "transfers/vtx_demo/archive", "already-aborted"); err != nil {
		t.Fatalf("AbortMultipart() missing upload error = %v", err)
	}
	if core.ctxValue != "preserved" {
		t.Fatalf("caller context was not propagated: %#v", core.ctxValue)
	}

	core.abortErr = errors.New("s3 unavailable")
	if err := store.AbortMultipart(ctx, "transfers/vtx_demo/archive", "active"); !errors.Is(err, core.abortErr) {
		t.Fatalf("AbortMultipart() dependency error = %v", err)
	}
}

func TestS3StorePropagatesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	store := newS3Store(&fakeS3Core{}, "volume-transfers", false)
	if _, err := store.CreateMultipart(ctx, "transfers/vtx_demo/archive"); !errors.Is(err, context.Canceled) {
		t.Fatalf("CreateMultipart() cancellation error = %v", err)
	}
}

func TestNewS3StoreRequiresSecureEndpointUnlessExplicit(t *testing.T) {
	config := S3Config{
		Endpoint:        "http://127.0.0.1:9000",
		Bucket:          "volume-transfers",
		AccessKeyID:     "access",
		SecretAccessKey: "secret",
	}
	if _, err := NewS3Store(config); !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("insecure endpoint error = %v", err)
	}
	config.AllowInsecureEndpoint = true
	store, err := NewS3Store(config)
	if err != nil {
		t.Fatalf("NewS3Store() error = %v", err)
	}
	if store.putOptions.ServerSideEncryption == nil {
		t.Fatal("server-side encryption should be enabled by default")
	}
}

func TestS3StoreSpanKeepsParentAndDoesNotExposeObjectKey(t *testing.T) {
	previous := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
		otel.SetTracerProvider(previous)
		otel.SetTextMapPropagator(previousPropagator)
	})

	parentCtx, parent := otel.Tracer("test").Start(context.Background(), "parent")
	store := newS3Store(&fakeS3Core{}, "secret-bucket", false)
	_, err := store.CreateMultipart(parentCtx, "transfers/vtx_sensitive/archive")
	parent.End()
	if err != nil {
		t.Fatalf("CreateMultipart() error = %v", err)
	}

	spans := recorder.Ended()
	var operation sdktrace.ReadOnlySpan
	for _, span := range spans {
		if span.Name() == "volumestore.multipart.create" {
			operation = span
			break
		}
	}
	if operation == nil {
		t.Fatal("volume store operation span not recorded")
	}
	if operation.Parent().SpanID() != parent.SpanContext().SpanID() {
		t.Fatalf("operation parent = %s, want %s", operation.Parent().SpanID(), parent.SpanContext().SpanID())
	}
	for _, attr := range operation.Attributes() {
		value := attr.Value.Emit()
		if strings.Contains(value, "secret-bucket") || strings.Contains(value, "vtx_sensitive") {
			t.Fatalf("sensitive storage identifier leaked in telemetry attribute %s=%q", attr.Key, value)
		}
	}
}

func TestInstrumentS3TransportRedactsTelemetryButRestoresNetworkURL(t *testing.T) {
	previous := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
		otel.SetTracerProvider(previous)
		otel.SetTextMapPropagator(previousPropagator)
	})

	var networkURL string
	var networkHost string
	var traceparent string
	base := roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		networkURL = request.URL.String()
		networkHost = request.Host
		traceparent = request.Header.Get("traceparent")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    request,
		}, nil
	})
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"https://secret-bucket.s3.example.com/transfers/vtx_sensitive/archive?uploadId=private", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() error = %v", err)
	}
	request.Host = "secret-bucket.s3.example.com"
	response, err := instrumentS3Transport(base).RoundTrip(request)
	if err != nil {
		t.Fatalf("RoundTrip() error = %v", err)
	}
	_ = response.Body.Close()
	if networkURL != request.URL.String() {
		t.Fatalf("network URL = %q, want %q", networkURL, request.URL.String())
	}
	if networkHost != request.Host {
		t.Fatalf("network host = %q, want %q", networkHost, request.Host)
	}
	if traceparent == "" {
		t.Fatal("instrumented request did not propagate traceparent")
	}
	for _, span := range recorder.Ended() {
		for _, attr := range span.Attributes() {
			value := attr.Value.Emit()
			if strings.Contains(value, "secret-bucket") || strings.Contains(value, "vtx_sensitive") || strings.Contains(value, "uploadId") {
				t.Fatalf("object URL leaked in HTTP telemetry attribute %s=%q", attr.Key, value)
			}
		}
	}
}

func TestCloneS3BaseTransportHandlesCustomDefaultTransport(t *testing.T) {
	previous := http.DefaultTransport
	custom := roundTripperFunc(func(*http.Request) (*http.Response, error) { return nil, nil })
	http.DefaultTransport = custom
	t.Cleanup(func() { http.DefaultTransport = previous })
	if got := cloneS3BaseTransport(time.Second); got == nil {
		t.Fatal("cloneS3BaseTransport() returned nil for custom fallback")
	} else if _, ok := got.(roundTripperFunc); !ok {
		t.Fatalf("cloneS3BaseTransport() type = %T, want custom fallback", got)
	}
}
