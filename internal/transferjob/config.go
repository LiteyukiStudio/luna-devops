package transferjob

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	DirectionImport           = "import"
	DirectionExport           = "export"
	FormatTarGZ               = "tar_gz"
	FormatRawZST              = "raw_zst"
	ModeFilesystem            = "Filesystem"
	ModeBlock                 = "Block"
	defaultMaxFiles           = 1_000_000
	maximumTransferSize int64 = 5 * 1024 * 1024 * 1024 * 1024
)

var transferIDPattern = regexp.MustCompile(`^vtx_[A-Za-z0-9_-]{1,120}$`)

// Config contains only immutable, non-secret execution metadata. Archive bytes
// arrive through Kubernetes exec and are never fetched from a remote endpoint.
type Config struct {
	TransferID      string
	Direction       string
	Format          string
	VolumeMode      string
	ConsistencyMode string
	DataPath        string
	CapacityBytes   int64
	MaxArchiveBytes int64
	ExpectedBytes   int64
	ExpectedSHA256  string
	ExportedAt      time.Time
	MaxFiles        int
	Traceparent     string
	Tracestate      string
}

func ConfigFromEnv() (Config, error) { return ConfigFromLookup(os.Getenv) }

func ConfigFromLookup(lookup func(string) string) (Config, error) {
	if lookup == nil {
		return Config{}, invalidConfig("environment lookup")
	}
	capacity, err := parsePositiveInt64(lookup("LUNA_VOLUME_TRANSFER_CAPACITY_BYTES"))
	if err != nil {
		return Config{}, invalidConfig("capacity bytes")
	}
	expected, err := parseNonNegativeInt64(lookup("LUNA_VOLUME_TRANSFER_EXPECTED_BYTES"))
	if err != nil {
		return Config{}, invalidConfig("expected bytes")
	}
	maxArchiveBytes, err := parsePositiveInt64(lookup("LUNA_VOLUME_TRANSFER_MAX_BYTES"))
	if err != nil {
		return Config{}, invalidConfig("maximum archive bytes")
	}
	maxFiles := defaultMaxFiles
	if value := strings.TrimSpace(lookup("LUNA_VOLUME_TRANSFER_MAX_FILES")); value != "" {
		parsed, parseErr := strconv.Atoi(value)
		if parseErr != nil || parsed < 1 || parsed > defaultMaxFiles {
			return Config{}, invalidConfig("maximum file count")
		}
		maxFiles = parsed
	}
	config := Config{
		TransferID:      strings.TrimSpace(lookup("LUNA_VOLUME_TRANSFER_ID")),
		Direction:       strings.ToLower(strings.TrimSpace(lookup("LUNA_VOLUME_TRANSFER_DIRECTION"))),
		Format:          strings.ToLower(strings.TrimSpace(lookup("LUNA_VOLUME_TRANSFER_FORMAT"))),
		VolumeMode:      strings.TrimSpace(lookup("LUNA_VOLUME_TRANSFER_VOLUME_MODE")),
		ConsistencyMode: strings.ToLower(strings.TrimSpace(lookup("LUNA_VOLUME_TRANSFER_CONSISTENCY_MODE"))),
		DataPath:        strings.TrimSpace(lookup("LUNA_VOLUME_TRANSFER_DATA_PATH")),
		CapacityBytes:   capacity,
		MaxArchiveBytes: maxArchiveBytes,
		ExpectedBytes:   expected,
		ExpectedSHA256:  strings.ToLower(strings.TrimSpace(lookup("LUNA_VOLUME_TRANSFER_EXPECTED_SHA256"))),
		MaxFiles:        maxFiles,
		Traceparent:     strings.TrimSpace(lookup("LUNA_VOLUME_TRANSFER_TRACEPARENT")),
		Tracestate:      strings.TrimSpace(lookup("LUNA_VOLUME_TRANSFER_TRACESTATE")),
	}
	if value := strings.TrimSpace(lookup("LUNA_VOLUME_TRANSFER_EXPORTED_AT")); value != "" {
		config.ExportedAt, err = time.Parse(time.RFC3339Nano, value)
		if err != nil {
			return Config{}, invalidConfig("export timestamp")
		}
		config.ExportedAt = config.ExportedAt.UTC()
	}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

func (config Config) Validate() error {
	if !transferIDPattern.MatchString(config.TransferID) {
		return invalidConfig("transfer id")
	}
	if config.Direction != DirectionImport && config.Direction != DirectionExport {
		return invalidConfig("direction")
	}
	if config.Direction == DirectionImport && (config.ExpectedBytes < 1 || config.ExpectedSHA256 == "") {
		return invalidConfig("import length and checksum")
	}
	switch config.VolumeMode {
	case ModeFilesystem:
		if config.Format != FormatTarGZ {
			return invalidConfig("filesystem format")
		}
	case ModeBlock:
		if config.Format != FormatRawZST {
			return invalidConfig("block format")
		}
	default:
		return invalidConfig("volume mode")
	}
	if config.Direction == DirectionExport {
		switch config.ConsistencyMode {
		case "snapshot", "live", "unmounted":
		default:
			return invalidConfig("consistency mode")
		}
		if config.ExportedAt.IsZero() {
			return invalidConfig("export timestamp")
		}
	}
	if config.CapacityBytes < 1 || config.CapacityBytes > maximumTransferSize ||
		config.MaxArchiveBytes < 1 || config.MaxArchiveBytes > maximumTransferSize ||
		config.ExpectedBytes < 0 || config.ExpectedBytes > maximumTransferSize {
		return invalidConfig("byte limits")
	}
	if config.MaxFiles < 1 || config.MaxFiles > defaultMaxFiles {
		return invalidConfig("maximum file count")
	}
	if config.ExpectedSHA256 != "" && !isSHA256(config.ExpectedSHA256) {
		return invalidConfig("expected checksum")
	}
	if !safeAbsolutePath(config.DataPath) {
		return invalidConfig("data path")
	}
	if len(config.Traceparent) > 128 || len(config.Tracestate) > 512 || strings.ContainsAny(config.Traceparent+config.Tracestate, "\r\n") {
		return invalidConfig("trace context")
	}
	return nil
}

func safeAbsolutePath(value string) bool {
	return filepath.IsAbs(value) && value != string(filepath.Separator) && filepath.Clean(value) == value && !strings.ContainsRune(value, 0)
}

func parsePositiveInt64(value string) (int64, error) {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || parsed < 1 {
		return 0, invalidConfig("positive integer")
	}
	return parsed, nil
}

func parseNonNegativeInt64(value string) (int64, error) {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || parsed < 0 {
		return 0, invalidConfig("non-negative integer")
	}
	return parsed, nil
}

func isSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
