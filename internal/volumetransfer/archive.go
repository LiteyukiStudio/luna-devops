package volumetransfer

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
)

const (
	DefaultMaxArchiveFiles = 1_000_000
	defaultMaxPathBytes    = 4_096
)

var (
	ErrArchiveUnsafe           = errors.New("volume archive is unsafe")
	ErrArchiveCapacityExceeded = errors.New("volume archive exceeds target capacity")
	ErrArchiveFileLimit        = errors.New("volume archive exceeds file count limit")
	ErrArchiveChecksumMismatch = errors.New("volume archive checksum does not match manifest")
)

type ExtractLimits struct {
	MaxLogicalBytes int64
	MaxFiles        int
}

type ExtractResult struct {
	LogicalBytes int64
	Entries      int
	Files        int
	Directories  int
	Links        int
	DataSHA256   string
	Manifest     *ArchiveManifest
}

// ExtractTarGzip extracts a filesystem volume archive beneath destination.
// os.Root provides the final containment boundary even when the target volume
// already contains symbolic links. Archive entry names are intentionally not
// included in returned errors because they may contain sensitive user data.
func ExtractTarGzip(ctx context.Context, source io.Reader, destination string, limits ExtractLimits) (ExtractResult, error) {
	if ctx == nil {
		panic("volume archive extraction context is required")
	}
	if err := ctx.Err(); err != nil {
		return ExtractResult{}, err
	}
	if limits.MaxLogicalBytes <= 0 {
		return ExtractResult{}, fmt.Errorf("%w: invalid capacity limit", ErrArchiveCapacityExceeded)
	}
	if limits.MaxFiles <= 0 {
		limits.MaxFiles = DefaultMaxArchiveFiles
	}

	gzipReader, err := gzip.NewReader(&contextReader{ctx: ctx, reader: source})
	if err != nil {
		return ExtractResult{}, fmt.Errorf("%w: invalid gzip stream", ErrArchiveUnsafe)
	}
	defer gzipReader.Close()

	root, err := os.OpenRoot(destination)
	if err != nil {
		return ExtractResult{}, err
	}
	defer root.Close()

	return extractTar(ctx, tar.NewReader(gzipReader), root, limits)
}

func extractTar(ctx context.Context, reader *tar.Reader, root *os.Root, limits ExtractLimits) (ExtractResult, error) {
	result := ExtractResult{}
	seen := make(map[string]struct{})
	digest := sha256.New()
	firstEntry := true
	for {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			result.DataSHA256 = hex.EncodeToString(digest.Sum(nil))
			if result.Manifest != nil && (result.Manifest.FileCount != result.Entries || result.Manifest.RegularFiles != result.Files || result.Manifest.LogicalBytes != result.LogicalBytes || !strings.EqualFold(result.Manifest.DataSHA256, result.DataSHA256)) {
				return result, ErrArchiveChecksumMismatch
			}
			return result, nil
		}
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return result, ctxErr
			}
			return result, fmt.Errorf("%w: invalid tar stream", ErrArchiveUnsafe)
		}

		rawName, err := cleanArchivePath(header.Name)
		if err != nil {
			return result, err
		}
		if firstEntry && rawName == ArchiveManifestName {
			manifest, err := parseArchiveManifestEntry(reader, header)
			if err != nil {
				return result, err
			}
			result.Manifest = &manifest
			firstEntry = false
			continue
		}
		firstEntry = false

		name := rawName
		linkName := header.Linkname
		if result.Manifest != nil {
			name, err = stripPlatformArchiveDataRoot(rawName, header.Typeflag)
			if err != nil {
				return result, err
			}
			if name == "." {
				continue
			}
			if header.Typeflag == tar.TypeLink {
				linkName, err = stripPlatformArchiveHardlinkTarget(header.Linkname)
				if err != nil {
					return result, err
				}
			}
		}
		if name == "." {
			if header.Typeflag == tar.TypeDir {
				continue
			}
			return result, fmt.Errorf("%w: invalid root entry", ErrArchiveUnsafe)
		}
		if _, exists := seen[name]; exists {
			return result, fmt.Errorf("%w: duplicate entry", ErrArchiveUnsafe)
		}
		seen[name] = struct{}{}
		if result.Entries >= limits.MaxFiles {
			return result, ErrArchiveFileLimit
		}
		result.Entries++

		if header.Mode&(0o4000|0o2000) != 0 {
			return result, fmt.Errorf("%w: privileged mode", ErrArchiveUnsafe)
		}
		if header.Size < 0 {
			return result, fmt.Errorf("%w: negative entry size", ErrArchiveUnsafe)
		}

		switch header.Typeflag {
		case tar.TypeReg, tar.TypeRegA:
			if err := reserveArchiveFile(&result, header.Size, limits); err != nil {
				return result, err
			}
			writeDigestMetadata(digest, 'f', name, header.Size)
			if err := writeRegularFile(ctx, reader, root, name, header.Size, os.FileMode(header.Mode)&0o777, digest); err != nil {
				return result, err
			}
		case tar.TypeDir:
			if err := ensureArchiveDirectory(root, name, directoryMode(header.Mode)); err != nil {
				return result, err
			}
			writeDigestMetadata(digest, 'd', name, 0)
			result.Directories++
		case tar.TypeSymlink:
			if err := createArchiveSymlink(root, name, linkName); err != nil {
				return result, err
			}
			writeDigestMetadata(digest, 'l', name, int64(len(linkName)))
			_, _ = io.WriteString(digest, linkName)
			result.Links++
		case tar.TypeLink:
			if result.Manifest != nil {
				return result, fmt.Errorf("%w: hard links are not supported in platform archives", ErrArchiveUnsafe)
			}
			if err := createArchiveHardlink(root, name, linkName); err != nil {
				return result, err
			}
			result.Links++
		case tar.TypeXHeader, tar.TypeXGlobalHeader:
			return result, fmt.Errorf("%w: standalone extended header", ErrArchiveUnsafe)
		default:
			return result, fmt.Errorf("%w: unsupported entry type", ErrArchiveUnsafe)
		}
	}
}

