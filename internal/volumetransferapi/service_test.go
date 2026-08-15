package volumetransferapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/provider/volumestore"
	"github.com/LiteyukiStudio/devops/internal/volume"
)

type transferDomainStub struct {
	volumeDomain
	mu           sync.Mutex
	transfer     model.VolumeTransfer
	parts        []model.VolumeTransferPart
	progress     volume.TransferProgress
	preflightErr error
	writeEntered chan struct{}
	writeOnce    sync.Once
}

func (stub *transferDomainStub) GetVolumeTransfer(_ context.Context, projectID, transferID string) (model.VolumeTransfer, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.transfer.ProjectID != projectID || stub.transfer.ID != transferID {
		return model.VolumeTransfer{}, &volume.DomainError{Code: volume.CodeTransferNotFound, Message: "not found"}
	}
	return stub.transfer, nil
}

func (stub *transferDomainStub) GetVolumeTransferForMaintenance(_ context.Context, transferID string) (model.VolumeTransfer, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.transfer.ID != transferID {
		return model.VolumeTransfer{}, &volume.DomainError{Code: volume.CodeTransferNotFound, Message: "not found"}
	}
	return stub.transfer, nil
}

func (stub *transferDomainStub) PreflightVolumeTransferPart(context.Context, string, string, model.VolumeTransferPart) error {
	return stub.preflightErr
}

func (stub *transferDomainStub) WriteVolumeTransferPart(ctx context.Context, _, _ string, part model.VolumeTransferPart, writer volume.TransferPartWriter) (model.VolumeTransferPart, int64, error) {
	if stub.writeEntered != nil {
		stub.writeOnce.Do(func() { close(stub.writeEntered) })
	}
	stub.mu.Lock()
	defer stub.mu.Unlock()
	for _, existing := range stub.parts {
		if existing.Offset == part.Offset {
			if existing.Size != part.Size || existing.SHA256 != part.SHA256 {
				return model.VolumeTransferPart{}, partsOffset(stub.parts), &volume.DomainError{Code: volume.CodeTransferOffsetMismatch, Message: "offset mismatch"}
			}
			return existing, partsOffset(stub.parts), nil
		}
	}
	if part.Offset != partsOffset(stub.parts) {
		return model.VolumeTransferPart{}, partsOffset(stub.parts), &volume.DomainError{Code: volume.CodeTransferOffsetMismatch, Message: "offset mismatch"}
	}
	part.PartNumber = len(stub.parts) + 1
	etag, err := writer(ctx, part.PartNumber)
	if err != nil {
		return model.VolumeTransferPart{}, partsOffset(stub.parts), err
	}
	part.ETag = etag
	stub.parts = append(stub.parts, part)
	return part, part.Offset + part.Size, nil
}

func (stub *transferDomainStub) ListVolumeTransferParts(_ context.Context, transferID string, page, pageSize int) ([]model.VolumeTransferPart, int64, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.transfer.ID != transferID {
		return nil, 0, &volume.DomainError{Code: volume.CodeTransferNotFound, Message: "not found"}
	}
	items := append([]model.VolumeTransferPart(nil), stub.parts...)
	sort.Slice(items, func(i, j int) bool { return items[i].PartNumber < items[j].PartNumber })
	start := (page - 1) * pageSize
	if start >= len(items) {
		return []model.VolumeTransferPart{}, int64(len(items)), nil
	}
	end := min(start+pageSize, len(items))
	return items[start:end], int64(len(items)), nil
}

func (stub *transferDomainStub) CompleteVolumeTransferUpload(_ context.Context, _, _ string, length int64, checksum string) (model.VolumeTransfer, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	stub.transfer.State = model.VolumeTransferStateQueued
	stub.transfer.ExpectedBytes = length
	stub.transfer.TransferredBytes = length
	stub.transfer.SHA256 = checksum
	stub.transfer.MultipartUploadID = ""
	return stub.transfer, nil
}

func (stub *transferDomainStub) UpdateVolumeTransferProgress(_ context.Context, _, _ string, progress volume.TransferProgress) (model.VolumeTransfer, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	stub.progress = progress
	stub.transfer.TransferredBytes = progress.TransferredBytes
	stub.transfer.ProcessedFiles = progress.ProcessedFiles
	stub.transfer.Phase = progress.Phase
	return stub.transfer, nil
}

func (stub *transferDomainStub) ReportVolumeTransferCompletion(_ context.Context, _, _ string, completion volume.TransferCompletion) (model.VolumeTransfer, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.transfer.State != completion.ExpectedState {
		return model.VolumeTransfer{}, &volume.DomainError{Code: volume.CodeTransferStateConflict, Message: "state changed"}
	}
	if stub.transfer.CompletionReportedAt != nil {
		if stub.transfer.TransferredBytes == completion.TransferredBytes && stub.transfer.SHA256 == completion.SHA256 &&
			stub.transfer.LogicalBytes == completion.LogicalBytes && stub.transfer.DataSHA256 == completion.DataSHA256 {
			return stub.transfer, nil
		}
		return model.VolumeTransfer{}, &volume.DomainError{Code: volume.CodeTransferStateConflict, Message: "completion changed"}
	}
	stub.transfer.ExpectedBytes = completion.TransferredBytes
	stub.transfer.TransferredBytes = completion.TransferredBytes
	stub.transfer.SHA256 = completion.SHA256
	stub.transfer.LogicalBytes = completion.LogicalBytes
	stub.transfer.DataSHA256 = completion.DataSHA256
	now := time.Now().UTC()
	stub.transfer.CompletionReportedAt = &now
	return stub.transfer, nil
}

