package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/openapi"
	"github.com/gin-gonic/gin"
)

func TestGetAPIMeta(t *testing.T) {
	t.Setenv("APP_VERSION", "0.1.0-test")
	gin.SetMode(gin.TestMode)

	handlers := &Handlers{}
	router := gin.New()
	router.GET("/api/v1/meta", handlers.GetAPIMeta)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/meta", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}

	var response apiMetaResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.APIVersion != "v1" {
		t.Fatalf("apiVersion = %q, want v1", response.APIVersion)
	}
	if response.ServerVersion != "0.1.0-test" {
		t.Fatalf("serverVersion = %q, want 0.1.0-test", response.ServerVersion)
	}
	if !strings.HasPrefix(response.OpenAPIDigest, "sha256:") || len(response.OpenAPIDigest) != len("sha256:")+64 {
		t.Fatalf("unexpected openapiDigest %q", response.OpenAPIDigest)
	}
	if response.MinimumCLIVersion != minimumCLIVersion {
		t.Fatalf("minimumCliVersion = %q, want %q", response.MinimumCLIVersion, minimumCLIVersion)
	}
	if !response.Features.AccessToken ||
		!response.Features.OAuthAuthorization ||
		!response.Features.DeviceCode ||
		!response.Features.MFABearer ||
		!response.Features.OpenAPIOperations {
		t.Fatalf("expected stable CLI features to be enabled: %#v", response.Features)
	}
	if len(openapi.SpecYAML) == 0 {
		t.Fatal("embedded OpenAPI specification is empty")
	}
}
