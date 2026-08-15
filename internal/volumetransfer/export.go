package volumetransfer

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
	"time"
)

const (
	ArchiveManifestName = "luna-volume-manifest.json"
	ArchiveDataRoot     = "data"
)

type ArchiveManifest struct {
	SchemaVersion   int       `json:"schemaVersion"`
	VolumeMode      string    `json:"volumeMode"`
	Format          string    `json:"format"`
	ExportedAt      time.Time `json:"exportedAt"`
	LogicalBytes    int64     `json:"logicalBytes"`
	FileCount       int       `json:"fileCount"`
	RegularFiles    int       `json:"regularFiles"`
	DataSHA256      string    `json:"dataSha256"`
	ConsistencyMode string    `json:"consistencyMode"`
}

type ExportOptions struct {
	ConsistencyMode string
	ExportedAt      time.Time
	MaxLogicalBytes int64
	MaxFiles        int
}

// WriteTarGzip writes a deterministic, self-describing filesystem volume
// archive. It scans before writing and hashes again while writing so a source
// that changes during export fails instead of producing a misleading digest.
func WriteTarGzip(ctx context.Context, sourceDirectory string, destination io.Writer, options ExportOptions) (ArchiveManifest, error) {
	if ctx == nil {
		panic("volume archive export context is required")
	}
	if err := validateExportOptions(options); err != nil {
		return ArchiveManifest{}, err
	}
	if options.MaxFiles <= 0 {
		options.MaxFiles = DefaultMaxArchiveFiles
	}
	if options.ExportedAt.IsZero() {
		options.ExportedAt = time.Now().UTC()
	} else {
		options.ExportedAt = options.ExportedAt.UTC()
	}

	root, err := os.OpenRoot(sourceDirectory)
	if err != nil {
		return ArchiveManifest{}, err
	}
	defer root.Close()

	manifest, err := scanArchiveSource(ctx, root, options)
	if err != nil {
		return ArchiveManifest{}, err
	}
	manifest.ExportedAt = options.ExportedAt
	manifest.ConsistencyMode = options.ConsistencyMode

	gzipWriter := gzip.NewWriter(&contextWriter{ctx: ctx, writer: destination})
	tarWriter := tar.NewWriter(gzipWriter)
	closeWithError := func(current error) error {
		if err := tarWriter.Close(); current == nil {
			current = err
		}
		if err := gzipWriter.Close(); current == nil {
			current = err
		}
		return current
	}

	if err := writeArchiveManifest(tarWriter, manifest); err != nil {
		return ArchiveManifest{}, closeWithError(err)
	}
	if err := tarWriter.WriteHeader(&tar.Header{
		Name:     ArchiveDataRoot + "/",
		Typeflag: tar.TypeDir,
		Mode:     0o750,
		ModTime:  manifest.ExportedAt,
	}); err != nil {
		return ArchiveManifest{}, closeWithError(err)
	}

	writtenManifest, err := writeArchiveData(ctx, root, tarWriter, options)
	err = closeWithError(err)
	if err != nil {
		return ArchiveManifest{}, err
	}
	if !sameArchiveData(manifest, writtenManifest) {
		return ArchiveManifest{}, fmt.Errorf("%w: source changed during export", ErrArchiveUnsafe)
	}
	return manifest, nil
}

func validateExportOptions(options ExportOptions) error {
	if options.MaxLogicalBytes <= 0 {
		return fmt.Errorf("%w: invalid capacity limit", ErrArchiveCapacityExceeded)
	}
	switch options.ConsistencyMode {
	case "snapshot", "live", "unmounted":
		return nil
	default:
		return fmt.Errorf("%w: invalid consistency mode", ErrArchiveUnsafe)
	}
}

func scanArchiveSource(ctx context.Context, root *os.Root, options ExportOptions) (ArchiveManifest, error) {
	digest := sha256.New()
	manifest := ArchiveManifest{SchemaVersion: 1, VolumeMode: "Filesystem", Format: "tar_gz"}
	err := fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if name == "." {
			return nil
		}
		if _, err := cleanArchivePath(name); err != nil {
			return err
		}
		if manifest.FileCount >= options.MaxFiles {
			return ErrArchiveFileLimit
		}
		manifest.FileCount++

		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&(os.ModeSetuid|os.ModeSetgid) != 0 {
			return fmt.Errorf("%w: privileged mode", ErrArchiveUnsafe)
		}
		switch {
		case info.IsDir():
			writeDigestMetadata(digest, 'd', name, 0)
			return nil
		case info.Mode().IsRegular():
			if info.Size() > options.MaxLogicalBytes-manifest.LogicalBytes {
				return ErrArchiveCapacityExceeded
			}
			manifest.LogicalBytes += info.Size()
			manifest.RegularFiles++
			writeDigestMetadata(digest, 'f', name, info.Size())
			return hashArchiveFile(ctx, root, name, info.Size(), digest)
		case info.Mode()&os.ModeSymlink != 0:
			target, err := root.Readlink(name)
			if err != nil {
				return err
			}
			if _, err := cleanArchiveLinkTarget(name, target); err != nil {
				return err
			}
			writeDigestMetadata(digest, 'l', name, int64(len(target)))
			_, err = io.WriteString(digest, target)
			return err
		default:
			return fmt.Errorf("%w: unsupported source entry", ErrArchiveUnsafe)
		}
	})
	if err != nil {
		return ArchiveManifest{}, err
	}
	manifest.DataSHA256 = hex.EncodeToString(digest.Sum(nil))
	return manifest, nil
}

