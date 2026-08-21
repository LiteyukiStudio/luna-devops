package api

import (
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func registerStaticUI(router *gin.Engine, staticFS fs.FS, brandTheme func() string) {
	if staticFS == nil {
		return
	}
	fileServer := http.FileServer(http.FS(staticFS))
	router.NoRoute(func(ctx *gin.Context) {
		if !staticUIRequestAllowed(ctx.Request) {
			ctx.Status(http.StatusNotFound)
			return
		}
		target := staticUIPath(ctx.Request.URL.Path)
		if target == "index.html" {
			serveStaticUIIndex(ctx, staticFS, brandTheme)
			return
		}
		if staticUIFileExists(staticFS, target) {
			setStaticUICacheHeaders(ctx, target)
			if staticUIFileExists(staticFS, target+".br") || staticUIFileExists(staticFS, target+".gz") {
				ctx.Writer.Header().Add("Vary", "Accept-Encoding")
			}
			encodedTarget, encoding := staticUIEncodedTarget(staticFS, target, ctx.GetHeader("Accept-Encoding"))
			if encoding != "" {
				header := ctx.Writer.Header()
				header.Set("Content-Encoding", encoding)
				if contentType := mime.TypeByExtension(path.Ext(target)); contentType != "" {
					header.Set("Content-Type", contentType)
				}
			}
			ctx.Request.URL.Path = "/" + encodedTarget
			fileServer.ServeHTTP(ctx.Writer, ctx.Request)
			return
		}
		serveStaticUIIndex(ctx, staticFS, brandTheme)
	})
}

func staticUIEncodedTarget(files fs.FS, target string, acceptEncoding string) (string, string) {
	quality := staticUIAcceptEncodingQuality(acceptEncoding)
	selectedTarget := target
	selectedEncoding := ""
	selectedQuality := 0.0
	for _, encoding := range []string{"br", "gzip"} {
		if quality[encoding] <= selectedQuality {
			continue
		}
		extension := "." + encoding
		if encoding == "gzip" {
			extension = ".gz"
		}
		encodedTarget := target + extension
		if staticUIFileExists(files, encodedTarget) {
			selectedTarget = encodedTarget
			selectedEncoding = encoding
			selectedQuality = quality[encoding]
		}
	}
	return selectedTarget, selectedEncoding
}

func staticUIAcceptEncodingQuality(header string) map[string]float64 {
	quality := map[string]float64{"br": 0, "gzip": 0}
	explicit := map[string]bool{"br": false, "gzip": false}
	wildcard := -1.0
	for _, item := range strings.Split(strings.ToLower(header), ",") {
		parts := strings.Split(strings.TrimSpace(item), ";")
		if len(parts) == 0 || parts[0] == "" {
			continue
		}
		value := 1.0
		for _, parameter := range parts[1:] {
			name, rawValue, found := strings.Cut(strings.TrimSpace(parameter), "=")
			if !found || name != "q" {
				continue
			}
			parsed, err := strconv.ParseFloat(rawValue, 64)
			if err != nil || parsed < 0 || parsed > 1 {
				value = 0
			} else {
				value = parsed
			}
		}
		switch parts[0] {
		case "br", "gzip":
			quality[parts[0]] = value
			explicit[parts[0]] = true
		case "*":
			wildcard = value
		}
	}
	if wildcard >= 0 {
		for encoding := range quality {
			if !explicit[encoding] {
				quality[encoding] = wildcard
			}
		}
	}
	return quality
}

func staticUIRequestAllowed(request *http.Request) bool {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		return false
	}
	cleanPath := path.Clean("/" + strings.TrimSpace(request.URL.Path))
	return cleanPath != "/healthz" && !strings.HasPrefix(cleanPath, "/api/")
}

func staticUIPath(rawPath string) string {
	cleanPath := strings.TrimPrefix(path.Clean("/"+rawPath), "/")
	if cleanPath == "." || cleanPath == "" {
		return "index.html"
	}
	return cleanPath
}

func staticUIFileExists(files fs.FS, name string) bool {
	info, err := fs.Stat(files, name)
	return err == nil && !info.IsDir()
}

func serveStaticUIIndex(ctx *gin.Context, staticFS fs.FS, brandTheme func() string) {
	data, err := fs.ReadFile(staticFS, "index.html")
	if err != nil {
		ctx.Status(http.StatusNotFound)
		return
	}
	preset := defaultBrandColorPreset
	if brandTheme != nil {
		preset = normalizeBrandColorPreset(brandTheme())
	}
	data = []byte(strings.ReplaceAll(string(data), brandThemeHTMLPlaceholder, preset))
	setStaticUICacheHeaders(ctx, "index.html")
	ctx.Data(http.StatusOK, "text/html; charset=utf-8", data)
}

func setStaticUICacheHeaders(ctx *gin.Context, target string) {
	header := ctx.Writer.Header()
	switch {
	case target == "index.html":
		header.Set("Cache-Control", "no-cache, must-revalidate")
	case strings.HasPrefix(target, "assets/"):
		header.Set("Cache-Control", "public, max-age=31536000, immutable")
	default:
		header.Set("Cache-Control", "public, max-age=3600")
	}
}
