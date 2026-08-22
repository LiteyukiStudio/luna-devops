package volumetransferapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/volume"
)

func TestFinalizingExportReaderEOFAndCloseCommitOnlyOnce(t *testing.T) {
	waitStarted := make(chan struct{})
	releaseWait := make(chan struct{})
	stream := &exportStreamStub{waitStarted: waitStarted, releaseWait: releaseWait}
	domain := &volumeDomainStub{}
	service := &Service{volumes: domain}
	reader := &finalizingExportReader{
		stream: stream, service: service, ctx: context.Background(),
		transfer: model.VolumeTransfer{ID: "vtx_test", ProjectID: "prj_test", State: model.VolumeTransferStateStreaming},
	}

	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		_, _ = reader.Read(make([]byte, 1))
	}()
	<-waitStarted

	closeDone := make(chan struct{})
	go func() {
		defer close(closeDone)
		_ = reader.Close()
	}()
	select {
	case <-closeDone:
		t.Fatal("Close returned while EOF completion was still committing")
	case <-time.After(20 * time.Millisecond):
	}
	close(releaseWait)
	<-readDone
	<-closeDone

	if got := domain.completeCalls.Load(); got != 1 {
		t.Fatalf("completion calls = %d, want 1", got)
	}
	if got := domain.failCalls.Load(); got != 0 {
		t.Fatalf("failure calls = %d, want 0", got)
	}
	if got := stream.waitCalls.Load(); got != 1 {
		t.Fatalf("wait calls = %d, want 1", got)
	}
	if got := stream.closeCalls.Load(); got != 1 {
		t.Fatalf("close calls = %d, want 1", got)
	}
}

func TestFailUsesBoundedDetachedContextOnlyAfterCancellation(t *testing.T) {
	key := struct{}{}
	original := context.WithValue(context.Background(), key, "trace-parent")
	cancelled, cancel := context.WithCancel(original)
	cancel()
	domain := &volumeDomainStub{}
	domain.failHook = func(ctx context.Context) {
		if ctx.Err() != nil {
			t.Fatalf("failure persistence context remains cancelled: %v", ctx.Err())
		}
		if ctx.Value(key) != "trace-parent" {
			t.Fatal("failure persistence lost parent context values")
		}
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) <= 0 || time.Until(deadline) > 5*time.Second {
			t.Fatalf("failure persistence deadline = %v, ok = %t", deadline, ok)
		}
	}
	service := &Service{volumes: domain}
	_ = service.fail(cancelled, model.VolumeTransfer{ID: "vtx_test", ProjectID: "prj_test"}, volume.CodeTransferJobFailed, "stream cancelled", context.Canceled)
	if got := domain.failCalls.Load(); got != 1 {
		t.Fatalf("failure calls = %d, want 1", got)
	}
}

func TestStreamHeartbeatRefreshesProgressAndStops(t *testing.T) {
	domain := &volumeDomainStub{}
	heartbeatObserved := make(chan volume.TransferProgress, 2)
	var transferredBytes atomic.Int64
	var processedFiles atomic.Int64
	transferredBytes.Store(123)
	processedFiles.Store(7)
	domain.progressHook = func(ctx context.Context, progress volume.TransferProgress) {
		select {
		case heartbeatObserved <- progress:
		default:
		}
	}
	service := &Service{volumes: domain, heartbeatInterval: 5 * time.Millisecond}
	stop := service.startHeartbeat(context.Background(), model.VolumeTransfer{ID: "vtx_heartbeat", ProjectID: "prj_heartbeat"}, func() (int64, int64) {
		return transferredBytes.Load(), processedFiles.Load()
	})
	select {
	case progress := <-heartbeatObserved:
		if progress.TransferredBytes != 123 || progress.ProcessedFiles != 7 || progress.Phase != "streaming" {
			t.Fatalf("heartbeat progress = %#v", progress)
		}
	case <-time.After(time.Second):
		t.Fatal("stream heartbeat was not persisted")
	}
	time.Sleep(20 * time.Millisecond)
	if got := domain.progressCalls.Load(); got != 1 {
		t.Fatalf("unchanged progress refreshed heartbeat %d times, want 1", got)
	}
	transferredBytes.Store(456)
	processedFiles.Store(8)
	select {
	case progress := <-heartbeatObserved:
		if progress.TransferredBytes != 456 || progress.ProcessedFiles != 8 {
			t.Fatalf("advanced heartbeat progress = %#v", progress)
		}
	case <-time.After(time.Second):
		t.Fatal("advanced stream progress was not persisted")
	}
	stop()
	callsAfterStop := domain.progressCalls.Load()
	time.Sleep(20 * time.Millisecond)
	if got := domain.progressCalls.Load(); got != callsAfterStop {
		t.Fatalf("heartbeat calls continued after stop: before=%d after=%d", callsAfterStop, got)
	}
}

