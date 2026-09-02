package runtimeapi

import (
	"net/http"
	"slices"

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
	writeArgumentErrorCode(ctx, http.StatusBadRequest, code, code, path, allowedValues, false)
}