func (stub *transferDomainStub) FailVolumeTransferExecution(_ context.Context, projectID, transferID, code, diagnostic string) (model.VolumeTransfer, error) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if stub.transfer.ProjectID != projectID || stub.transfer.ID != transferID {
		return model.VolumeTransfer{}, &volume.DomainError{Code: volume.CodeTransferNotFound, Message: "not found"}
	}
	if stub.transfer.State == model.VolumeTransferStateFailed {
		if stub.transfer.LastErrorCode == code {
			return stub.transfer, nil
		}
		return model.VolumeTransfer{}, &volume.DomainError{Code: volume.CodeTransferStateConflict, Message: "failure changed"}
	}
	if stub.transfer.State != model.VolumeTransferStateRunning && stub.transfer.State != model.VolumeTransferStateUploading {
		return model.VolumeTransfer{}, &volume.DomainError{Code: volume.CodeTransferStateConflict, Message: "state changed"}
	}
	stub.transfer.State = model.VolumeTransferStateFailed
	stub.transfer.LastErrorCode = code
	stub.transfer.LastErrorMessage = diagnostic
	return stub.transfer, nil
}

type memoryVolumeStore struct {
	mu        sync.Mutex
	objects   map[string][]byte
	parts     map[string]map[int][]byte
	cancelled bool
	readCalls int
}

func newMemoryVolumeStore() *memoryVolumeStore {
	return &memoryVolumeStore{objects: map[string][]byte{}, parts: map[string]map[int][]byte{}}
}

func (store *memoryVolumeStore) CreateMultipart(ctx context.Context, key string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.parts[key] = map[int][]byte{}
	return "upload-demo", nil
}

func (store *memoryVolumeStore) WritePart(ctx context.Context, key, _ string, partNumber int, body io.Reader, size int64) (string, error) {
	if err := ctx.Err(); err != nil {
		store.cancelled = true
		return "", err
	}
	content, err := io.ReadAll(body)
	if err != nil {
		return "", err
	}
	if int64(len(content)) != size {
		return "", errors.New("part size mismatch")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.parts[key] == nil {
		store.parts[key] = map[int][]byte{}
	}
	store.parts[key][partNumber] = append([]byte(nil), content...)
	return "etag-demo", nil
}

func (store *memoryVolumeStore) CompleteMultipart(ctx context.Context, key, _ string, parts []volumestore.CompletedPart) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	var content []byte
	for _, part := range parts {
		content = append(content, store.parts[key][part.PartNumber]...)
	}
	store.objects[key] = content
	return nil
}

func (store *memoryVolumeStore) AbortMultipart(context.Context, string, string) error { return nil }