func writeArchiveData(ctx context.Context, root *os.Root, writer *tar.Writer, options ExportOptions) (ArchiveManifest, error) {
	digest := sha256.New()
	manifest := ArchiveManifest{SchemaVersion: 1, VolumeMode: "Filesystem", Format: "tar_gz", ConsistencyMode: options.ConsistencyMode}
	err := fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if name == "." {
			return nil
		}
		if manifest.FileCount >= options.MaxFiles {
			return ErrArchiveFileLimit
		}
		manifest.FileCount++
		info, err := entry.Info()
		if err != nil {
			return err
		}
		archiveName := path.Join(ArchiveDataRoot, name)
		header := &tar.Header{
			Name:    archiveName,
			Mode:    int64(info.Mode().Perm()),
			ModTime: info.ModTime(),
		}
		switch {
		case info.IsDir():
			header.Name += "/"
			header.Typeflag = tar.TypeDir
			writeDigestMetadata(digest, 'd', name, 0)
		case info.Mode().IsRegular():
			if info.Size() > options.MaxLogicalBytes-manifest.LogicalBytes {
				return ErrArchiveCapacityExceeded
			}
			header.Typeflag = tar.TypeReg
			header.Size = info.Size()
			manifest.LogicalBytes += info.Size()
			manifest.RegularFiles++
			writeDigestMetadata(digest, 'f', name, info.Size())
		case info.Mode()&os.ModeSymlink != 0:
			target, err := root.Readlink(name)
			if err != nil {
				return err
			}
			if _, err := cleanArchiveLinkTarget(name, target); err != nil {
				return err
			}
			header.Typeflag = tar.TypeSymlink
			header.Linkname = target
			writeDigestMetadata(digest, 'l', name, int64(len(target)))
			if _, err := io.WriteString(digest, target); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%w: unsupported source entry", ErrArchiveUnsafe)
		}
		if err := writer.WriteHeader(header); err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		return copyArchiveFile(ctx, root, name, info.Size(), writer, digest)
	})
	if err != nil {
		return ArchiveManifest{}, err
	}
	manifest.DataSHA256 = hex.EncodeToString(digest.Sum(nil))
	return manifest, nil
}

func hashArchiveFile(ctx context.Context, root *os.Root, name string, size int64, digest hash.Hash) error {
	file, err := root.Open(name)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != size {
		return fmt.Errorf("%w: source changed during scan", ErrArchiveUnsafe)
	}
	written, err := io.CopyN(digest, &contextReader{ctx: ctx, reader: file}, size)
	if err != nil || written != size {
		return fmt.Errorf("%w: source changed during scan", ErrArchiveUnsafe)
	}
	return nil
}

func copyArchiveFile(ctx context.Context, root *os.Root, name string, size int64, writer io.Writer, digest hash.Hash) error {
	file, err := root.Open(name)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != size {
		return fmt.Errorf("%w: source changed during export", ErrArchiveUnsafe)
	}
	written, err := io.CopyN(io.MultiWriter(writer, digest), &contextReader{ctx: ctx, reader: file}, size)
	if err != nil || written != size {
		return fmt.Errorf("%w: source changed during export", ErrArchiveUnsafe)
	}
	return nil
}

func writeArchiveManifest(writer *tar.Writer, manifest ArchiveManifest) error {
	content, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	if err := writer.WriteHeader(&tar.Header{
		Name:     ArchiveManifestName,
		Typeflag: tar.TypeReg,
		Mode:     0o640,
		Size:     int64(len(content)),
		ModTime:  manifest.ExportedAt,
	}); err != nil {
		return err
	}
	_, err = writer.Write(content)
	return err
}

func writeDigestMetadata(digest hash.Hash, kind byte, name string, size int64) {
	_, _ = digest.Write([]byte{kind})
	var field [8]byte
	binary.BigEndian.PutUint64(field[:], uint64(len(name)))
	_, _ = digest.Write(field[:])
	_, _ = io.WriteString(digest, name)
	binary.BigEndian.PutUint64(field[:], uint64(size))
	_, _ = digest.Write(field[:])
}

func sameArchiveData(left, right ArchiveManifest) bool {
	return left.SchemaVersion == right.SchemaVersion &&
		left.VolumeMode == right.VolumeMode &&
		left.Format == right.Format &&
		left.ConsistencyMode == right.ConsistencyMode &&
		left.LogicalBytes == right.LogicalBytes &&
		left.FileCount == right.FileCount &&
		left.RegularFiles == right.RegularFiles &&
		strings.EqualFold(left.DataSHA256, right.DataSHA256)
}

type contextWriter struct {
	ctx    context.Context
	writer io.Writer
}

func (w *contextWriter) Write(buffer []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	return w.writer.Write(buffer)
}