func reserveArchiveFile(result *ExtractResult, size int64, limits ExtractLimits) error {
	if size > limits.MaxLogicalBytes-result.LogicalBytes {
		return ErrArchiveCapacityExceeded
	}
	result.Files++
	result.LogicalBytes += size
	return nil
}

func writeRegularFile(ctx context.Context, reader io.Reader, root *os.Root, name string, size int64, mode os.FileMode, digest io.Writer) (err error) {
	if err := ensureArchiveParent(root, name); err != nil {
		return err
	}
	if err := removeArchiveLeaf(root, name); err != nil {
		return err
	}
	if mode == 0 {
		mode = 0o600
	}
	file, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
	}()

	destination := io.Writer(file)
	if digest != nil {
		destination = io.MultiWriter(file, digest)
	}
	written, err := io.CopyN(destination, &contextReader{ctx: ctx, reader: reader}, size)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("%w: truncated regular file", ErrArchiveUnsafe)
	}
	if written != size {
		return fmt.Errorf("%w: regular file size mismatch", ErrArchiveUnsafe)
	}
	return nil
}

func parseArchiveManifestEntry(reader io.Reader, header *tar.Header) (ArchiveManifest, error) {
	const maxManifestBytes = 64 * 1024
	if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA || header.Size <= 0 || header.Size > maxManifestBytes {
		return ArchiveManifest{}, fmt.Errorf("%w: invalid platform manifest", ErrArchiveUnsafe)
	}
	content, err := io.ReadAll(io.LimitReader(reader, header.Size))
	if err != nil || int64(len(content)) != header.Size {
		return ArchiveManifest{}, fmt.Errorf("%w: truncated platform manifest", ErrArchiveUnsafe)
	}
	var manifest ArchiveManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		return ArchiveManifest{}, fmt.Errorf("%w: invalid platform manifest", ErrArchiveUnsafe)
	}
	checksum, err := hex.DecodeString(manifest.DataSHA256)
	if err != nil || len(checksum) != sha256.Size || manifest.SchemaVersion != 1 || manifest.VolumeMode != "Filesystem" || manifest.Format != "tar_gz" || manifest.FileCount < 0 || manifest.RegularFiles < 0 || manifest.RegularFiles > manifest.FileCount || manifest.LogicalBytes < 0 {
		return ArchiveManifest{}, fmt.Errorf("%w: unsupported platform manifest", ErrArchiveUnsafe)
	}
	switch manifest.ConsistencyMode {
	case "snapshot", "live", "unmounted":
		return manifest, nil
	default:
		return ArchiveManifest{}, fmt.Errorf("%w: invalid platform manifest consistency", ErrArchiveUnsafe)
	}
}

