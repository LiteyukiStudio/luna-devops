package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
)

func TestVolumeDownloadSessionCookieIsBoundToTransferPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	path := "/api/v1/projects/prj_demo/volume-transfers/vtx_demo/content"
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodHead, path+"?ticket=secret", nil)
	expiresAt := time.Now().Add(20 * time.Minute).UTC()

	(&Handlers{mode: "production"}).setVolumeDownloadSessionCookie(ctx, volumeDownloadSession{
		Token: "vds_test_session", ExpiresAt: expiresAt,
	})

	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies=%d", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != volumeDownloadSessionCookieName || cookie.Value != "vds_test_session" || cookie.Path != "/api/v1/projects/prj_demo/volume-transfers/vtx_demo/" {
		t.Fatalf("unexpected cookie identity: %#v", cookie)
	}
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie flags are not production safe: %#v", cookie)
	}
	if cookie.MaxAge < 1 || cookie.MaxAge > 20*60 || cookie.Expires.After(expiresAt) || expiresAt.Sub(cookie.Expires) > time.Second {
		t.Fatalf("cookie lifetime is outside the session bound: %#v", cookie)
	}
}

func TestVolumeDownloadSessionCookiePathIsSharedByContentAndManifestOnly(t *testing.T) {
	want := "/api/v1/projects/prj_demo/volume-transfers/vtx_demo/"
	for _, path := range []string{want + "content", want + "manifest"} {
		if got := volumeDownloadSessionCookiePath(path); got != want {
			t.Fatalf("cookie path for %q = %q, want %q", path, got, want)
		}
	}
	other := "/api/v1/projects/prj_demo/volume-transfers/vtx_other/manifest"
	if got := volumeDownloadSessionCookiePath(other); got == want {
		t.Fatalf("other transfer inherited cookie path %q", got)
	}
}

func TestVolumeDownloadSessionCookieAllowsLocalHTTPDevelopment(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/projects/prj_demo/volume-transfers/vtx_demo/content", nil)

	(&Handlers{mode: "development"}).setVolumeDownloadSessionCookie(ctx, volumeDownloadSession{
		Token: "vds_test_session", ExpiresAt: time.Now().Add(time.Minute),
	})

	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("unexpected development cookie: %#v", cookies)
	}
}

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
	router := NewRouter(db)
	routes := make(map[string]bool)
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	for _, expected := range []string{
		"HEAD /api/v1/projects/:projectId/volume-transfers/:transferId/manifest",
		"GET /api/v1/projects/:projectId/volume-transfers/:transferId/manifest",
	} {
		if !routes[expected] {
			t.Fatalf("route %q is not registered", expected)
		}
	}
}
