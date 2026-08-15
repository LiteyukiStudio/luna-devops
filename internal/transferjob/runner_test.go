package transferjob

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/volumetransfer"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type transferCallbackFixture struct {
	t             *testing.T
	token         string
	direction     string
	content       []byte
	chunkSize     int64
	mutex         sync.Mutex
	progress      []ProgressInput
	complete      *CompleteInput
	failure       *FailInput
	authorization []string
}

func (fixture *transferCallbackFixture) handler(response http.ResponseWriter, request *http.Request) {
	fixture.mutex.Lock()
	fixture.authorization = append(fixture.authorization, request.Header.Get("Authorization"))
	fixture.mutex.Unlock()
	if request.Header.Get("Authorization") != "Bearer "+fixture.token {
		response.WriteHeader(http.StatusUnauthorized)
		return
	}
	action := filepath.Base(request.URL.Path)
	switch {
	case request.Method == http.MethodHead && action == "content":
		fixture.mutex.Lock()
		offset := len(fixture.content)
		if fixture.direction == DirectionImport {
			response.Header().Set("Upload-Length", strconv.Itoa(offset))
		} else {
			response.Header().Set("Upload-Length", "0")
		}
		response.Header().Set("Upload-Offset", strconv.Itoa(offset))
		chunkSize := fixture.chunkSize
		if chunkSize == 0 {
			chunkSize = minimumChunkSize
		}
		response.Header().Set("Upload-Chunk-Size", strconv.FormatInt(chunkSize, 10))
		fixture.mutex.Unlock()
		response.WriteHeader(http.StatusOK)
	case request.Method == http.MethodGet && action == "content":
		fixture.mutex.Lock()
		content := append([]byte(nil), fixture.content...)
		fixture.mutex.Unlock()
		response.Header().Set("Content-Length", strconv.Itoa(len(content)))
		response.WriteHeader(http.StatusOK)
		_, _ = response.Write(content)
	case request.Method == http.MethodPatch && action == "content":
		content, _ := io.ReadAll(request.Body)
		fixture.mutex.Lock()
		expectedOffset := len(fixture.content)
		if request.Header.Get("Upload-Offset") != strconv.Itoa(expectedOffset) {
			fixture.mutex.Unlock()
			response.WriteHeader(http.StatusConflict)
			return
		}
		fixture.content = append(fixture.content, content...)
		next := len(fixture.content)
		fixture.mutex.Unlock()
		response.Header().Set("Upload-Offset", strconv.Itoa(next))
		response.WriteHeader(http.StatusNoContent)
	case request.Method == http.MethodPost && action == "progress":
		var input ProgressInput
		_ = json.NewDecoder(request.Body).Decode(&input)
		fixture.mutex.Lock()
		fixture.progress = append(fixture.progress, input)
		fixture.mutex.Unlock()
		response.WriteHeader(http.StatusNoContent)
	case request.Method == http.MethodPost && action == "complete":
		var input CompleteInput
		_ = json.NewDecoder(request.Body).Decode(&input)
		fixture.mutex.Lock()
		fixture.complete = &input
		fixture.mutex.Unlock()
		response.WriteHeader(http.StatusOK)
		_, _ = response.Write([]byte(`{"state":"succeeded"}`))
	case request.Method == http.MethodPost && action == "fail":
		var input FailInput
		_ = json.NewDecoder(request.Body).Decode(&input)
		fixture.mutex.Lock()
		fixture.failure = &input
		fixture.mutex.Unlock()
		response.WriteHeader(http.StatusOK)
	default:
		http.NotFound(response, request)
	}
}

func TestRunnerImportsFilesystemArchiveAndReportsCompletion(t *testing.T) {
	archive := makeFilesystemArchive(t, map[string]string{"db/data.txt": "payload"})
	digest := sha256.Sum256(archive)
	fixture := &transferCallbackFixture{t: t, token: strings.Repeat("i", 32), direction: DirectionImport, content: archive}
	server := httptest.NewTLSServer(http.HandlerFunc(fixture.handler))
	defer server.Close()
	destination := t.TempDir()
	config := runnerConfig(t, server.URL, fixture.token, DirectionImport, ModeFilesystem, destination)
	config.ExpectedBytes = int64(len(archive))
	config.ExpectedSHA256 = stringHex(digest[:])
	runner, err := NewRunner(config, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer runner.Close()
	result, err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(destination, "db", "data.txt"))
	if err != nil || string(content) != "payload" {
		t.Fatalf("imported content = %q, %v", content, err)
	}
	if result.SHA256 != config.ExpectedSHA256 || result.ProcessedFiles != 2 || fixture.complete == nil || fixture.failure != nil {
		t.Fatalf("result=%#v complete=%#v failure=%#v", result, fixture.complete, fixture.failure)
	}
	assertNoTokenInCallbackPayloads(t, fixture, fixture.token)
}

