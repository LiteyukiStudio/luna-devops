package volumetransfer

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type archiveEntry struct {
	name     string
	typeflag byte
	linkName string
	mode     int64
	content  []byte
}

func TestExtractTarGzipExtractsContainedEntries(t *testing.T) {
	archive := makeTarGzip(t,
		archiveEntry{name: "data", typeflag: tar.TypeDir, mode: 0o750},
		archiveEntry{name: "data/config.json", typeflag: tar.TypeReg, mode: 0o640, content: []byte("content")},
		archiveEntry{name: "data/config-copy.json", typeflag: tar.TypeLink, linkName: "data/config.json", mode: 0o640},
		archiveEntry{name: "data/current", typeflag: tar.TypeSymlink, linkName: "config.json", mode: 0o777},
	)
	destination := t.TempDir()

	result, err := ExtractTarGzip(context.Background(), bytes.NewReader(archive), destination, ExtractLimits{MaxLogicalBytes: 1 << 20, MaxFiles: 10})
	if err != nil {
		t.Fatalf("ExtractTarGzip returned error: %v", err)
	}
	if result.Entries != 4 || result.Files != 1 || result.Directories != 1 || result.Links != 2 || result.LogicalBytes != 7 {
		t.Fatalf("result = %#v", result)
	}
	content, err := os.ReadFile(filepath.Join(destination, "data", "current"))
	if err != nil {
		t.Fatalf("read extracted symlink: %v", err)
	}
	if string(content) != "content" {
		t.Fatalf("content = %q", content)
	}
}

func TestExtractTarGzipRejectsUnsafeEntries(t *testing.T) {
	tests := []struct {
		name  string
		entry archiveEntry
	}{
		{name: "parent traversal", entry: archiveEntry{name: "../outside", typeflag: tar.TypeReg, content: []byte("x")}},
		{name: "absolute path", entry: archiveEntry{name: "/outside", typeflag: tar.TypeReg, content: []byte("x")}},
		{name: "windows separator", entry: archiveEntry{name: `..\outside`, typeflag: tar.TypeReg, content: []byte("x")}},
		{name: "escaping symlink", entry: archiveEntry{name: "data/link", typeflag: tar.TypeSymlink, linkName: "../../outside"}},
		{name: "device node", entry: archiveEntry{name: "data/device", typeflag: tar.TypeChar}},
		{name: "setuid", entry: archiveEntry{name: "data/tool", typeflag: tar.TypeReg, mode: 0o4755, content: []byte("x")}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			archive := makeTarGzip(t, tt.entry)
			_, err := ExtractTarGzip(context.Background(), bytes.NewReader(archive), t.TempDir(), ExtractLimits{MaxLogicalBytes: 1024, MaxFiles: 10})
			if !errors.Is(err, ErrArchiveUnsafe) {
				t.Fatalf("error = %v, want ErrArchiveUnsafe", err)
			}
		})
	}
}

func TestExtractTarGzipEnforcesCapacityBeforeWritingOversizedEntry(t *testing.T) {
	destination := t.TempDir()
	archive := makeTarGzip(t, archiveEntry{name: "data/large", typeflag: tar.TypeReg, content: []byte("too-large")})

	_, err := ExtractTarGzip(context.Background(), bytes.NewReader(archive), destination, ExtractLimits{MaxLogicalBytes: 3, MaxFiles: 10})
	if !errors.Is(err, ErrArchiveCapacityExceeded) {
		t.Fatalf("error = %v, want ErrArchiveCapacityExceeded", err)
	}
	if _, statErr := os.Stat(filepath.Join(destination, "data", "large")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("oversized entry was created: %v", statErr)
	}
}

func TestExtractTarGzipEnforcesFileLimit(t *testing.T) {
	archive := makeTarGzip(t,
		archiveEntry{name: "one", typeflag: tar.TypeReg, content: []byte("1")},
		archiveEntry{name: "two", typeflag: tar.TypeReg, content: []byte("2")},
	)

	_, err := ExtractTarGzip(context.Background(), bytes.NewReader(archive), t.TempDir(), ExtractLimits{MaxLogicalBytes: 1024, MaxFiles: 1})
	if !errors.Is(err, ErrArchiveFileLimit) {
		t.Fatalf("error = %v, want ErrArchiveFileLimit", err)
	}
}

func TestExtractTarGzipCountsDirectoriesAndLinksTowardEntryLimit(t *testing.T) {
	archive := makeTarGzip(t,
		archiveEntry{name: "data", typeflag: tar.TypeDir},
		archiveEntry{name: "data/link", typeflag: tar.TypeSymlink, linkName: "."},
	)

	_, err := ExtractTarGzip(context.Background(), bytes.NewReader(archive), t.TempDir(), ExtractLimits{MaxLogicalBytes: 1024, MaxFiles: 1})
	if !errors.Is(err, ErrArchiveFileLimit) {
		t.Fatalf("error = %v, want ErrArchiveFileLimit", err)
	}
}

func TestExtractTarGzipHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ExtractTarGzip(ctx, bytes.NewReader(nil), t.TempDir(), ExtractLimits{MaxLogicalBytes: 1024})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestExtractTarGzipReplacesExistingSymlinkInsteadOfWritingThroughIt(t *testing.T) {
	destination := t.TempDir()
	if err := os.WriteFile(filepath.Join(destination, "existing"), []byte("keep"), 0o600); err != nil {
		t.Fatalf("write existing target: %v", err)
	}
	if err := os.Symlink("existing", filepath.Join(destination, "payload")); err != nil {
		t.Fatalf("create existing symlink: %v", err)
	}
	archive := makeTarGzip(t, archiveEntry{name: "payload", typeflag: tar.TypeReg, content: []byte("new")})

	if _, err := ExtractTarGzip(context.Background(), bytes.NewReader(archive), destination, ExtractLimits{MaxLogicalBytes: 1024}); err != nil {
		t.Fatalf("ExtractTarGzip returned error: %v", err)
	}
	existing, err := os.ReadFile(filepath.Join(destination, "existing"))
	if err != nil || string(existing) != "keep" {
		t.Fatalf("existing target = %q, %v", existing, err)
	}
	payload, err := os.ReadFile(filepath.Join(destination, "payload"))
	if err != nil || string(payload) != "new" {
		t.Fatalf("payload = %q, %v", payload, err)
	}
	info, err := os.Lstat(filepath.Join(destination, "payload"))
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("payload mode = %v, %v", info, err)
	}
}

func makeTarGzip(t *testing.T, entries ...archiveEntry) []byte {
	t.Helper()
	var output bytes.Buffer
	gzipWriter := gzip.NewWriter(&output)
	tarWriter := tar.NewWriter(gzipWriter)
	for _, entry := range entries {
		mode := entry.mode
		if mode == 0 {
			mode = 0o640
		}
		header := &tar.Header{
			Name:     entry.name,
			Typeflag: entry.typeflag,
			Linkname: entry.linkName,
			Mode:     mode,
			Size:     int64(len(entry.content)),
		}
		if entry.typeflag == tar.TypeDir || entry.typeflag == tar.TypeSymlink || entry.typeflag == tar.TypeLink || entry.typeflag == tar.TypeChar {
			header.Size = 0
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatalf("write tar header: %v", err)
		}
		if len(entry.content) > 0 {
			if _, err := tarWriter.Write(entry.content); err != nil {
				t.Fatalf("write tar content: %v", err)
			}
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}
	return output.Bytes()
}