func (store *memoryVolumeStore) Head(ctx context.Context, key string) (volumestore.ObjectInfo, error) {
	if err := ctx.Err(); err != nil {
		return volumestore.ObjectInfo{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	content, exists := store.objects[key]
	if !exists {
		return volumestore.ObjectInfo{}, errors.New("not found")
	}
	return volumestore.ObjectInfo{Size: int64(len(content)), ETag: "etag-object"}, nil
}

func (store *memoryVolumeStore) ReadRange(ctx context.Context, key string, offset, length int64) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.readCalls++
	content, exists := store.objects[key]
	if !exists || offset < 0 || length < 1 || offset > int64(len(content))-length {
		return nil, errors.New("invalid range")
	}
	return io.NopCloser(bytes.NewReader(append([]byte(nil), content[offset:offset+length]...))), nil
}

func (store *memoryVolumeStore) Delete(_ context.Context, key string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	delete(store.objects, key)
	return nil
}

func TestImportUploadIsResumableAndDefersFullVerificationToWorker(t *testing.T) {
	t.Parallel()
	content := []byte("verified volume archive")
	checksum := sha256.Sum256(content)
	domain := &transferDomainStub{transfer: model.VolumeTransfer{
		ID: "vtx_import", ProjectID: "prj_demo", ProjectVolumeID: "pvol_demo",
		Direction: model.VolumeTransferDirectionImport, Format: model.VolumeTransferFormatTarGZ,
		State: model.VolumeTransferStateUploading, ObjectKey: "transfers/import-demo",
		MultipartUploadID: "upload-demo", ExpectedBytes: int64(len(content)), SHA256: hex.EncodeToString(checksum[:]),
		ActorID: "usr_demo", ExpiresAt: time.Now().Add(time.Hour),
	}}
	store := newMemoryVolumeStore()
	store.parts[domain.transfer.ObjectKey] = map[int][]byte{}
	service := NewService(domain, store, NewMemoryTicketStore(), Options{MaxBytes: 1024})
	chunkChecksum := base64.StdEncoding.EncodeToString(checksum[:])

	offset, chunkSize, err := service.WriteImportPart(context.Background(), "prj_demo", "vtx_import", Actor{UserID: "usr_demo"}, 0, chunkChecksum, bytes.NewReader(content), int64(len(content)))
	if err != nil || offset != int64(len(content)) {
		t.Fatalf("write import part offset=%d err=%v", offset, err)
	}
	if chunkSize != MinimumChunkSize {
		t.Fatalf("chunk size=%d, want %d", chunkSize, MinimumChunkSize)
	}
	replayed, _, err := service.WriteImportPart(context.Background(), "prj_demo", "vtx_import", Actor{UserID: "usr_demo"}, 0, chunkChecksum, bytes.NewReader(content), int64(len(content)))
	if err != nil || replayed != offset || len(domain.parts) != 1 {
		t.Fatalf("replay offset=%d parts=%d err=%v", replayed, len(domain.parts), err)
	}
	if _, _, err := service.WriteImportPart(context.Background(), "prj_demo", "vtx_import", Actor{UserID: "usr_demo"}, offset+1, chunkChecksum, bytes.NewReader(content), int64(len(content))); volume.ErrorCode(err) != volume.CodeTransferOffsetMismatch {
		t.Fatalf("offset mismatch code=%q err=%v", volume.ErrorCode(err), err)
	}
	completed, err := service.CompleteImport(context.Background(), "prj_demo", "vtx_import", Actor{UserID: "usr_demo"}, int64(len(content)), hex.EncodeToString(checksum[:]))
	if err != nil || completed.State != model.VolumeTransferStateQueued || completed.SHA256 != hex.EncodeToString(checksum[:]) {
		t.Fatalf("complete result=%#v err=%v", completed, err)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.readCalls != 0 {
		t.Fatalf("API completion synchronously re-read the archive %d times", store.readCalls)
	}
}

func TestUploadCancellationReachesObjectStore(t *testing.T) {
	t.Parallel()
	content := []byte("cancelled")
	checksum := sha256.Sum256(content)
	domain := &transferDomainStub{transfer: model.VolumeTransfer{
		ID: "vtx_cancel", ProjectID: "prj_demo", Direction: model.VolumeTransferDirectionImport,
		State: model.VolumeTransferStateUploading, ObjectKey: "transfers/cancel-demo", MultipartUploadID: "upload-demo",
		ExpectedBytes: int64(len(content)), ActorID: "usr_demo", ExpiresAt: time.Now().Add(time.Hour),
	}}
	store := newMemoryVolumeStore()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := NewService(domain, store, NewMemoryTicketStore(), Options{MaxBytes: 1024}).WriteImportPart(
		ctx, "prj_demo", domain.transfer.ID, Actor{UserID: "usr_demo"}, 0,
		base64.StdEncoding.EncodeToString(checksum[:]), bytes.NewReader(content), int64(len(content)),
	)
	if !errors.Is(err, context.Canceled) || store.cancelled {
		t.Fatalf("write error=%v objectStoreReached=%t", err, store.cancelled)
	}
}

func TestRequiredChunkSizeKeepsFiveTiBWithinMultipartLimit(t *testing.T) {
	t.Parallel()
	const fiveTiB = int64(5 * 1024 * 1024 * 1024 * 1024)
	chunkSize := RequiredChunkSize(fiveTiB)
	partCount := (fiveTiB + chunkSize - 1) / chunkSize
	if chunkSize != 525*1024*1024 {
		t.Fatalf("chunk size=%d, want %d", chunkSize, int64(525*1024*1024))
	}
	if partCount > MaxMultipartParts || chunkSize > MaximumChunkSize {
		t.Fatalf("chunk size=%d parts=%d", chunkSize, partCount)
	}
	if got := RequiredChunkSize(MinimumChunkSize * MaxMultipartParts); got != MinimumChunkSize {
		t.Fatalf("threshold chunk size=%d, want %d", got, MinimumChunkSize)
	}
	if got := RequiredChunkSize(MinimumChunkSize*MaxMultipartParts + 1); got != 65*1024*1024 {
		t.Fatalf("above-threshold chunk size=%d, want %d", got, int64(65*1024*1024))
	}
}

func TestUploadSpoolsSlowSourceBeforeEnteringDatabaseWriter(t *testing.T) {
	t.Parallel()
	content := []byte("slow client body")
	checksum := sha256.Sum256(content)
	writeEntered := make(chan struct{})
	domain := &transferDomainStub{writeEntered: writeEntered, transfer: model.VolumeTransfer{
		ID: "vtx_slow", ProjectID: "prj_demo", Direction: model.VolumeTransferDirectionImport,
		State: model.VolumeTransferStateUploading, ObjectKey: "transfers/slow-demo", MultipartUploadID: "upload-demo",
		ExpectedBytes: int64(len(content)), ActorID: "usr_demo", ExpiresAt: time.Now().Add(time.Hour),
	}}
	store := newMemoryVolumeStore()
	store.parts[domain.transfer.ObjectKey] = map[int][]byte{}
	tempDir := t.TempDir()
	reader, writer := io.Pipe()
	type result struct {
		offset int64
		err    error
	}
	resultChannel := make(chan result, 1)
	go func() {
		offset, _, err := NewService(domain, store, NewMemoryTicketStore(), Options{MaxBytes: 1024, TempDir: tempDir}).WriteImportPart(
			context.Background(), "prj_demo", domain.transfer.ID, Actor{UserID: "usr_demo"}, 0,
			base64.StdEncoding.EncodeToString(checksum[:]), reader, int64(len(content)),
		)
		resultChannel <- result{offset: offset, err: err}
	}()
	if _, err := writer.Write(content[:1]); err != nil {
		t.Fatal(err)
	}
	select {
	case <-writeEntered:
		t.Fatal("database writer entered before the request body was fully spooled")
	case <-time.After(50 * time.Millisecond):
	}
	if _, err := writer.Write(content[1:]); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-writeEntered:
	case <-time.After(time.Second):
		t.Fatal("database writer was not entered after the request body completed")
	}
	completed := <-resultChannel
	if completed.err != nil || completed.offset != int64(len(content)) {
		t.Fatalf("write offset=%d err=%v", completed.offset, completed.err)
	}
	assertDirectoryEmpty(t, tempDir)
}

func TestUploadRejectsActivePartLeaseBeforeReadingOrSpoolingBody(t *testing.T) {
	t.Parallel()
	content := []byte("already uploading")
	checksum := sha256.Sum256(content)
	domain := &transferDomainStub{
		preflightErr: &volume.DomainError{Code: volume.CodeTransferPartInProgress, Message: "part in progress"},
		transfer: model.VolumeTransfer{
			ID: "vtx_active_lease", ProjectID: "prj_demo", Direction: model.VolumeTransferDirectionImport,
			State: model.VolumeTransferStateUploading, ObjectKey: "transfers/active-lease", MultipartUploadID: "upload-demo",
			ExpectedBytes: int64(len(content)), ActorID: "usr_demo", ExpiresAt: time.Now().Add(time.Hour),
		},
	}
	body := &trackingReader{}
	tempDir := t.TempDir()
	_, _, err := NewService(domain, newMemoryVolumeStore(), NewMemoryTicketStore(), Options{MaxBytes: 1024, TempDir: tempDir}).WriteImportPart(
		context.Background(), "prj_demo", domain.transfer.ID, Actor{UserID: "usr_demo"}, 0,
		base64.StdEncoding.EncodeToString(checksum[:]), body, int64(len(content)),
	)
	if volume.ErrorCode(err) != volume.CodeTransferPartInProgress || body.read {
		t.Fatalf("write code=%q err=%v bodyRead=%t", volume.ErrorCode(err), err, body.read)
	}
	assertDirectoryEmpty(t, tempDir)
}

func TestUploadRejectsChecksumAndCancellationBeforeDatabaseLockAndCleansTempFile(t *testing.T) {
	t.Parallel()
	t.Run("checksum", func(t *testing.T) {
		content := []byte("checksum body")
		wrong := sha256.Sum256([]byte("other body"))
		writeEntered := make(chan struct{})
		domain := uploadTestDomain("vtx_bad_checksum", int64(len(content)), writeEntered)
		tempDir := t.TempDir()
		_, _, err := NewService(domain, newMemoryVolumeStore(), NewMemoryTicketStore(), Options{MaxBytes: 1024, TempDir: tempDir}).WriteImportPart(
			context.Background(), domain.transfer.ProjectID, domain.transfer.ID, Actor{UserID: domain.transfer.ActorID}, 0,
			base64.StdEncoding.EncodeToString(wrong[:]), bytes.NewReader(content), int64(len(content)),
		)
		if volume.ErrorCode(err) != volume.CodeTransferChunkChecksumMismatch {
			t.Fatalf("checksum error code=%q err=%v", volume.ErrorCode(err), err)
		}
		select {
		case <-writeEntered:
			t.Fatal("database writer entered for a checksum mismatch")
		default:
		}
		assertDirectoryEmpty(t, tempDir)
	})

	t.Run("cancellation", func(t *testing.T) {
		content := []byte("cancel body")
		checksum := sha256.Sum256(content)
		writeEntered := make(chan struct{})
		domain := uploadTestDomain("vtx_cancel_slow", int64(len(content)), writeEntered)
		tempDir := t.TempDir()
		ctx, cancel := context.WithCancel(context.Background())
		reader, _ := io.Pipe()
		resultChannel := make(chan error, 1)
		go func() {
			_, _, err := NewService(domain, newMemoryVolumeStore(), NewMemoryTicketStore(), Options{MaxBytes: 1024, TempDir: tempDir}).WriteImportPart(
				ctx, domain.transfer.ProjectID, domain.transfer.ID, Actor{UserID: domain.transfer.ActorID}, 0,
				base64.StdEncoding.EncodeToString(checksum[:]), reader, int64(len(content)),
			)
			resultChannel <- err
		}()
		cancel()
		select {
		case err := <-resultChannel:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation error=%v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("cancelled upload did not stop")
		}
		select {
		case <-writeEntered:
			t.Fatal("database writer entered for a cancelled body")
		default:
		}
		assertDirectoryEmpty(t, tempDir)
	})
}

func TestUploadSpoolBudgetFailsFastAndReleasesAfterCleanup(t *testing.T) {
	t.Parallel()
	content := []byte("bounded spool body")
	checksum := sha256.Sum256(content)
	domain := uploadTestDomain("vtx_spool_budget", int64(len(content)), nil)
	store := newMemoryVolumeStore()
	store.parts[domain.transfer.ObjectKey] = map[int][]byte{}
	service := NewService(domain, store, NewMemoryTicketStore(), Options{
		MaxBytes:            1024,
		TempDir:             t.TempDir(),
		SpoolMaxBytes:       int64(len(content)),
		SpoolMinFreeBytes:   1,
		SpoolAvailableBytes: func(string) (int64, error) { return 1024 * 1024, nil },
	})
	reader, writer := io.Pipe()
	firstResult := make(chan error, 1)
	go func() {
		_, _, err := service.WriteImportPart(
			context.Background(), domain.transfer.ProjectID, domain.transfer.ID, Actor{UserID: domain.transfer.ActorID}, 0,
			base64.StdEncoding.EncodeToString(checksum[:]), reader, int64(len(content)),
		)
		firstResult <- err
	}()
	if _, err := writer.Write(content[:1]); err != nil {
		t.Fatal(err)
	}
	_, _, err := service.WriteImportPart(
		context.Background(), domain.transfer.ProjectID, domain.transfer.ID, Actor{UserID: domain.transfer.ActorID}, 0,
		base64.StdEncoding.EncodeToString(checksum[:]), bytes.NewReader(content), int64(len(content)),
	)
	if volume.ErrorCode(err) != volume.CodeTransferSpoolBusy {
		t.Fatalf("second upload code=%q err=%v", volume.ErrorCode(err), err)
	}
	if _, err := writer.Write(content[1:]); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-firstResult; err != nil {
		t.Fatalf("first upload: %v", err)
	}

	// Cleanup releases the weighted byte reservation, so a subsequent
	// idempotent retry is admitted instead of leaking process capacity.
	if _, _, err := service.WriteImportPart(
		context.Background(), domain.transfer.ProjectID, domain.transfer.ID, Actor{UserID: domain.transfer.ActorID}, 0,
		base64.StdEncoding.EncodeToString(checksum[:]), bytes.NewReader(content), int64(len(content)),
	); err != nil {
		t.Fatalf("retry after cleanup: %v", err)
	}
}

func TestUploadSpoolRejectsInsufficientDiskBeforeCreatingFile(t *testing.T) {
	t.Parallel()
	content := []byte("disk bounded body")
	checksum := sha256.Sum256(content)
	tempDir := t.TempDir()
	domain := uploadTestDomain("vtx_spool_disk", int64(len(content)), nil)
	service := NewService(domain, newMemoryVolumeStore(), NewMemoryTicketStore(), Options{
		MaxBytes:          1024,
		TempDir:           tempDir,
		SpoolMaxBytes:     1024,
		SpoolMinFreeBytes: 8,
		SpoolAvailableBytes: func(string) (int64, error) {
			return int64(len(content)) + 7, nil
		},
	})
	_, _, err := service.WriteImportPart(
		context.Background(), domain.transfer.ProjectID, domain.transfer.ID, Actor{UserID: domain.transfer.ActorID}, 0,
		base64.StdEncoding.EncodeToString(checksum[:]), bytes.NewReader(content), int64(len(content)),
	)
	if volume.ErrorCode(err) != volume.CodeTransferSpoolInsufficient {
		t.Fatalf("disk error code=%q err=%v", volume.ErrorCode(err), err)
	}
	assertDirectoryEmpty(t, tempDir)
}

func TestSpoolInitializationRemovesOnlyOldOwnedOrphans(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	now := time.Now().UTC()
	oldPart := filepath.Join(tempDir, spoolPartPrefix+"old")
	freshPart := filepath.Join(tempDir, spoolPartPrefix+"fresh")
	unrelated := filepath.Join(tempDir, "keep-me")
	for _, path := range []string{oldPart, freshPart, unrelated} {
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chtimes(oldPart, now.Add(-2*time.Hour), now.Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	_ = NewService(nil, nil, nil, Options{TempDir: tempDir, Now: func() time.Time { return now }, SpoolOrphanAge: time.Hour})
	if _, err := os.Stat(oldPart); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old orphan still exists: %v", err)
	}
	for _, path := range []string{freshPart, unrelated} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("preserved file %s: %v", path, err)
		}
	}
	if mode := (mustStat(t, tempDir).Mode() & os.ModePerm); mode != 0o700 {
		t.Fatalf("spool directory mode=%#o", mode)
	}
}

