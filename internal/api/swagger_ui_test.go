package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSwaggerUIRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(securityHeaders())
	registerSwaggerUI(router)

	specReq := httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil)
	specRec := httptest.NewRecorder()
	router.ServeHTTP(specRec, specReq)
	if specRec.Code != http.StatusOK {
		t.Fatalf("openapi spec expected 200, got %d", specRec.Code)
	}
	if !strings.Contains(specRec.Body.String(), "openapi: 3.1.0") {
		t.Fatalf("openapi spec body does not look like the bundled spec")
	}

	uiReq := httptest.NewRequest(http.MethodGet, "/swagger", nil)
	uiRec := httptest.NewRecorder()
	router.ServeHTTP(uiRec, uiReq)
	if uiRec.Code != http.StatusMovedPermanently {
		t.Fatalf("swagger ui redirect expected 301, got %d", uiRec.Code)
	}

	indexReq := httptest.NewRequest(http.MethodGet, "/swagger/", nil)
	indexRec := httptest.NewRecorder()
	router.ServeHTTP(indexRec, indexReq)
	if indexRec.Code != http.StatusOK {
		t.Fatalf("swagger ui index expected 200, got %d", indexRec.Code)
	}
	if !strings.Contains(indexRec.Body.String(), `src="/swagger/swagger-initializer.js"`) {
		t.Fatalf("swagger ui body does not load the same-origin initializer")
	}
	if strings.Contains(indexRec.Body.String(), "<script>") {
		t.Fatalf("swagger ui index must not contain CSP-blocked inline scripts")
	}
	if csp := indexRec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'self';") ||
		strings.Contains(csp, "script-src 'self' 'unsafe-inline'") {
		t.Fatalf("swagger UI must remain compatible with the strict script policy: %q", csp)
	}

	initializerReq := httptest.NewRequest(http.MethodGet, "/swagger/swagger-initializer.js", nil)
	initializerRec := httptest.NewRecorder()
	router.ServeHTTP(initializerRec, initializerReq)
	if initializerRec.Code != http.StatusOK {
		t.Fatalf("swagger initializer expected 200, got %d", initializerRec.Code)
	}
	if contentType := initializerRec.Header().Get("Content-Type"); !strings.Contains(contentType, "application/javascript") {
		t.Fatalf("swagger initializer content type = %q", contentType)
	}
	if !strings.Contains(initializerRec.Body.String(), `url: "/openapi.yaml"`) {
		t.Fatalf("swagger initializer does not reference the bundled OpenAPI document")
	}

	bundleReq := httptest.NewRequest(http.MethodGet, "/swagger/swagger-ui-bundle.js", nil)
	bundleRec := httptest.NewRecorder()
	router.ServeHTTP(bundleRec, bundleReq)
	if bundleRec.Code != http.StatusOK {
		t.Fatalf("swagger bundle expected 200, got %d", bundleRec.Code)
	}
}
