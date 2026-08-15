package volumetransfer

const (
	// MinimumChunkSize keeps ordinary transfers efficient while dynamic sizing
	// prevents large transfers from exceeding the S3 multipart part limit.
	MinimumChunkSize    int64 = 64 * 1024 * 1024
	MaximumChunkSize    int64 = 5 * 1024 * 1024 * 1024
	MaximumTransferSize int64 = 5 * 1024 * 1024 * 1024 * 1024
	MaxMultipartParts   int64 = 10_000
	chunkSizeAlignment  int64 = 1024 * 1024
)

// RequiredChunkSize returns a MiB-aligned multipart size that keeps the
// declared transfer within the S3 10,000-part limit.
func RequiredChunkSize(expectedBytes int64) int64 {
	if expectedBytes <= 0 {
		return MinimumChunkSize
	}
	required := (expectedBytes + MaxMultipartParts - 1) / MaxMultipartParts
	required = ((required + chunkSizeAlignment - 1) / chunkSizeAlignment) * chunkSizeAlignment
	if required < MinimumChunkSize {
		return MinimumChunkSize
	}
	if required > MaximumChunkSize {
		return MaximumChunkSize
	}
	return required
}