func uploadTestDomain(transferID string, expectedBytes int64, writeEntered chan struct{}) *transferDomainStub {
	return &transferDomainStub{writeEntered: writeEntered, transfer: model.VolumeTransfer{
		ID: transferID, ProjectID: "prj_demo", Direction: model.VolumeTransferDirectionImport,
		State: model.VolumeTransferStateUploading, ObjectKey: "transfers/upload-test", MultipartUploadID: "upload-demo",
		ExpectedBytes: expectedBytes, ActorID: "usr_demo", ExpiresAt: time.Now().Add(time.Hour),
	}}
}

func assertDirectoryEmpty(t *testing.T, path string) {
	t.Helper()
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("temporary files were not cleaned up: %#v", entries)
	}
}

type trackingReader struct {
	read bool
}

func (reader *trackingReader) Read([]byte) (int, error) {
	reader.read = true
	return 0, errors.New("unexpected body read")
}

func mustStat(t *testing.T, path string) os.FileInfo {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info
}

func TestDownloadTicketIsBoundOneTimeAndSupportsRange(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	content := []byte("hello volume")
	checksum := sha256.Sum256(content)
	transfer := model.VolumeTransfer{
		ID: "vtx_export", ProjectID: "prj_demo", Direction: model.VolumeTransferDirectionExport,
		Format: model.VolumeTransferFormatTarGZ, State: model.VolumeTransferStateSucceeded,
		ObjectKey: "transfers/export-demo", ExpectedBytes: int64(len(content)), SHA256: hex.EncodeToString(checksum[:]),
		ActorID: "usr_demo", ExpiresAt: now.Add(time.Hour),
	}
	domain := &transferDomainStub{transfer: transfer}
	store := newMemoryVolumeStore()
	store.objects[transfer.ObjectKey] = content
	ticketStore := NewMemoryTicketStore()
	service := NewService(domain, store, ticketStore, Options{MaxBytes: 1024, Now: func() time.Time { return now }})
	binding := DownloadBinding{UserID: "usr_demo", SubjectID: "ses_demo", AssertionID: "mfa_demo", AssertionRequired: true, Deadline: now.Add(time.Hour)}
	authorization, err := service.AuthorizeDownload(context.Background(), Actor{UserID: "usr_demo"}, transfer, binding)
	if err != nil || authorization.Ticket == "" {
		t.Fatalf("authorize download=%#v err=%v", authorization, err)
	}
	for key := range ticketStore.values {
		if strings.Contains(key, authorization.Ticket) {
			t.Fatalf("raw download ticket was retained in store key: %q", key)
		}
	}
	if _, _, err := service.HeadDownload(context.Background(), Actor{UserID: "usr_demo"}, transfer, DownloadCredential{}, binding); volume.ErrorCode(err) != volume.CodeTransferDownloadUnauthorized {
		t.Fatalf("missing credential code=%q err=%v", volume.ErrorCode(err), err)
	}
	info, session, err := service.HeadDownload(context.Background(), Actor{UserID: "usr_demo"}, transfer, DownloadCredential{Ticket: authorization.Ticket}, binding)
	if err != nil {
		t.Fatalf("exchange ticket with HEAD: %v", err)
	}
	if info.Size != int64(len(content)) || session.Token == "" || session.ExpiresAt.After(now.Add(30*time.Minute)) {
		t.Fatalf("head info=%#v session=%#v", info, session)
	}
	for key := range ticketStore.values {
		if strings.Contains(key, session.Token) {
			t.Fatalf("raw download session was retained in store key: %q", key)
		}
	}
	if _, _, err := service.OpenDownload(context.Background(), Actor{UserID: "usr_demo"}, transfer, DownloadCredential{Ticket: authorization.Ticket}, "", binding); volume.ErrorCode(err) != volume.CodeTransferExpired {
		t.Fatalf("ticket replay code=%q err=%v", volume.ErrorCode(err), err)
	}

	download, _, err := service.OpenDownload(context.Background(), Actor{UserID: "usr_demo"}, transfer, DownloadCredential{Session: session.Token}, "bytes=6-11", binding)
	if err != nil {
		t.Fatalf("open first session range: %v", err)
	}
	body, _ := io.ReadAll(download.Body)
	_ = download.Body.Close()
	if string(body) != "volume" || download.Status != 206 || download.ContentRange != "bytes 6-11/12" {
		t.Fatalf("download body=%q status=%d range=%q", body, download.Status, download.ContentRange)
	}
	download, _, err = service.OpenDownload(context.Background(), Actor{UserID: "usr_demo"}, transfer, DownloadCredential{Session: session.Token}, "bytes=0-4", binding)
	if err != nil {
		t.Fatalf("open second session range: %v", err)
	}
	body, _ = io.ReadAll(download.Body)
	_ = download.Body.Close()
	if string(body) != "hello" || download.ContentRange != "bytes 0-4/12" {
		t.Fatalf("second range body=%q range=%q", body, download.ContentRange)
	}
	for name, candidate := range map[string]DownloadBinding{
		"user":      {UserID: "usr_other", SubjectID: binding.SubjectID, AssertionID: binding.AssertionID, AssertionRequired: true, Deadline: binding.Deadline},
		"session":   {UserID: binding.UserID, SubjectID: "ses_other", AssertionID: binding.AssertionID, AssertionRequired: true, Deadline: binding.Deadline},
		"assertion": {UserID: binding.UserID, SubjectID: binding.SubjectID, AssertionID: "mfa_other", AssertionRequired: true, Deadline: binding.Deadline},
	} {
		if _, _, err := service.OpenDownload(context.Background(), Actor{UserID: "usr_demo"}, transfer, DownloadCredential{Session: session.Token}, "", candidate); volume.ErrorCode(err) != volume.CodeTransferDownloadUnauthorized {
			t.Fatalf("%s session binding code=%q err=%v", name, volume.ErrorCode(err), err)
		}
	}
	otherTransfer := transfer
	otherTransfer.ID = "vtx_other"
	if _, _, err := service.OpenDownload(context.Background(), Actor{UserID: "usr_demo"}, otherTransfer, DownloadCredential{Session: session.Token}, "", binding); volume.ErrorCode(err) != volume.CodeTransferDownloadUnauthorized {
		t.Fatalf("transfer session binding code=%q err=%v", volume.ErrorCode(err), err)
	}
	otherTransfer = transfer
	otherTransfer.ProjectID = "prj_other"
	if _, _, err := service.OpenDownload(context.Background(), Actor{UserID: "usr_demo"}, otherTransfer, DownloadCredential{Session: session.Token}, "", binding); volume.ErrorCode(err) != volume.CodeTransferDownloadUnauthorized {
		t.Fatalf("project session binding code=%q err=%v", volume.ErrorCode(err), err)
	}

	boundAuthorization, err := service.AuthorizeDownload(context.Background(), Actor{UserID: "usr_demo"}, transfer, binding)
	if err != nil {
		t.Fatalf("authorize bound download: %v", err)
	}
	wrongBinding := binding
	wrongBinding.SubjectID = "ses_other"
	if _, _, err := service.OpenDownload(context.Background(), Actor{UserID: "usr_demo"}, transfer, DownloadCredential{Ticket: boundAuthorization.Ticket}, "", wrongBinding); volume.ErrorCode(err) != volume.CodeTransferDownloadUnauthorized {
		t.Fatalf("binding mismatch code=%q err=%v", volume.ErrorCode(err), err)
	}

	rotationAuthorization, err := service.AuthorizeDownload(context.Background(), Actor{UserID: "usr_demo"}, transfer, binding)
	if err != nil {
		t.Fatalf("authorize rotation: %v", err)
	}
	_, rotatedSession, err := service.HeadDownload(context.Background(), Actor{UserID: "usr_demo"}, transfer,
		DownloadCredential{Ticket: rotationAuthorization.Ticket, Session: session.Token}, binding)
	if err != nil || rotatedSession.Token == "" || rotatedSession.Token == session.Token {
		t.Fatalf("ticket did not take precedence over cookie: session=%#v err=%v", rotatedSession, err)
	}

	now = now.Add(31 * time.Minute)
	if _, _, err := service.OpenDownload(context.Background(), Actor{UserID: "usr_demo"}, transfer, DownloadCredential{Session: rotatedSession.Token}, "", binding); volume.ErrorCode(err) != volume.CodeTransferExpired {
		t.Fatalf("expired session code=%q err=%v", volume.ErrorCode(err), err)
	}
}

