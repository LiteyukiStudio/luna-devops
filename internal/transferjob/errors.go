package transferjob

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const (
	CodeArchiveUnsafe     = "volume_transfer.archive_unsafe"
	CodeCapacityExceeded  = "volume_transfer.capacity_exceeded"
	CodeChecksumMismatch  = "volume_transfer.checksum_mismatch"
	CodeStateConflict     = "volume_transfer.state_conflict"
	CodeFormatUnsupported = "volume_transfer.format_unsupported"
	CodeJobFailed         = "volume_transfer.job_failed"
)

var stableErrorCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(?:\.[a-z][a-z0-9_]*)+$`)

type Error struct {
	Code  string
	cause error
}

func (err *Error) Error() string {
	if err == nil || !stableErrorCodePattern.MatchString(err.Code) {
		return CodeJobFailed
	}
	return err.Code
}

func (err *Error) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.cause
}

func newError(code string, cause error) error {
	if !stableErrorCodePattern.MatchString(code) {
		code = CodeJobFailed
	}
	return &Error{Code: code, cause: cause}
}

func ErrorCode(err error) string {
	if err == nil {
		return ""
	}
	var coded *Error
	if errors.As(err, &coded) && stableErrorCodePattern.MatchString(coded.Code) {
		return coded.Code
	}
	return CodeJobFailed
}

func stableTransferErrorCode(code string) bool {
	return stableErrorCodePattern.MatchString(code) && (strings.HasPrefix(code, "volume_transfer.") || strings.HasPrefix(code, "volume."))
}

func invalidConfig(field string) error {
	return fmt.Errorf("invalid volume transfer job configuration: %s", field)
}
