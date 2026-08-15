package volumetransfer

import (
	"testing"

	"github.com/LiteyukiStudio/devops/internal/config"
)

func TestNewConfiguredStoreDisabled(t *testing.T) {
	store, err := NewConfiguredStore(config.Config{})
	if err != nil || store != nil {
		t.Fatalf("NewConfiguredStore() = (%v, %v), want (nil, nil)", store, err)
	}
}

func TestNewConfiguredStoreRejectsInvalidEndpoint(t *testing.T) {
	_, err := NewConfiguredStore(config.Config{
		VolumeTransferStore: "s3", VolumeTransferS3Endpoint: "://invalid",
		VolumeTransferS3Bucket: "transfers", VolumeTransferS3AccessKeyID: "access", VolumeTransferS3SecretKey: "secret",
	})
	if err == nil {
		t.Fatal("NewConfiguredStore() accepted an invalid endpoint")
	}
}