func TestBlockManifestUsesTheContentTicketSessionAndContainsOnlyPortableMetadata(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 15, 8, 30, 0, 0, time.UTC)
	finishedAt := now.Add(-2 * time.Minute)
	archive := []byte("compressed raw block")
	archiveDigest := sha256.Sum256(archive)
	dataSHA := strings.Repeat("b", 64)
	transfer := model.VolumeTransfer{
		ID: "vtx_block", ProjectID: "prj_secret", ProjectVolumeID: "pvol_secret",
		Direction: model.VolumeTransferDirectionExport, Format: model.VolumeTransferFormatRawZST,
		ConsistencyMode: model.VolumeTransferConsistencySnapshot, State: model.VolumeTransferStateSucceeded,
		ObjectKey: "transfers/opaque", ExpectedBytes: int64(len(archive)), SHA256: hex.EncodeToString(archiveDigest[:]),
		LogicalBytes: 4096, DataSHA256: dataSHA, ActorID: "usr_secret", FinishedAt: &finishedAt,
		ExpiresAt: now.Add(time.Hour),
	}
	domain := &transferDomainStub{transfer: transfer}
	store := newMemoryVolumeStore()
	store.objects[transfer.ObjectKey] = archive
	service := NewService(domain, store, NewMemoryTicketStore(), Options{MaxBytes: 1 << 20, Now: func() time.Time { return now }})
	binding := DownloadBinding{UserID: transfer.ActorID, SubjectID: "ses_demo", AssertionID: "mfa_demo", AssertionRequired: true, Deadline: now.Add(time.Hour)}
	authorization, err := service.AuthorizeDownload(context.Background(), Actor{UserID: transfer.ActorID}, transfer, binding)
	if err != nil {
		t.Fatalf("authorize manifest download: %v", err)
	}
	info, session, err := service.HeadManifest(context.Background(), Actor{UserID: transfer.ActorID}, transfer, DownloadCredential{Ticket: authorization.Ticket}, binding)
	if err != nil || info.Size < 1 || info.ETag == "" || session.Token == "" {
		t.Fatalf("manifest HEAD info=%#v session=%#v err=%v", info, session, err)
	}
	download, _, err := service.OpenManifest(context.Background(), Actor{UserID: transfer.ActorID}, transfer, DownloadCredential{Session: session.Token}, binding)
	if err != nil {
		t.Fatalf("manifest GET: %v", err)
	}
	content, readErr := io.ReadAll(download.Body)
	_ = download.Body.Close()
	if readErr != nil || int64(len(content)) != info.Size || download.ETag != info.ETag || download.ContentType != blockManifestContentType {
		t.Fatalf("manifest download=%#v bytes=%d readErr=%v", download, len(content), readErr)
	}
	var payload map[string]any
	if err := json.Unmarshal(content, &payload); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if len(payload) != 8 || payload["schemaVersion"] != float64(1) || payload["volumeMode"] != model.ProjectVolumeModeBlock ||
		payload["format"] != model.VolumeTransferFormatRawZST || payload["logicalBytes"] != float64(4096) ||
		payload["fileCount"] != float64(0) || payload["dataSHA256"] != dataSHA ||
		payload["consistencyMode"] != model.VolumeTransferConsistencySnapshot || payload["exportedAt"] != finishedAt.Format(time.RFC3339) {
		t.Fatalf("manifest payload=%#v", payload)
	}
	for _, forbidden := range []string{"project", "user", "cluster", "secret", "url", "object"} {
		if strings.Contains(strings.ToLower(string(content)), forbidden) {
			t.Fatalf("manifest contains forbidden metadata %q: %s", forbidden, content)
		}
	}
}