func TestRunnerExportsFilesystemArchiveInResumableChunks(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "data.txt"), []byte(strings.Repeat("x", 4096)), 0o600); err != nil {
		t.Fatal(err)
	}
	fixture := &transferCallbackFixture{t: t, token: strings.Repeat("e", 32), direction: DirectionExport}
	server := httptest.NewTLSServer(http.HandlerFunc(fixture.handler))
	defer server.Close()
	config := runnerConfig(t, server.URL, fixture.token, DirectionExport, ModeFilesystem, source)
	config.ExportedAt = time.Unix(1_700_000_000, 0).UTC()
	runner, err := NewRunner(config, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer runner.Close()
	result, err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	fixture.mutex.Lock()
	archive := append([]byte(nil), fixture.content...)
	complete := fixture.complete
	fixture.mutex.Unlock()
	if int64(len(archive)) != result.TransferredBytes || complete == nil || complete.SHA256 != result.SHA256 {
		t.Fatalf("archive bytes=%d result=%#v complete=%#v", len(archive), result, complete)
	}
	destination := t.TempDir()
	if _, err := volumetransfer.ExtractTarGzip(context.Background(), bytes.NewReader(archive), destination, volumetransfer.ExtractLimits{MaxLogicalBytes: 1 << 20, MaxFiles: 100}); err != nil {
		t.Fatalf("exported archive cannot be imported: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(destination, "data.txt"))
	if err != nil || len(content) != 4096 {
		t.Fatalf("round-trip content length=%d error=%v", len(content), err)
	}
}

func TestRunnerRejectsChunkContractThatExceedsJobScratchSizing(t *testing.T) {
	source := t.TempDir()
	fixture := &transferCallbackFixture{
		t: t, token: strings.Repeat("m", 32), direction: DirectionExport,
		chunkSize: 2 * minimumChunkSize,
	}
	server := httptest.NewTLSServer(http.HandlerFunc(fixture.handler))
	defer server.Close()
	config := runnerConfig(t, server.URL, fixture.token, DirectionExport, ModeFilesystem, source)
	config.ExportedAt = time.Unix(1_700_000_000, 0).UTC()
	runner, err := NewRunner(config, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer runner.Close()
	if _, err := runner.Run(context.Background()); ErrorCode(err) != CodeStateConflict {
		t.Fatalf("chunk mismatch code=%q err=%v", ErrorCode(err), err)
	}
}

func TestRunnerReportsServerObservedBlockDigestForExportAndImport(t *testing.T) {
	original := bytes.Repeat([]byte("raw-block-data"), 257)
	dataDigest := sha256.Sum256(original)
	wantDataSHA := stringHex(dataDigest[:])

	t.Run("export", func(t *testing.T) {
		source := filepath.Join(t.TempDir(), "source.raw")
		if err := os.WriteFile(source, original, 0o600); err != nil {
			t.Fatal(err)
		}
		fixture := &transferCallbackFixture{t: t, token: strings.Repeat("b", 32), direction: DirectionExport}
		server := httptest.NewTLSServer(http.HandlerFunc(fixture.handler))
		defer server.Close()
		config := runnerConfig(t, server.URL, fixture.token, DirectionExport, ModeBlock, source)
		config.CapacityBytes = int64(len(original))
		config.ExportedAt = time.Unix(1_700_000_000, 0).UTC()
		runner, err := NewRunner(config, server.Client())
		if err != nil {
			t.Fatal(err)
		}
		defer runner.Close()
		result, err := runner.Run(context.Background())
		if err != nil || fixture.complete == nil || result.LogicalBytes != int64(len(original)) ||
			result.DataSHA256 != wantDataSHA || fixture.complete.LogicalBytes != result.LogicalBytes || fixture.complete.DataSHA256 != wantDataSHA {
			t.Fatalf("result=%#v complete=%#v err=%v", result, fixture.complete, err)
		}
	})

	t.Run("import", func(t *testing.T) {
		source := filepath.Join(t.TempDir(), "source.raw")
		if err := os.WriteFile(source, original, 0o600); err != nil {
			t.Fatal(err)
		}
		var archive bytes.Buffer
		if _, err := exportRawZST(context.Background(), source, int64(len(original)), &archive); err != nil {
			t.Fatal(err)
		}
		archiveDigest := sha256.Sum256(archive.Bytes())
		destination := filepath.Join(t.TempDir(), "destination.raw")
		if err := os.WriteFile(destination, make([]byte, len(original)), 0o600); err != nil {
			t.Fatal(err)
		}
		fixture := &transferCallbackFixture{t: t, token: strings.Repeat("c", 32), direction: DirectionImport, content: archive.Bytes()}
		server := httptest.NewTLSServer(http.HandlerFunc(fixture.handler))
		defer server.Close()
		config := runnerConfig(t, server.URL, fixture.token, DirectionImport, ModeBlock, destination)
		config.CapacityBytes = int64(len(original))
		config.ExpectedBytes = int64(archive.Len())
		config.ExpectedSHA256 = stringHex(archiveDigest[:])
		runner, err := NewRunner(config, server.Client())
		if err != nil {
			t.Fatal(err)
		}
		defer runner.Close()
		result, err := runner.Run(context.Background())
		if err != nil || fixture.complete == nil || result.LogicalBytes != int64(len(original)) ||
			result.DataSHA256 != wantDataSHA || fixture.complete.LogicalBytes != result.LogicalBytes || fixture.complete.DataSHA256 != wantDataSHA {
			t.Fatalf("result=%#v complete=%#v err=%v", result, fixture.complete, err)
		}
	})
}

func TestRunnerRejectsUnsafeArchiveAndReportsOnlyStableDiagnostic(t *testing.T) {
	archive := makeUnsafeArchive(t)
	fixture := &transferCallbackFixture{t: t, token: strings.Repeat("s", 32), direction: DirectionImport, content: archive}
	server := httptest.NewTLSServer(http.HandlerFunc(fixture.handler))
	defer server.Close()
	config := runnerConfig(t, server.URL, fixture.token, DirectionImport, ModeFilesystem, t.TempDir())
	config.ExpectedBytes = int64(len(archive))
	runner, err := NewRunner(config, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer runner.Close()
	if _, err := runner.Run(context.Background()); ErrorCode(err) != CodeArchiveUnsafe {
		t.Fatalf("unsafe archive error = %v", err)
	}
	fixture.mutex.Lock()
	failure := fixture.failure
	fixture.mutex.Unlock()
	if failure == nil || failure.ErrorCode != CodeArchiveUnsafe || strings.ContainsAny(failure.Diagnostic, "/\\:") || strings.Contains(failure.Diagnostic, "..") {
		t.Fatalf("failure callback = %#v", failure)
	}
	assertNoTokenInCallbackPayloads(t, fixture, fixture.token)
}

func TestRunnerTelemetryDoesNotContainTokenURLOrArchivePath(t *testing.T) {
	previousProvider := otel.GetTracerProvider()
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
		otel.SetTracerProvider(previousProvider)
	})

	archive := makeUnsafeArchive(t)
	token := strings.Repeat("z", 32)
	fixture := &transferCallbackFixture{t: t, token: token, direction: DirectionImport, content: archive}
	server := httptest.NewTLSServer(http.HandlerFunc(fixture.handler))
	defer server.Close()
	dataPath := t.TempDir()
	config := runnerConfig(t, server.URL, token, DirectionImport, ModeFilesystem, dataPath)
	config.ExpectedBytes = int64(len(archive))
	runner, err := NewRunner(config, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer runner.Close()
	const remoteParent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	runCtx, err := ContextWithRemoteTrace(context.Background(), remoteParent, "")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = runner.Run(runCtx)

	foundTransferSpan := false
	foundCallbackSpan := false
	for _, span := range exporter.GetSpans() {
		serialized := fmt.Sprintf("%s %#v %#v", span.Name, span.Attributes, span.Events)
		for _, forbidden := range []string{token, server.URL, dataPath, "../escape"} {
			if strings.Contains(serialized, forbidden) {
				t.Fatalf("span %q leaked forbidden transfer data %q: %s", span.Name, forbidden, serialized)
			}
		}
		if span.Name == "volume.transfer.import" {
			foundTransferSpan = true
			if span.Parent.SpanID().String() != "00f067aa0ba902b7" || span.SpanContext.TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" {
				t.Fatalf("transfer span parent chain = trace %s parent %s", span.SpanContext.TraceID(), span.Parent.SpanID())
			}
			if span.Status.Code != codes.Error {
				t.Fatalf("failed transfer span status = %#v", span.Status)
			}
		}
		if span.Name == "volume.transfer_callback" {
			foundCallbackSpan = true
			if span.SpanContext.TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" || !span.Parent.IsValid() {
				t.Fatalf("callback span parent chain = trace %s parent %#v", span.SpanContext.TraceID(), span.Parent)
			}
		}
	}
	if !foundTransferSpan {
		t.Fatal("volume.transfer.import span was not recorded")
	}
	if !foundCallbackSpan {
		t.Fatal("volume.transfer_callback child span was not recorded")
	}
}

func TestRawZSTRoundTripAndCapacityLimit(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source.raw")
	original := bytes.Repeat([]byte("block-data"), 1024)
	if err := os.WriteFile(source, original, 0o600); err != nil {
		t.Fatal(err)
	}
	wantDigest := sha256.Sum256(original)
	wantSHA256 := fmt.Sprintf("%x", wantDigest[:])
	var archive bytes.Buffer
	exported, err := exportRawZST(context.Background(), source, int64(len(original)), &archive)
	if err != nil {
		t.Fatal(err)
	}
	if exported.LogicalBytes != int64(len(original)) || exported.DataSHA256 != wantSHA256 {
		t.Fatalf("raw export result = %#v", exported)
	}
	destination := filepath.Join(t.TempDir(), "destination.raw")
	if err := os.WriteFile(destination, make([]byte, len(original)), 0o600); err != nil {
		t.Fatal(err)
	}
	imported, err := importRawZST(context.Background(), bytes.NewReader(archive.Bytes()), destination, int64(len(original)))
	if err != nil || imported.LogicalBytes != int64(len(original)) || imported.DataSHA256 != wantSHA256 {
		t.Fatalf("importRawZST = %#v, %v", imported, err)
	}
	content, _ := os.ReadFile(destination)
	if !bytes.Equal(content, original) {
		t.Fatal("raw.zst round trip changed content")
	}
	tooSmall := filepath.Join(t.TempDir(), "too-small.raw")
	if err := os.WriteFile(tooSmall, make([]byte, len(original)-1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := importRawZST(context.Background(), bytes.NewReader(archive.Bytes()), tooSmall, int64(len(original)-1)); ErrorCode(err) != CodeCapacityExceeded {
		t.Fatalf("capacity error = %v", err)
	}
}

func makeFilesystemArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	source := t.TempDir()
	for name, content := range files {
		path := filepath.Join(source, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var archive bytes.Buffer
	if _, err := volumetransfer.WriteTarGzip(context.Background(), source, &archive, volumetransfer.ExportOptions{
		ConsistencyMode: "unmounted", ExportedAt: time.Unix(1_700_000_000, 0).UTC(), MaxLogicalBytes: 1 << 20, MaxFiles: 100,
	}); err != nil {
		t.Fatal(err)
	}
	return archive.Bytes()
}

func makeUnsafeArchive(t *testing.T) []byte {
	t.Helper()
	var output bytes.Buffer
	gzipWriter := gzip.NewWriter(&output)
	tarWriter := tar.NewWriter(gzipWriter)
	content := []byte("escape")
	if err := tarWriter.WriteHeader(&tar.Header{Name: "../escape", Typeflag: tar.TypeReg, Mode: 0o600, Size: int64(len(content))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func runnerConfig(t *testing.T, baseURL, token, direction, mode, dataPath string) Config {
	t.Helper()
	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte(token), 0o600); err != nil {
		t.Fatal(err)
	}
	format := FormatTarGZ
	if mode == ModeBlock {
		format = FormatRawZST
	}
	config := Config{
		TransferID: "vtx_test", Direction: direction, Format: format, VolumeMode: mode,
		ConsistencyMode: "unmounted", CallbackBaseURL: baseURL, TokenFile: tokenFile, DataPath: dataPath,
		CapacityBytes: 1 << 20, ExpectedBytes: 0, MaxFiles: 100, ChunkSize: minimumChunkSize,
	}
	if direction == DirectionImport {
		config.ExpectedBytes = 1
		config.ExpectedSHA256 = strings.Repeat("a", 64)
	}
	return config
}

func assertNoTokenInCallbackPayloads(t *testing.T, fixture *transferCallbackFixture, token string) {
	t.Helper()
	fixture.mutex.Lock()
	payload := struct {
		Progress []ProgressInput
		Complete *CompleteInput
		Failure  *FailInput
	}{fixture.progress, fixture.complete, fixture.failure}
	fixture.mutex.Unlock()
	serialized, _ := json.Marshal(payload)
	if strings.Contains(string(serialized), token) {
		t.Fatal("callback token leaked into callback payload")
	}
}
