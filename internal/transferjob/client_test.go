package transferjob

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
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
)

func TestClientUsesFileTokenTUSChecksumAndTraceContext(t *testing.T) {
	const token = "abcdefghijklmnopqrstuvwxyz012345"
	var mutex sync.Mutex
	var uploaded []byte
	var sawTraceparent bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/internal/v1/volume-transfers/vtx_test/content" && !strings.HasPrefix(request.URL.Path, "/internal/v1/volume-transfers/vtx_test/") {
			http.NotFound(response, request)
			return
		}
		if request.Header.Get("Authorization") != "Bearer "+token {
			response.WriteHeader(http.StatusUnauthorized)
			return
		}
		if request.Header.Get("traceparent") != "" {
			sawTraceparent = true
		}
		switch {
		case request.Method == http.MethodHead:
			mutex.Lock()
			offset := len(uploaded)
			mutex.Unlock()
			response.Header().Set("Upload-Offset", strconv.Itoa(offset))
			response.Header().Set("Upload-Length", strconv.Itoa(offset))
			response.Header().Set("Upload-Chunk-Size", strconv.FormatInt(minimumChunkSize, 10))
			response.WriteHeader(http.StatusOK)
		case request.Method == http.MethodPatch:
			if request.ContentLength < 1 {
				t.Errorf("missing content length: %d", request.ContentLength)
			}
			content, _ := io.ReadAll(request.Body)
			digest := sha256.Sum256(content)
			if request.Header.Get("Upload-Checksum") != "sha256 "+base64.StdEncoding.EncodeToString(digest[:]) {
				response.WriteHeader(http.StatusBadRequest)
				return
			}
			mutex.Lock()
			if request.Header.Get("Upload-Offset") != strconv.Itoa(len(uploaded)) {
				mutex.Unlock()
				response.WriteHeader(http.StatusConflict)
				return
			}
			uploaded = append(uploaded, content...)
			next := len(uploaded)
			mutex.Unlock()
			response.Header().Set("Upload-Offset", strconv.Itoa(next))
			response.WriteHeader(http.StatusNoContent)
		default:
			response.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()

	config := testClientConfig(t, server.URL, token)
	client, err := NewClient(config, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx, err := ContextWithRemoteTrace(context.Background(), "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01", "")
	if err != nil {
		t.Fatal(err)
	}
	info, err := client.HeadContent(ctx)
	if err != nil || info.Offset != 0 {
		t.Fatalf("HeadContent = %#v, %v", info, err)
	}
	if next, err := client.WritePart(ctx, 0, []byte("hello")); err != nil || next != 5 {
		t.Fatalf("WritePart = %d, %v", next, err)
	}
	if err := client.Progress(ctx, ProgressInput{ExpectedState: "running", TransferredBytes: 5, Stage: "uploading"}); err != nil {
		t.Fatal(err)
	}
	checksum := sha256.Sum256([]byte("hello"))
	if err := client.Complete(ctx, CompleteInput{ExpectedState: "running", TransferredBytes: 5, SHA256: strconv.FormatUint(0, 10)}); ErrorCode(err) != CodeChecksumMismatch {
		t.Fatalf("invalid Complete checksum error = %v", err)
	}
	if err := client.Complete(ctx, CompleteInput{ExpectedState: "running", TransferredBytes: 5, SHA256: stringHex(checksum[:])}); err != nil {
		t.Fatal(err)
	}
	if string(uploaded) != "hello" || !sawTraceparent {
		t.Fatalf("uploaded=%q sawTraceparent=%t", uploaded, sawTraceparent)
	}
}

func TestClientDoesNotFollowRedirectWithAuthorization(t *testing.T) {
	const token = "abcdefghijklmnopqrstuvwxyz012345"
	redirectTargetCalled := false
	target := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirectTargetCalled = true }))
	defer target.Close()
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		http.Redirect(response, request, target.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	config := testClientConfig(t, server.URL, token)
	client, err := NewClient(config, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err := client.HeadContent(context.Background()); err == nil {
		t.Fatal("redirect response was accepted")
	}
	if redirectTargetCalled {
		t.Fatal("authorization-bearing request followed redirect")
	}
}

func TestClientCancellationTerminatesCallbackRequest(t *testing.T) {
	const token = "abcdefghijklmnopqrstuvwxyz012345"
	started := make(chan struct{})
	cancelled := make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(started)
		<-request.Context().Done()
		close(cancelled)
	}))
	defer server.Close()
	client, err := NewClient(testClientConfig(t, server.URL, token), server.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := client.HeadContent(ctx)
		result <- err
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("callback request did not start")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("HeadContent cancellation error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("HeadContent did not stop after cancellation")
	}
	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("callback server did not observe request cancellation")
	}
}

func TestRemoteErrorOnlyPreservesStableCode(t *testing.T) {
	response := httptest.NewRecorder()
	response.Code = http.StatusInternalServerError
	response.Body.WriteString(`{"code":"../../secret","detail":"/private/path"}`)
	err := remoteResponseError(response.Result())
	if ErrorCode(err) != CodeCallbackUnavailable || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "private") {
		t.Fatalf("remote error leaked response content: %v", err)
	}
}

func testClientConfig(t *testing.T, baseURL, token string) Config {
	t.Helper()
	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte(token), 0o600); err != nil {
		t.Fatal(err)
	}
	return Config{
		TransferID: "vtx_test", Direction: DirectionImport, Format: FormatTarGZ, VolumeMode: ModeFilesystem,
		CallbackBaseURL: baseURL, TokenFile: tokenFile, DataPath: filepath.Join(t.TempDir(), "volume"),
		CapacityBytes: 1024 * 1024, ExpectedBytes: 1, ExpectedSHA256: strings.Repeat("a", 64), MaxFiles: 100, ChunkSize: minimumChunkSize,
	}
}

func stringHex(value []byte) string {
	const alphabet = "0123456789abcdef"
	result := make([]byte, len(value)*2)
	for index, item := range value {
		result[index*2] = alphabet[item>>4]
		result[index*2+1] = alphabet[item&0x0f]
	}
	return string(result)
}