func TestBlockManifestRejectsFilesystemAndIncompleteLegacyResults(t *testing.T) {
	t.Parallel()
	finishedAt := time.Now().UTC()
	for name, transfer := range map[string]model.VolumeTransfer{
		"filesystem": {
			Direction: model.VolumeTransferDirectionExport, Format: model.VolumeTransferFormatTarGZ,
			State: model.VolumeTransferStateSucceeded, FinishedAt: &finishedAt,
		},
		"missing raw digest": {
			Direction: model.VolumeTransferDirectionExport, Format: model.VolumeTransferFormatRawZST,
			State: model.VolumeTransferStateSucceeded, FinishedAt: &finishedAt,
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := blockManifestFor(transfer); volume.ErrorCode(err) == "" {
				t.Fatalf("manifest error=%v", err)
			}
		})
	}
}

func TestInternalCallbackTokenIsConstantTimeBoundAndCompletionIdempotent(t *testing.T) {
	t.Parallel()
	content := []byte("internal import")
	checksum := sha256.Sum256(content)
	rawToken := strings.Repeat("callback-token-", 3)
	tokenHash := sha256.Sum256([]byte(rawToken))
	expiresAt := time.Now().Add(time.Hour)
	transfer := model.VolumeTransfer{
		ID: "vtx_internal", ProjectID: "prj_demo", ProjectVolumeID: "pvol_demo",
		Direction: model.VolumeTransferDirectionImport, Format: model.VolumeTransferFormatTarGZ,
		State: model.VolumeTransferStateRunning, ObjectKey: "transfers/internal-demo",
		ExpectedBytes: int64(len(content)), SHA256: hex.EncodeToString(checksum[:]),
		CallbackTokenHash: hex.EncodeToString(tokenHash[:]), CallbackTokenExpiresAt: &expiresAt,
	}
	domain := &transferDomainStub{transfer: transfer}
	store := newMemoryVolumeStore()
	store.objects[transfer.ObjectKey] = content
	service := NewService(domain, store, NewMemoryTicketStore(), Options{MaxBytes: 1024})
	if _, err := service.InternalHead(context.Background(), transfer.ID, strings.Repeat("wrong-token-", 4)); volume.ErrorCode(err) != volume.CodeTransferCallbackUnauthorized {
		t.Fatalf("invalid token code=%q err=%v", volume.ErrorCode(err), err)
	}
	completed, err := service.InternalComplete(context.Background(), transfer.ID, rawToken, Completion{
		ExpectedState: model.VolumeTransferStateRunning, TransferredBytes: int64(len(content)), SHA256: hex.EncodeToString(checksum[:]),
	})
	if err != nil || completed.State != model.VolumeTransferStateRunning || completed.CompletionReportedAt == nil {
		t.Fatalf("internal complete result=%#v err=%v", completed, err)
	}
	replayed, err := service.InternalComplete(context.Background(), transfer.ID, rawToken, Completion{
		ExpectedState: model.VolumeTransferStateRunning, TransferredBytes: int64(len(content)), SHA256: hex.EncodeToString(checksum[:]),
	})
	if err != nil || replayed.State != model.VolumeTransferStateRunning || replayed.CompletionReportedAt == nil {
		t.Fatalf("internal complete replay result=%#v err=%v", replayed, err)
	}
}

