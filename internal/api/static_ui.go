package api

import (
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
)

func registerStaticUI(router *gin.Engine, staticFS fs.FS) {
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
			serveStaticUIIndex(ctx, staticFS)
			return
		}
		if staticUIFileExists(staticFS, target) {
			setStaticUICacheHeaders(ctx, target)
			ctx.Request.URL.Path = "/" + target
			fileServer.ServeHTTP(ctx.Writer, ctx.Request)
			return
		}
		serveStaticUIIndex(ctx, staticFS)
	})
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

func serveStaticUIIndex(ctx *gin.Context, staticFS fs.FS) {
	data, err := fs.ReadFile(staticFS, "index.html")
	if err != nil {
		ctx.Status(http.StatusNotFound)
		return
	}
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