func TestStreamImportHasAbsoluteDurationLimit(t *testing.T) {
	content := []byte("direct import archive")
	digest := sha256.Sum256(content)
	checksum := hex.EncodeToString(digest[:])
	domain := &volumeDomainStub{
		project: model.ProjectVolume{ID: "pvol_timeout", ProjectID: "prj_timeout"},
		transfer: model.VolumeTransfer{
			ID: "vtx_timeout", ProjectID: "prj_timeout", ProjectVolumeID: "pvol_timeout",
			Direction: model.VolumeTransferDirectionImport, State: model.VolumeTransferStateReady,
			ExpectedBytes: int64(len(content)), SHA256: checksum, ActorID: "usr_timeout", ExpiresAt: time.Now().Add(time.Hour),
		},
	}
	service := NewService(domain, deadlineRuntimeStub{}, ticketStoreStub{}, Options{
		HeartbeatInterval: time.Hour,
		MaxStreamDuration: 20 * time.Millisecond,
	})
	body := newDeadlineBlockingReader()
	started := time.Now()
	_, err := service.StreamImport(context.Background(), "prj_timeout", "vtx_timeout", Actor{UserID: "usr_timeout"}, body, int64(len(content)), checksum)
	if !errors.Is(err, context.DeadlineExceeded) || volume.ErrorCode(err) != volume.CodeTransferJobFailed {
		t.Fatalf("stream timeout error = %v (code %q)", err, volume.ErrorCode(err))
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("stream duration limit took %s", elapsed)
	}
	if got := domain.failCalls.Load(); got != 1 {
		t.Fatalf("stream timeout failure calls = %d, want 1", got)
	}
	if !body.closed.Load() {
		t.Fatal("stream timeout did not close the blocked request body")
	}
}

