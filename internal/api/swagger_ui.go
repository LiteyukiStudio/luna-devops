package api

import (
	"net/http"
	"strings"

	"github.com/LiteyukiStudio/devops/openapi"
	"github.com/gin-gonic/gin"
	"github.com/swaggest/swgui/v5emb"
)

const swaggerIndexHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Luna DevOps API - Swagger UI</title>
  <link rel="stylesheet" href="/swagger/swagger-ui.css">
  <link rel="icon" type="image/png" href="/swagger/favicon-32x32.png" sizes="32x32">
  <style>html{box-sizing:border-box;overflow-y:scroll}*,*:before,*:after{box-sizing:inherit}body{margin:0;background:#fafafa}</style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="/swagger/swagger-ui-bundle.js"></script>
  <script src="/swagger/swagger-ui-standalone-preset.js"></script>
  <script src="/swagger/swagger-initializer.js"></script>
</body>
</html>`

const swaggerInitializerJS = `"use strict";
window.addEventListener("load", function () {
  window.ui = SwaggerUIBundle({
    url: "/openapi.yaml",
    dom_id: "#swagger-ui",
    deepLinking: true,
    defaultModelsExpandDepth: -1,
    layout: "StandaloneLayout",
    presets: [SwaggerUIBundle.presets.apis, SwaggerUIStandalonePreset],
    plugins: [SwaggerUIBundle.plugins.DownloadUrl],
    showCommonExtensions: true,
    showExtensions: true,
    validatorUrl: null
  });
});
`

func registerSwaggerUI(router *gin.Engine) {
	router.GET("/openapi.yaml", func(ctx *gin.Context) {
		ctx.Data(http.StatusOK, "application/yaml; charset=utf-8", openapi.SpecYAML)
	})
	router.GET("/swagger", func(ctx *gin.Context) {
		ctx.Redirect(http.StatusMovedPermanently, "/swagger/")
	})
	assets := v5emb.New("Luna DevOps API", "/openapi.yaml", "/swagger/")
	router.Any("/swagger/*any", func(ctx *gin.Context) {
		switch strings.TrimPrefix(ctx.Param("any"), "/") {
		case "":
			ctx.Data(http.StatusOK, "text/html; charset=utf-8", []byte(swaggerIndexHTML))
		case "swagger-initializer.js":
			ctx.Data(http.StatusOK, "application/javascript; charset=utf-8", []byte(swaggerInitializerJS))
		default:
			assets.ServeHTTP(ctx.Writer, ctx.Request)
		}
	})
}
