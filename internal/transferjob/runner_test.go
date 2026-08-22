package transferjob

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFilesystemDirectStreamRoundTrip(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "hello.txt"), []byte("hello direct stream"), 0o600); err != nil {
		t.Fatal(err)
	}
	exporter, err := NewRunner(testConfig(DirectionExport, source))
	if err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	exportResult, err := exporter.RunStream(context.Background(), nil, &archive)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(archive.Bytes())
	if exportResult.SHA256 != hex.EncodeToString(digest[:]) || exportResult.TransferredBytes != int64(archive.Len()) {
		t.Fatalf("export result = %#v", exportResult)
	}
	destination := t.TempDir()
	importConfig := testConfig(DirectionImport, destination)
	importConfig.ExpectedBytes = int64(archive.Len())
	importConfig.ExpectedSHA256 = exportResult.SHA256
	importer, err := NewRunner(importConfig)
	if err != nil {
		t.Fatal(err)
	}
	importResult, err := importer.RunStream(context.Background(), bytes.NewReader(archive.Bytes()), nil)
	if err != nil {
		t.Fatal(err)
	}
	if importResult.SHA256 != exportResult.SHA256 || importResult.ProcessedFiles != 1 {
		t.Fatalf("import result = %#v", importResult)
	}
	content, err := os.ReadFile(filepath.Join(destination, "hello.txt"))
	if err != nil || string(content) != "hello direct stream" {
		t.Fatalf("imported content = %q, %v", content, err)
	}
}

func TestImportRejectsChecksumAndTrailingBytes(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "file.txt"), []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	exporter, _ := NewRunner(testConfig(DirectionExport, source))
	var archive bytes.Buffer
	result, err := exporter.RunStream(context.Background(), nil, &archive)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name     string
		content  []byte
		expected string
	}{
		{name: "checksum", content: archive.Bytes(), expected: strings.Repeat("0", 64)},
		{name: "trailing", content: append(append([]byte(nil), archive.Bytes()...), 'x'), expected: result.SHA256},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			config := testConfig(DirectionImport, t.TempDir())
			config.ExpectedBytes = int64(archive.Len())
			config.ExpectedSHA256 = item.expected
			runner, err := NewRunner(config)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := runner.RunStream(context.Background(), bytes.NewReader(item.content), nil); ErrorCode(err) != CodeChecksumMismatch {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestBlockDirectStreamRoundTrip(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source.raw")
	content := bytes.Repeat([]byte("raw-block-data"), 4096)
	if err := os.WriteFile(source, content, 0o600); err != nil {
		t.Fatal(err)
	}
	exportConfig := Config{TransferID: "vtx_block", Direction: DirectionExport, Format: FormatRawZST, VolumeMode: ModeBlock,
		ConsistencyMode: "unmounted", DataPath: source, CapacityBytes: int64(len(content)), MaxArchiveBytes: 1 << 20, ExportedAt: time.Unix(1, 0).UTC(), MaxFiles: 1}
	exporter, err := NewRunner(exportConfig)
	if err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	exported, err := exporter.RunStream(context.Background(), nil, &archive)
	if err != nil || exported.LogicalBytes != int64(len(content)) || exported.DataSHA256 == "" {
		t.Fatalf("export = %#v, %v", exported, err)
	}
	destination := filepath.Join(t.TempDir(), "destination.raw")
	if err := os.WriteFile(destination, make([]byte, len(content)), 0o600); err != nil {
		t.Fatal(err)
	}
	importConfig := exportConfig
	importConfig.Direction = DirectionImport
	importConfig.DataPath = destination
	importConfig.ExpectedBytes = int64(archive.Len())
	importConfig.ExpectedSHA256 = exported.SHA256
	importer, _ := NewRunner(importConfig)
	imported, err := importer.RunStream(context.Background(), bytes.NewReader(archive.Bytes()), nil)
	if err != nil || imported.DataSHA256 != exported.DataSHA256 {
		t.Fatalf("import = %#v, %v", imported, err)
	}
	got, err := os.ReadFile(destination)
	if err != nil || !bytes.Equal(got, content) {
		t.Fatal("raw direct stream round trip changed content")
	}
}

func TestExportStopsAtConfiguredArchiveLimit(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "large.bin"), bytes.Repeat([]byte("x"), 128*1024), 0o600); err != nil {
		t.Fatal(err)
	}
	config := testConfig(DirectionExport, source)
	config.MaxArchiveBytes = 32
	runner, err := NewRunner(config)
	if err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	if _, err := runner.RunStream(context.Background(), nil, &archive); ErrorCode(err) != CodeCapacityExceeded {
		t.Fatalf("error = %v", err)
	}
	if archive.Len() > int(config.MaxArchiveBytes) {
		t.Fatalf("archive bytes = %d, limit = %d", archive.Len(), config.MaxArchiveBytes)
	}
}

func TestConfigUsesOnlyDirectStreamSettings(t *testing.T) {
	values := map[string]string{
		"LUNA_VOLUME_TRANSFER_ID":               "vtx_test",
		"LUNA_VOLUME_TRANSFER_DIRECTION":        DirectionExport,
		"LUNA_VOLUME_TRANSFER_FORMAT":           FormatTarGZ,
		"LUNA_VOLUME_TRANSFER_VOLUME_MODE":      ModeFilesystem,
		"LUNA_VOLUME_TRANSFER_CONSISTENCY_MODE": "unmounted",
		"LUNA_VOLUME_TRANSFER_DATA_PATH":        "/volume",
		"LUNA_VOLUME_TRANSFER_CAPACITY_BYTES":   "1024",
		"LUNA_VOLUME_TRANSFER_MAX_BYTES":        "2048",
		"LUNA_VOLUME_TRANSFER_EXPECTED_BYTES":   "0",
		"LUNA_VOLUME_TRANSFER_EXPORTED_AT":      time.Now().UTC().Format(time.RFC3339Nano),
	}
	config, err := ConfigFromLookup(func(key string) string { return values[key] })
	if err != nil {
		t.Fatal(err)
	}
	if config.Direction != DirectionExport {
		t.Fatalf("config = %#v", config)
	}
}

func testConfig(direction, dataPath string) Config {
	return Config{TransferID: "vtx_test", Direction: direction, Format: FormatTarGZ, VolumeMode: ModeFilesystem,
		ConsistencyMode: "unmounted", DataPath: dataPath, CapacityBytes: 1 << 20, MaxArchiveBytes: 1 << 20, ExportedAt: time.Unix(1, 0).UTC(), MaxFiles: 100}
}
