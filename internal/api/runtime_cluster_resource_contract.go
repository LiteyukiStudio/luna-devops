package api

import (
	"net/http"
	"slices"

	"github.com/LiteyukiStudio/devops/internal/telemetry"
	"github.com/gin-gonic/gin"
)

var runtimeResourceCategories = []string{"namespaces", "workloads", "services", "configs", "storage"}

var runtimeResourceKinds = []string{
	"Namespace",
	"Deployment",
	"StatefulSet",
	"Pod",
	"HorizontalPodAutoscaler",
	"Service",
	"HTTPRoute",
	"Gateway",
	"ConfigMap",
	"Secret",
	"PersistentVolumeClaim",
}

func validRuntimeResourceCategory(value string) bool {
	return slices.Contains(runtimeResourceCategories, value)
}

func validRuntimeResourceKind(value string) bool {
	return slices.Contains(runtimeResourceKinds, value)
}

func writeRuntimeResourceArgumentError(ctx *gin.Context, code, path string, allowedValues []string) {
	telemetry.SetHTTPError(ctx, code, code)
	response := errorEnvelope(ctx, http.StatusBadRequest, code)
	response["retryable"] = false
	response["path"] = path
	response["allowedValues"] = allowedValues
	ctx.JSON(http.StatusBadRequest, response)
}
