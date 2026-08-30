package api

import (
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
)

func TestVolumeTransferNativeDownloadFilenamesAreStable(t *testing.T) {
	for format, want := range map[string]string{
		model.VolumeTransferFormatTarGZ:  "vtx_demo.tar.gz",
		model.VolumeTransferFormatRawZST: "vtx_demo.raw.zst",
	} {
		if got := volumeTransferArchiveFilename(model.VolumeTransfer{ID: "vtx_demo", Format: format}); got != want {
			t.Fatalf("archive filename for %q = %q, want %q", format, got, want)
		}
	}
}

func TestVolumeTransferManifestRoutesAreRegistered(t *testing.T) {
	db := authIntegrationDB(t)
	if err := db.AutoMigrate(&model.AppConfig{}); err != nil {
		t.Fatalf("migrate route config dependency: %v", err)
	}
	router := NewRouter(db, mustTestConfig(t))
	routes := make(map[string]bool)
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	for _, expected := range []string{
		"PUT /api/v1/projects/:projectId/volume-imports/:transferId/content",
		"GET /api/v1/projects/:projectId/volume-transfers/:transferId/content",
		"GET /api/v1/projects/:projectId/volume-transfers/:transferId/manifest",
	} {
		if !routes[expected] {
			t.Fatalf("route %q is not registered", expected)
		}
	}
	for _, removed := range []string{
		"HEAD /api/v1/projects/:projectId/volume-imports/:transferId/content",
		"PATCH /api/v1/projects/:projectId/volume-imports/:transferId/content",
		"HEAD /api/v1/projects/:projectId/volume-transfers/:transferId/content",
		"HEAD /api/v1/projects/:projectId/volume-transfers/:transferId/manifest",
	} {
		if routes[removed] {
			t.Fatalf("legacy transfer route %q remains registered", removed)
		}
	}
}