func stripPlatformArchiveDataRoot(name string, typeFlag byte) (string, error) {
	if name == ArchiveDataRoot && typeFlag == tar.TypeDir {
		return ".", nil
	}
	prefix := ArchiveDataRoot + "/"
	if !strings.HasPrefix(name, prefix) {
		return "", fmt.Errorf("%w: platform archive entry is outside data root", ErrArchiveUnsafe)
	}
	stripped := strings.TrimPrefix(name, prefix)
	if stripped == "" {
		return ".", nil
	}
	return cleanArchivePath(stripped)
}

func stripPlatformArchiveHardlinkTarget(linkName string) (string, error) {
	target, err := cleanArchivePath(linkName)
	if err != nil {
		return "", err
	}
	prefix := ArchiveDataRoot + "/"
	if !strings.HasPrefix(target, prefix) {
		return "", fmt.Errorf("%w: platform hard link is outside data root", ErrArchiveUnsafe)
	}
	return cleanArchivePath(strings.TrimPrefix(target, prefix))
}

func createArchiveSymlink(root *os.Root, name, linkName string) error {
	if err := ensureArchiveParent(root, name); err != nil {
		return err
	}
	target, err := cleanArchiveLinkTarget(name, linkName)
	if err != nil {
		return err
	}
	if target == "." {
		return fmt.Errorf("%w: invalid symbolic link target", ErrArchiveUnsafe)
	}
	if err := removeArchiveLeaf(root, name); err != nil {
		return err
	}
	if err := root.Symlink(linkName, name); err != nil {
		return err
	}
	return nil
}

func createArchiveHardlink(root *os.Root, name, linkName string) error {
	if err := ensureArchiveParent(root, name); err != nil {
		return err
	}
	target, err := cleanArchivePath(linkName)
	if err != nil || target == "." {
		return fmt.Errorf("%w: invalid hard link target", ErrArchiveUnsafe)
	}
	info, err := root.Lstat(target)
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("%w: hard link target is unavailable", ErrArchiveUnsafe)
	}
	if err := removeArchiveLeaf(root, name); err != nil {
		return err
	}
	if err := root.Link(target, name); err != nil {
		return err
	}
	return nil
}

func ensureArchiveParent(root *os.Root, name string) error {
	parent := path.Dir(name)
	if parent == "." {
		return nil
	}
	return root.MkdirAll(parent, 0o750)
}

func ensureArchiveDirectory(root *os.Root, name string, mode os.FileMode) error {
	info, err := root.Lstat(name)
	if err == nil && info.IsDir() {
		return nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err == nil {
		if err := root.RemoveAll(name); err != nil {
			return err
		}
	}
	return root.MkdirAll(name, mode)
}

func removeArchiveLeaf(root *os.Root, name string) error {
	_, err := root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return root.RemoveAll(name)
}

func cleanArchivePath(value string) (string, error) {
	if value == "" || len(value) > defaultMaxPathBytes || strings.ContainsRune(value, '\x00') || strings.Contains(value, `\`) {
		return "", fmt.Errorf("%w: invalid entry path", ErrArchiveUnsafe)
	}
	if path.IsAbs(value) {
		return "", fmt.Errorf("%w: absolute entry path", ErrArchiveUnsafe)
	}
	cleaned := path.Clean(value)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("%w: entry escapes destination", ErrArchiveUnsafe)
	}
	return cleaned, nil
}

func cleanArchiveLinkTarget(entryName, linkName string) (string, error) {
	if linkName == "" || len(linkName) > defaultMaxPathBytes || strings.ContainsRune(linkName, '\x00') || strings.Contains(linkName, `\`) || path.IsAbs(linkName) {
		return "", fmt.Errorf("%w: invalid symbolic link target", ErrArchiveUnsafe)
	}
	target := path.Clean(path.Join(path.Dir(entryName), linkName))
	if target == ".." || strings.HasPrefix(target, "../") {
		return "", fmt.Errorf("%w: symbolic link escapes destination", ErrArchiveUnsafe)
	}
	return target, nil
}

func directoryMode(mode int64) os.FileMode {
	permissions := os.FileMode(mode) & 0o777
	if permissions == 0 {
		return 0o750
	}
	return permissions
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(buffer)
}