func TestExportCancellationClosesBlockedStreamAndFinalizes(t *testing.T) {
	domain := &volumeDomainStub{}
	service := &Service{volumes: domain}
	stream := newBlockingExportStream()
	streamCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	reader := &finalizingExportReader{
		stream: stream, service: service, ctx: streamCtx, cancelStream: cancel,
		transfer:  model.VolumeTransfer{ID: "vtx_export_timeout", ProjectID: "prj_timeout", State: model.VolumeTransferStateStreaming},
		finalized: make(chan struct{}),
	}
	reader.watchCancellation()
	readDone := make(chan error, 1)
	go func() {
		_, err := reader.Read(make([]byte, 1))
		readDone <- err
	}()
	select {
	case err := <-readDone:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("blocked export read error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("blocked export stream was not closed at its deadline")
	}
	if got := domain.failCalls.Load(); got != 1 {
		t.Fatalf("blocked export failure calls = %d, want 1", got)
	}
}

func TestExportConsistencyAutoUsesBindingSafety(t *testing.T) {
	unmounted, err := exportConsistency(model.ProjectVolume{VolumeMode: model.ProjectVolumeModeFilesystem}, "auto")
	if err != nil || unmounted != model.VolumeTransferConsistencyUnmounted {
		t.Fatalf("unbound auto consistency = %q, %v", unmounted, err)
	}
	mounted, err := exportConsistency(model.ProjectVolume{
		VolumeMode:     model.ProjectVolumeModeFilesystem,
		BindingSummary: model.ProjectVolumeBindingSummary{Active: 1},
	}, "auto")
	if err != nil || mounted != model.VolumeTransferConsistencySnapshot {
		t.Fatalf("bound auto consistency = %q, %v", mounted, err)
	}
}

func TestStreamErrorCodePreservesRuntimeStableCode(t *testing.T) {
	err := runtimeCodedError{code: volume.CodeTransferArchiveUnsafe}
	if got := streamErrorCode(err); got != volume.CodeTransferArchiveUnsafe {
		t.Fatalf("stream error code = %q", got)
	}
	if got := streamErrorCode(runtimeCodedError{code: "unsafe.user.input"}); got != volume.CodeClusterUnavailable {
		t.Fatalf("untrusted stream error code = %q", got)
	}
}

func TestStreamImportCompletionFailureTransitionsToFailed(t *testing.T) {
	content := []byte("direct import archive")
	digest := sha256.Sum256(content)
	checksum := hex.EncodeToString(digest[:])
	domain := &volumeDomainStub{
		project: model.ProjectVolume{ID: "pvol_import", ProjectID: "prj_import"},
		transfer: model.VolumeTransfer{
			ID: "vtx_import", ProjectID: "prj_import", ProjectVolumeID: "pvol_import",
			Direction: model.VolumeTransferDirectionImport, State: model.VolumeTransferStateReady,
			ExpectedBytes: int64(len(content)), SHA256: checksum, ActorID: "usr_import", ExpiresAt: time.Now().Add(time.Hour),
		},
		completeErr: errors.New("database unavailable"),
	}
	service := NewService(domain, runtimeStreamerStub{}, ticketStoreStub{}, Options{HeartbeatInterval: time.Hour})
	_, err := service.StreamImport(context.Background(), "prj_import", "vtx_import", Actor{UserID: "usr_import"}, io.NopCloser(bytesReader(content)), int64(len(content)), checksum)
	if volume.ErrorCode(err) != volume.CodeTransferJobFailed || domain.failCalls.Load() != 1 {
		t.Fatalf("completion failure = %v (code %q), fail calls=%d", err, volume.ErrorCode(err), domain.failCalls.Load())
	}
}

func TestReadyImportExpiryIsRejectedBeforeClaim(t *testing.T) {
	domain := &volumeDomainStub{transfer: model.VolumeTransfer{
		ID: "vtx_expired", ProjectID: "prj_expired", Direction: model.VolumeTransferDirectionImport,
		State: model.VolumeTransferStateReady, ActorID: "usr_expired", ExpiresAt: time.Now().Add(-time.Minute),
	}}
	service := NewService(domain, runtimeStreamerStub{}, ticketStoreStub{}, Options{})
	_, err := service.StreamImport(context.Background(), "prj_expired", "vtx_expired", Actor{UserID: "usr_expired"}, bytesReader(nil), 1, strings.Repeat("0", 64))
	if volume.ErrorCode(err) != volume.CodeTransferExpired || domain.claimCalls.Load() != 0 {
		t.Fatalf("expired stream error=%v code=%q claim calls=%d", err, volume.ErrorCode(err), domain.claimCalls.Load())
	}
}

type runtimeCodedError struct{ code string }

func (err runtimeCodedError) Error() string             { return err.code }
func (err runtimeCodedError) TransferErrorCode() string { return err.code }

type exportStreamStub struct {
	waitStarted chan struct{}
	releaseWait chan struct{}
	waitOnce    sync.Once
	waitCalls   atomic.Int32
	closeCalls  atomic.Int32
}

func (stream *exportStreamStub) Read([]byte) (int, error) { return 0, io.EOF }

func (stream *exportStreamStub) Close() error {
	stream.closeCalls.Add(1)
	return nil
}

func (stream *exportStreamStub) Wait() (StreamResult, error) {
	stream.waitCalls.Add(1)
	stream.waitOnce.Do(func() { close(stream.waitStarted) })
	<-stream.releaseWait
	return StreamResult{TransferredBytes: 42, SHA256: "sha"}, nil
}

type volumeDomainStub struct {
	completeCalls atomic.Int32
	failCalls     atomic.Int32
	progressCalls atomic.Int32
	claimCalls    atomic.Int32
	failHook      func(context.Context)
	progressHook  func(context.Context, volume.TransferProgress)
	project       model.ProjectVolume
	transfer      model.VolumeTransfer
	completeErr   error
}

func (*volumeDomainStub) CreateProjectVolume(context.Context, volume.CreateProjectVolumeInput) (volume.CreateProjectVolumeResult, error) {
	return volume.CreateProjectVolumeResult{}, nil
}

func (domain *volumeDomainStub) GetProjectVolume(context.Context, string, string) (model.ProjectVolume, error) {
	return domain.project, nil
}

func (*volumeDomainStub) SetProjectVolumeLifecycle(context.Context, string, string, []string, string, string, string) (model.ProjectVolume, error) {
	return model.ProjectVolume{}, nil
}

func (*volumeDomainStub) CreateVolumeTransfer(context.Context, volume.CreateVolumeTransferInput) (model.VolumeTransfer, error) {
	return model.VolumeTransfer{}, nil
}

func (domain *volumeDomainStub) GetVolumeTransfer(context.Context, string, string) (model.VolumeTransfer, error) {
	return domain.transfer, nil
}

func (domain *volumeDomainStub) ClaimVolumeTransferStream(context.Context, string, string, string) (model.VolumeTransfer, error) {
	domain.claimCalls.Add(1)
	transfer := domain.transfer
	transfer.State = model.VolumeTransferStateStreaming
	return transfer, nil
}

func (domain *volumeDomainStub) CompleteVolumeTransferStream(context.Context, string, string, volume.TransferCompletion) (model.VolumeTransfer, error) {
	domain.completeCalls.Add(1)
	return model.VolumeTransfer{}, domain.completeErr
}

type runtimeStreamerStub struct{}

func (runtimeStreamerStub) OpenVolumeTransferImport(_ context.Context, _ model.ProjectVolume, _ model.VolumeTransfer, source io.Reader) (StreamResult, error) {
	hasher := sha256.New()
	transferred, err := io.Copy(hasher, source)
	return StreamResult{TransferredBytes: transferred, SHA256: hex.EncodeToString(hasher.Sum(nil))}, err
}

func (runtimeStreamerStub) OpenVolumeTransferExport(context.Context, model.ProjectVolume, model.VolumeTransfer) (ExportStream, error) {
	return nil, errors.New("not implemented")
}

type deadlineRuntimeStub struct{}

func (deadlineRuntimeStub) OpenVolumeTransferImport(_ context.Context, _ model.ProjectVolume, _ model.VolumeTransfer, source io.Reader) (StreamResult, error) {
	_, err := io.Copy(io.Discard, source)
	return StreamResult{}, err
}

type deadlineBlockingReader struct {
	release chan struct{}
	closed  atomic.Bool
	once    sync.Once
}

func newDeadlineBlockingReader() *deadlineBlockingReader {
	return &deadlineBlockingReader{release: make(chan struct{})}
}

func (reader *deadlineBlockingReader) Read([]byte) (int, error) {
	<-reader.release
	return 0, context.DeadlineExceeded
}

func (reader *deadlineBlockingReader) Close() error {
	reader.once.Do(func() {
		reader.closed.Store(true)
		close(reader.release)
	})
	return nil
}

type blockingExportStream struct {
	release chan struct{}
	once    sync.Once
}

func newBlockingExportStream() *blockingExportStream {
	return &blockingExportStream{release: make(chan struct{})}
}

func (stream *blockingExportStream) Read([]byte) (int, error) {
	<-stream.release
	return 0, context.DeadlineExceeded
}

func (stream *blockingExportStream) Close() error {
	stream.once.Do(func() { close(stream.release) })
	return nil
}

func (*blockingExportStream) Wait() (StreamResult, error) {
	return StreamResult{}, context.DeadlineExceeded
}

func (deadlineRuntimeStub) OpenVolumeTransferExport(context.Context, model.ProjectVolume, model.VolumeTransfer) (ExportStream, error) {
	return nil, errors.New("not implemented")
}

type ticketStoreStub struct{}

func (ticketStoreStub) Put(context.Context, string, []byte, time.Duration) error { return nil }
func (ticketStoreStub) Get(context.Context, string) ([]byte, bool, error)        { return nil, false, nil }
func (ticketStoreStub) Consume(context.Context, string) ([]byte, bool, error)    { return nil, false, nil }

type byteReader struct {
	content []byte
	offset  int
}

func bytesReader(content []byte) *byteReader { return &byteReader{content: content} }

func (reader *byteReader) Read(buffer []byte) (int, error) {
	if reader.offset >= len(reader.content) {
		return 0, io.EOF
	}
	n := copy(buffer, reader.content[reader.offset:])
	reader.offset += n
	return n, nil
}

func (domain *volumeDomainStub) FailVolumeTransferExecution(ctx context.Context, _, _, _, _ string) (model.VolumeTransfer, error) {
	domain.failCalls.Add(1)
	if domain.failHook != nil {
		domain.failHook(ctx)
	}
	return model.VolumeTransfer{}, nil
}

func (domain *volumeDomainStub) UpdateVolumeTransferProgress(ctx context.Context, _, _ string, progress volume.TransferProgress) (model.VolumeTransfer, error) {
	domain.progressCalls.Add(1)
	if domain.progressHook != nil {
		domain.progressHook(ctx, progress)
	}
	return model.VolumeTransfer{}, nil
}