func TestInternalFailureCallbackUsesAuthoritativeAtomicFailure(t *testing.T) {
	t.Parallel()
	rawToken := strings.Repeat("failure-callback-token-", 2)
	tokenHash := sha256.Sum256([]byte(rawToken))
	expiresAt := time.Now().Add(time.Hour)
	transfer := model.VolumeTransfer{
		ID: "vtx_internal_failure", ProjectID: "prj_demo", ProjectVolumeID: "pvol_demo",
		Direction: model.VolumeTransferDirectionImport, Format: model.VolumeTransferFormatTarGZ,
		State: model.VolumeTransferStateRunning, ObjectKey: "transfers/internal-failure",
		CallbackTokenHash: hex.EncodeToString(tokenHash[:]), CallbackTokenExpiresAt: &expiresAt,
	}
	domain := &transferDomainStub{transfer: transfer}
	service := NewService(domain, newMemoryVolumeStore(), NewMemoryTicketStore(), Options{MaxBytes: 1024})
	failed, err := service.InternalFail(context.Background(), transfer.ID, rawToken, Failure{
		ExpectedState: model.VolumeTransferStateRunning,
		ErrorCode:     volume.CodeTransferJobFailed,
		Diagnostic:    "trusted transfer Job diagnostic",
	})
	if err != nil || failed.State != model.VolumeTransferStateFailed || failed.LastErrorCode != volume.CodeTransferJobFailed {
		t.Fatalf("internal failure result=%#v err=%v", failed, err)
	}
	replayed, err := service.InternalFail(context.Background(), transfer.ID, rawToken, Failure{
		ExpectedState: model.VolumeTransferStateRunning,
		ErrorCode:     volume.CodeTransferJobFailed,
		Diagnostic:    "trusted transfer Job diagnostic",
	})
	if err != nil || replayed.State != model.VolumeTransferStateFailed {
		t.Fatalf("internal failure replay=%#v err=%v", replayed, err)
	}
}

