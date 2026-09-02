package deploymentapi

import (
	runtimeapi "github.com/LiteyukiStudio/devops/internal/api/runtimeapi"
	"github.com/gin-gonic/gin"
)

type runtimeEnvironmentVariableInput = runtimeapi.RuntimeEnvironmentVariableInput
type runtimeEnvironmentVariableResponse = runtimeapi.RuntimeEnvironmentVariableResponse

func normalizePublicEnvironmentVariables(ctx *gin.Context, items []runtimeEnvironmentVariableInput) (map[string]string, bool) {
	return runtimeapi.NormalizePublicEnvironmentVariables(ctx, items)
}

func runtimeEnvironmentVariables(publicRaw, secretRaw string) []runtimeEnvironmentVariableResponse {
	return runtimeapi.RuntimeEnvironmentVariables(publicRaw, secretRaw)
}

func publicEnvironmentVariableInputs(raw string) []runtimeEnvironmentVariableInput {
	return runtimeapi.PublicEnvironmentVariableInputs(raw)
}
