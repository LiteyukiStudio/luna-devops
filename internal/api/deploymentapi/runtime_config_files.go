package deploymentapi

import (
	runtimeapi "github.com/LiteyukiStudio/devops/internal/api/runtimeapi"
	"github.com/gin-gonic/gin"
)

type runtimeConfigFileInput = runtimeapi.RuntimeConfigFileInput

func normalizeRuntimeConfigFilesInput(ctx *gin.Context, value string) (string, bool) {
	return runtimeapi.NormalizeRuntimeConfigFilesInput(ctx, value)
}

func normalizeRuntimeConfigFilePathInput(ctx *gin.Context, value string) (string, bool) {
	return runtimeapi.NormalizeRuntimeConfigFilePathInput(ctx, value)
}