func TestRawImportCompletionReplayRejectsDifferentServerObservedDigest(t *testing.T) {
	t.Parallel()
	content := []byte("compressed block archive")
	checksum := sha256.Sum256(content)
	rawToken := strings.Repeat("raw-callback-token-", 2)
	tokenHash := sha256.Sum256([]byte(rawToken))
	expiresAt := time.Now().Add(time.Hour)
	transfer := model.VolumeTransfer{
		ID: "vtx_raw_internal", ProjectID: "prj_demo", ProjectVolumeID: "pvol_demo",
		Direction: model.VolumeTransferDirectionImport, Format: model.VolumeTransferFormatRawZST,
		State: model.VolumeTransferStateRunning, ObjectKey: "transfers/raw-internal-demo",
		ExpectedBytes: int64(len(content)), SHA256: hex.EncodeToString(checksum[:]),
		CallbackTokenHash: hex.EncodeToString(tokenHash[:]), CallbackTokenExpiresAt: &expiresAt,
	}
	domain := &transferDomainStub{transfer: transfer}
	store := newMemoryVolumeStore()
	store.objects[transfer.ObjectKey] = content
	service := NewService(domain, store, NewMemoryTicketStore(), Options{MaxBytes: 1024})
	completion := Completion{
		ExpectedState: model.VolumeTransferStateRunning, TransferredBytes: int64(len(content)),
		SHA256: hex.EncodeToString(checksum[:]), LogicalBytes: 8192, DataSHA256: strings.Repeat("a", 64),
	}
	completed, err := service.InternalComplete(context.Background(), transfer.ID, rawToken, completion)
	if err != nil || completed.State != model.VolumeTransferStateRunning || completed.CompletionReportedAt == nil || completed.LogicalBytes != completion.LogicalBytes || completed.DataSHA256 != completion.DataSHA256 {
		t.Fatalf("raw internal complete result=%#v err=%v", completed, err)
	}
	completion.DataSHA256 = strings.Repeat("b", 64)
	if _, err := service.InternalComplete(context.Background(), transfer.ID, rawToken, completion); volume.ErrorCode(err) != volume.CodeTransferStateConflict {
		t.Fatalf("different raw replay code=%q err=%v", volume.ErrorCode(err), err)
	}
}

func TestUnmountedExportConsistencyRejectsReservedOrActiveVolume(t *testing.T) {
	t.Parallel()
	for name, summary := range map[string]model.ProjectVolumeBindingSummary{
		"reserved": {Reserved: 1},
		"active":   {Active: 1},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := exportConsistency(model.ProjectVolume{BindingSummary: summary}, model.VolumeTransferConsistencyUnmounted)
			if volume.ErrorCode(err) != volume.CodeTransferStateConflict {
				t.Fatalf("consistency code=%q err=%v", volume.ErrorCode(err), err)
			}
		})
	}
	consistency, err := exportConsistency(model.ProjectVolume{}, model.VolumeTransferConsistencyUnmounted)
	if err != nil || consistency != model.VolumeTransferConsistencyUnmounted {
		t.Fatalf("unmounted idle consistency=%q err=%v", consistency, err)
	}
	if _, err := exportConsistency(model.ProjectVolume{VolumeMode: model.ProjectVolumeModeBlock}, model.VolumeTransferConsistencyLive); volume.ErrorCode(err) != volume.CodeTransferStateConflict {
		t.Fatalf("block live export code=%q err=%v", volume.ErrorCode(err), err)
	}
}
