package platformapi

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"

	"github.com/LiteyukiStudio/devops/openapi"
	"github.com/gin-gonic/gin"
)

const (
	apiVersion        = "v1"
	minimumCLIVersion = "0.0.7"
)

type apiMetaFeatures struct {
	AccessToken        bool `json:"accessToken"`
	DeviceCode         bool `json:"deviceCode"`
	OAuthAuthorization bool `json:"oauthAuthorization"`
	OpenAPIOperations  bool `json:"openapiOperations"`
	KubectlGateway     bool `json:"kubectlGateway"`
}

type apiMetaResponse struct {
	APIVersion        string          `json:"apiVersion"`
	ServerVersion     string          `json:"serverVersion"`
	OpenAPIDigest     string          `json:"openapiDigest"`
	MinimumCLIVersion string          `json:"minimumCliVersion"`
	Features          apiMetaFeatures `json:"features"`
}

func (h *Handler) GetAPIMeta(ctx *gin.Context) {
	ctx.Header("Cache-Control", "no-store")
	ctx.JSON(http.StatusOK, apiMetaResponse{
		APIVersion:        apiVersion,
		ServerVersion:     h.config().AppVersion,
		OpenAPIDigest:     openAPIDigest(),
		MinimumCLIVersion: minimumCLIVersion,
		Features: apiMetaFeatures{
			AccessToken:        true,
			DeviceCode:         true,
			OAuthAuthorization: true,
			OpenAPIOperations:  true,
			KubectlGateway:     true,
		},
	})
}

func openAPIDigest() string {
	digest := sha256.Sum256(openapi.SpecYAML)
	return "sha256:" + hex.EncodeToString(digest[:])
}
