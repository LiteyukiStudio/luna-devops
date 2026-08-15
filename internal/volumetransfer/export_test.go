package volumetransfer

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteTarGzipCreatesManifestAndRoundTripsData(t *testing.T) {
	source := t.TempDir()
	if err := os.MkdirAll(filepath.Join(source, "nested"), 0o750); err != nil {
		t.Fatalf("create source directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(source, "nested", "data.txt"), []byte("payload"), 0o640); err != nil {
		t.Fatalf("write source file: %v", err)
	}
	if err := os.Symlink("nested/data.txt", filepath.Join(source, "current")); err != nil {
		t.Fatalf("create source symlink: %v", err)
	}

	var archive bytes.Buffer
	exportedAt := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	manifest, err := WriteTarGzip(context.Background(), source, &archive, ExportOptions{
		ConsistencyMode: "snapshot",
		ExportedAt:      exportedAt,
		MaxLogicalBytes: 1024,
		MaxFiles:        10,
	})
	if err != nil {
		t.Fatalf("WriteTarGzip returned error: %v", err)
	}
	if manifest.SchemaVersion != 1 || manifest.LogicalBytes != 7 || manifest.FileCount != 3 || manifest.RegularFiles != 1 || len(manifest.DataSHA256) != 64 {
		t.Fatalf("manifest = %#v", manifest)
	}

	storedManifest := readStoredArchiveManifest(t, archive.Bytes())
	if storedManifest != manifest {
		t.Fatalf("stored manifest = %#v, want %#v", storedManifest, manifest)
	}

	destination := t.TempDir()
	result, err := ExtractTarGzip(context.Background(), bytes.NewReader(archive.Bytes()), destination, ExtractLimits{MaxLogicalBytes: 1024, MaxFiles: 10})
	if err != nil {
		t.Fatalf("ExtractTarGzip returned error: %v", err)
	}
	if result.LogicalBytes != 7 {
		t.Fatalf("extract result = %#v", result)
	}
	content, err := os.ReadFile(filepath.Join(destination, "current"))
	if err != nil || string(content) != "payload" {
		t.Fatalf("round-trip content = %q, %v", content, err)
	}
}

func TestWriteTarGzipRejectsEscapingSymlink(t *testing.T) {
	source := t.TempDir()
	if err := os.Symlink("../outside", filepath.Join(source, "outside")); err != nil {
		t.Fatalf("create source symlink: %v", err)
	}
	_, err := WriteTarGzip(context.Background(), source, io.Discard, ExportOptions{ConsistencyMode: "unmounted", MaxLogicalBytes: 1024})
	if !errors.Is(err, ErrArchiveUnsafe) {
		t.Fatalf("error = %v, want ErrArchiveUnsafe", err)
	}
}

func TestWriteTarGzipEnforcesLimitsAndCancellation(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "large"), []byte("payload"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	_, err := WriteTarGzip(context.Background(), source, io.Discard, ExportOptions{ConsistencyMode: "live", MaxLogicalBytes: 3})
	if !errors.Is(err, ErrArchiveCapacityExceeded) {
		t.Fatalf("capacity error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = WriteTarGzip(ctx, source, io.Discard, ExportOptions{ConsistencyMode: "live", MaxLogicalBytes: 1024})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v", err)
	}
}

func TestExtractTarGzipRejectsManifestChecksumMismatch(t *testing.T) {
	manifest := ArchiveManifest{
		SchemaVersion:   1,
		VolumeMode:      "Filesystem",
		Format:          "tar_gz",
		ExportedAt:      time.Now().UTC(),
		LogicalBytes:    7,
		FileCount:       1,
		RegularFiles:    1,
		DataSHA256:      "0000000000000000000000000000000000000000000000000000000000000000",
		ConsistencyMode: "snapshot",
	}
	manifestContent, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	archive := makeTarGzip(t,
		archiveEntry{name: ArchiveManifestName, typeflag: tar.TypeReg, content: manifestContent},
		archiveEntry{name: ArchiveDataRoot, typeflag: tar.TypeDir},
		archiveEntry{name: ArchiveDataRoot + "/payload", typeflag: tar.TypeReg, content: []byte("payload")},
	)

	_, err = ExtractTarGzip(context.Background(), bytes.NewReader(archive), t.TempDir(), ExtractLimits{MaxLogicalBytes: 1024, MaxFiles: 10})
	if !errors.Is(err, ErrArchiveChecksumMismatch) {
		t.Fatalf("error = %v, want ErrArchiveChecksumMismatch", err)
	}
}

func readStoredArchiveManifest(t *testing.T, content []byte) ArchiveManifest {
	t.Helper()
	gzipReader, err := gzip.NewReader(bytes.NewReader(content))
	if err != nil {
		t.Fatalf("open gzip: %v", err)
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	header, err := tarReader.Next()
	if err != nil {
		t.Fatalf("read manifest header: %v", err)
	}
	if header.Name != ArchiveManifestName {
		t.Fatalf("first entry = %q", header.Name)
	}
	var manifest ArchiveManifest
	if err := json.NewDecoder(tarReader).Decode(&manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	return manifest
}
