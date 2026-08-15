package volumetransfer

import (
	"net/url"

	"github.com/LiteyukiStudio/devops/internal/config"
	"github.com/LiteyukiStudio/devops/internal/provider/volumestore"
)

// NewConfiguredStore builds the process-wide temporary archive store from
// administrator-owned configuration. A nil store means transfers are disabled;
// callers must return the stable store-unavailable response instead of falling
// back to request-local files or direct object-store access.
func NewConfiguredStore(cfg config.Config) (volumestore.Store, error) {
	if !cfg.VolumeTransferEnabled() {
		return nil, nil
	}
	endpoint, err := url.Parse(cfg.VolumeTransferS3Endpoint)
	if err != nil {
		return nil, err
	}
	return volumestore.NewS3Store(volumestore.S3Config{
		Endpoint:              cfg.VolumeTransferS3Endpoint,
		Region:                cfg.VolumeTransferS3Region,
		Bucket:                cfg.VolumeTransferS3Bucket,
		AccessKeyID:           cfg.VolumeTransferS3AccessKeyID,
		SecretAccessKey:       cfg.VolumeTransferS3SecretKey,
		AllowInsecureEndpoint: config.RuntimeMode() == "development" && endpoint.Scheme == "http",
		PathStyle:             cfg.VolumeTransferS3PathStyle,
	})
}
