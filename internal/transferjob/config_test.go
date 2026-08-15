package transferjob

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/volumetransfer"
	"go.opentelemetry.io/otel/trace"
)

func TestConfigFromLookupValidatesTransferModeAndPaths(t *testing.T) {
	environment := validEnvironment(t, DirectionImport, ModeFilesystem)
	config, err := ConfigFromLookup(func(key string) string { return environment[key] })
	if err != nil {
		t.Fatalf("ConfigFromLookup returned error: %v", err)
	}
	if config.Format != FormatTarGZ || config.CapacityBytes != 1024*1024 || config.MaxFiles != defaultMaxFiles {
		t.Fatalf("config = %#v", config)
	}

	environment["LUNA_VOLUME_TRANSFER_FORMAT"] = FormatRawZST
	if _, err := ConfigFromLookup(func(key string) string { return environment[key] }); err == nil {
		t.Fatal("filesystem/raw_zst mismatch was accepted")
	}
	environment["LUNA_VOLUME_TRANSFER_FORMAT"] = FormatTarGZ
	environment["LUNA_VOLUME_TRANSFER_CHUNK_SIZE"] = "1024"
	if _, err := ConfigFromLookup(func(key string) string { return environment[key] }); err == nil {
		t.Fatal("unaligned chunk size was accepted")
	}
	environment["LUNA_VOLUME_TRANSFER_CHUNK_SIZE"] = "67108864"
	environment["LUNA_VOLUME_TRANSFER_DATA_PATH"] = "relative/path"
	if _, err := ConfigFromLookup(func(key string) string { return environment[key] }); err == nil {
		t.Fatal("relative data path was accepted")
	}
}

func TestConfigFromLookupAcceptsFiveTiBChunkContract(t *testing.T) {
	environment := validEnvironment(t, DirectionImport, ModeFilesystem)
	environment["LUNA_VOLUME_TRANSFER_CAPACITY_BYTES"] = "5497558138880"
	environment["LUNA_VOLUME_TRANSFER_EXPECTED_BYTES"] = "5497558138880"
	environment["LUNA_VOLUME_TRANSFER_CHUNK_SIZE"] = "550502400"
	config, err := ConfigFromLookup(func(key string) string { return environment[key] })
	if err != nil {
		t.Fatalf("ConfigFromLookup returned error: %v", err)
	}
	if config.ChunkSize != 525*1024*1024 {
		t.Fatalf("chunk size = %d, want 525 MiB", config.ChunkSize)
	}
	if parts := (config.ExpectedBytes + config.ChunkSize - 1) / config.ChunkSize; parts > volumetransfer.MaxMultipartParts {
		t.Fatalf("multipart parts = %d, want <= %d", parts, volumetransfer.MaxMultipartParts)
	}
}

func TestContextWithRemoteTraceRejectsAnythingButValidW3CContext(t *testing.T) {
	const parent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	ctx, err := ContextWithRemoteTrace(context.Background(), parent, "vendor=value")
	if err != nil {
		t.Fatalf("ContextWithRemoteTrace returned error: %v", err)
	}
	spanContext := trace.SpanContextFromContext(ctx)
	if !spanContext.IsValid() || !spanContext.IsRemote() || spanContext.TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("span context = %#v", spanContext)
	}
	if _, err := ContextWithRemoteTrace(context.Background(), "invalid", ""); err == nil {
		t.Fatal("invalid traceparent was accepted")
	}
	if _, err := ContextWithRemoteTrace(context.Background(), "", "vendor=value"); err == nil {
		t.Fatal("orphan tracestate was accepted")
	}
}

func TestReadTokenFileRejectsWhitespaceAndClearsOnClose(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte(strings.Repeat("a", 32)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readTokenFile(tokenFile); ErrorCode(err) != CodeCallbackUnauthorized {
		t.Fatalf("whitespace token error = %v", err)
	}
	if err := os.WriteFile(tokenFile, append([]byte(strings.Repeat("a", 31)), 0xff), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readTokenFile(tokenFile); ErrorCode(err) != CodeCallbackUnauthorized {
		t.Fatalf("non-ASCII token error = %v", err)
	}
	if err := os.WriteFile(tokenFile, []byte(strings.Repeat("a", 32)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(tokenFile, 0o620); err != nil {
		t.Fatal(err)
	}
	if _, err := readTokenFile(tokenFile); ErrorCode(err) != CodeCallbackUnauthorized {
		t.Fatalf("writable token file error = %v", err)
	}
	if err := os.Chmod(tokenFile, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tokenFile, []byte(strings.Repeat("a", 32)), 0o600); err != nil {
		t.Fatal(err)
	}
	environment := validEnvironment(t, DirectionImport, ModeFilesystem)
	environment["LUNA_VOLUME_TRANSFER_TOKEN_FILE"] = tokenFile
	config, err := ConfigFromLookup(func(key string) string { return environment[key] })
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(config, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	for _, value := range client.token {
		if value != 0 {
			t.Fatal("token bytes were not cleared")
		}
	}
}

func validEnvironment(t *testing.T, direction, volumeMode string) map[string]string {
	t.Helper()
	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte(strings.Repeat("a", 32)), 0o600); err != nil {
		t.Fatal(err)
	}
	format := FormatTarGZ
	if volumeMode == ModeBlock {
		format = FormatRawZST
	}
	values := map[string]string{
		"LUNA_VOLUME_TRANSFER_ID":                "vtx_test",
		"LUNA_VOLUME_TRANSFER_DIRECTION":         direction,
		"LUNA_VOLUME_TRANSFER_FORMAT":            format,
		"LUNA_VOLUME_TRANSFER_VOLUME_MODE":       volumeMode,
		"LUNA_VOLUME_TRANSFER_CONSISTENCY_MODE":  "unmounted",
		"LUNA_VOLUME_TRANSFER_CALLBACK_BASE_URL": "https://api.example.invalid",
		"LUNA_VOLUME_TRANSFER_TOKEN_FILE":        tokenFile,
		"LUNA_VOLUME_TRANSFER_DATA_PATH":         filepath.Join(t.TempDir(), "volume"),
		"LUNA_VOLUME_TRANSFER_CAPACITY_BYTES":    "1048576",
		"LUNA_VOLUME_TRANSFER_EXPECTED_BYTES":    "1024",
		"LUNA_VOLUME_TRANSFER_EXPECTED_SHA256":   strings.Repeat("a", 64),
		"LUNA_VOLUME_TRANSFER_CHUNK_SIZE":        "67108864",
	}
	if direction == DirectionExport {
		values["LUNA_VOLUME_TRANSFER_EXPORTED_AT"] = time.Unix(1_700_000_000, 0).UTC().Format(time.RFC3339Nano)
	}
	return values
}
