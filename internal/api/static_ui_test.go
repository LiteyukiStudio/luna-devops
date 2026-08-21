package api

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
)

func TestStaticUIServesIndexWithoutRedirect(t *testing.T) {
	gin.SetMode(gin.TestMode)
	files := fstest.MapFS{
		"index.html": {
			Data: []byte("<!doctype html><title>Luna DevOps</title>"),
		},
		"assets/app.js": {
			Data: []byte("console.log('ok')"),
		},
		"luna-devops-logo.svg": {
			Data: []byte("<svg></svg>"),
		},
	}
	router := gin.New()
	registerStaticUI(router, files, nil)

	for _, path := range []string{"/", "/index.html", "/projects/prj_1/apps/app_1"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s expected 200, got %d", path, rec.Code)
		}
		if location := rec.Header().Get("Location"); location != "" {
			t.Fatalf("GET %s should not redirect, got Location %q", path, location)
		}
		if !strings.Contains(rec.Body.String(), "Luna DevOps") {
			t.Fatalf("GET %s expected index body, got %q", path, rec.Body.String())
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-cache, must-revalidate" {
			t.Fatalf("GET %s Cache-Control = %q", path, got)
		}
	}
}

func TestStaticUIServesBestPrecompressedAsset(t *testing.T) {
	gin.SetMode(gin.TestMode)
	files := fstest.MapFS{
		"index.html":       {Data: []byte("<!doctype html><title>Luna DevOps</title>")},
		"assets/app.js":    {Data: []byte("identity")},
		"assets/app.js.br": {Data: []byte("brotli")},
		"assets/app.js.gz": {Data: gzipBytes(t, []byte("gzip"))},
	}
	router := gin.New()
	registerStaticUI(router, files, nil)

	for _, test := range []struct {
		acceptEncoding string
		body           string
		encoding       string
	}{
		{acceptEncoding: "gzip, br", body: "brotli", encoding: "br"},
		{acceptEncoding: "br;q=0, gzip;q=1", body: "gzip", encoding: "gzip"},
		{acceptEncoding: "gzip;q=0, *;q=0.8", body: "brotli", encoding: "br"},
		{acceptEncoding: "identity", body: "identity", encoding: ""},
	} {
		req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
		req.Header.Set("Accept-Encoding", test.acceptEncoding)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("Accept-Encoding %q expected 200, got %d", test.acceptEncoding, rec.Code)
		}
		if got := rec.Header().Get("Content-Encoding"); got != test.encoding {
			t.Fatalf("Accept-Encoding %q Content-Encoding = %q, want %q", test.acceptEncoding, got, test.encoding)
		}
		body := rec.Body.Bytes()
		if test.encoding == "gzip" {
			reader, err := gzip.NewReader(strings.NewReader(rec.Body.String()))
			if err != nil {
				t.Fatalf("open gzip body: %v", err)
			}
			body, err = io.ReadAll(reader)
			if err != nil {
				t.Fatalf("read gzip body: %v", err)
			}
		}
		if string(body) != test.body {
			t.Fatalf("Accept-Encoding %q body = %q, want %q", test.acceptEncoding, body, test.body)
		}
		if !strings.Contains(rec.Header().Get("Vary"), "Accept-Encoding") {
			t.Fatalf("Accept-Encoding %q should vary on encoding", test.acceptEncoding)
		}
		if got := rec.Header().Get("Content-Type"); got != "text/javascript; charset=utf-8" {
			t.Fatalf("Accept-Encoding %q Content-Type = %q", test.acceptEncoding, got)
		}
	}
}

func gzipBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	var output strings.Builder
	writer := gzip.NewWriter(&output)
	if _, err := writer.Write(data); err != nil {
		t.Fatalf("write gzip fixture: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close gzip fixture: %v", err)
	}
	return []byte(output.String())
}

func TestStaticUIServesAssetsAndSkipsAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	files := fstest.MapFS{
		"index.html": {
			Data: []byte("<!doctype html><title>Luna DevOps</title>"),
		},
		"assets/app.js": {
			Data: []byte("console.log('ok')"),
		},
		"luna-devops-logo.svg": {
			Data: []byte("<svg></svg>"),
		},
	}
	router := gin.New()
	registerStaticUI(router, files, nil)

	assetReq := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	assetRec := httptest.NewRecorder()
	router.ServeHTTP(assetRec, assetReq)
	if assetRec.Code != http.StatusOK {
		t.Fatalf("asset expected 200, got %d", assetRec.Code)
	}
	if !strings.Contains(assetRec.Body.String(), "console.log") {
		t.Fatalf("asset expected file body, got %q", assetRec.Body.String())
	}
	if got := assetRec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("asset Cache-Control = %q", got)
	}

	publicReq := httptest.NewRequest(http.MethodGet, "/luna-devops-logo.svg", nil)
	publicRec := httptest.NewRecorder()
	router.ServeHTTP(publicRec, publicReq)
	if publicRec.Code != http.StatusOK {
		t.Fatalf("public asset expected 200, got %d", publicRec.Code)
	}
	if got := publicRec.Header().Get("Cache-Control"); got != "public, max-age=3600" {
		t.Fatalf("public asset Cache-Control = %q", got)
	}

	apiReq := httptest.NewRequest(http.MethodGet, "/api/unknown", nil)
	apiRec := httptest.NewRecorder()
	router.ServeHTTP(apiRec, apiReq)
	if apiRec.Code != http.StatusNotFound {
		t.Fatalf("api route expected 404, got %d", apiRec.Code)
	}
}

func TestStaticUIInjectsValidatedBrandThemeBeforeFirstPaint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	files := fstest.MapFS{
		"index.html": {
			Data: []byte(`<!doctype html><html data-brand-theme="__LUNA_DEVOPS_BRAND_THEME__"></html>`),
		},
	}

	for _, test := range []struct {
		name       string
		configured string
		want       string
	}{
		{name: "official preset", configured: "teal", want: `data-brand-theme="teal"`},
		{name: "invalid preset", configured: "url(javascript:bad)", want: `data-brand-theme="blue"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			registerStaticUI(router, files, func() string { return test.configured })
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

			if recorder.Code != http.StatusOK {
				t.Fatalf("GET / expected 200, got %d", recorder.Code)
			}
			if !strings.Contains(recorder.Body.String(), test.want) {
				t.Fatalf("index body %q does not contain %q", recorder.Body.String(), test.want)
			}
			if strings.Contains(recorder.Body.String(), brandThemeHTMLPlaceholder) {
				t.Fatalf("index body still contains theme placeholder: %q", recorder.Body.String())
			}
		})
	}
}
