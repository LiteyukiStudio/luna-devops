package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRuntimeResourceContractUsesDistinctStrictEnums(t *testing.T) {
	for _, category := range runtimeResourceCategories {
		if !validRuntimeResourceCategory(category) {
			t.Fatalf("resource category %q must be valid", category)
		}
		if validRuntimeResourceKind(category) {
			t.Fatalf("resource category %q leaked into resourceKind", category)
		}
	}
	for _, kind := range runtimeResourceKinds {
		if !validRuntimeResourceKind(kind) {
			t.Fatalf("resource kind %q must be valid", kind)
		}
		if validRuntimeResourceCategory(kind) {
			t.Fatalf("resource kind %q leaked into resourceCategory", kind)
		}
	}
	for _, invalid := range []string{"", "Deployment", "Pod", "workload", "WORKLOADS"} {
		if validRuntimeResourceCategory(invalid) {
			t.Fatalf("invalid resource category %q was accepted", invalid)
		}
	}
	for _, invalid := range []string{"deployment", "pods", "workloads", "NamespaceList"} {
		if validRuntimeResourceKind(invalid) {
			t.Fatalf("invalid resource kind %q was accepted", invalid)
		}
	}
}

func TestRuntimeResourceArgumentErrorIsStructuredAndNotRetryable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/runtime/clusters/cluster/resources", nil)
	writeRuntimeResourceArgumentError(ctx, "cluster.resource_category_invalid", "resourceCategory", runtimeResourceCategories)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"retryable":false`) ||
		!strings.Contains(recorder.Body.String(), `"allowedValues"`) || !strings.Contains(recorder.Body.String(), `"path":"resourceCategory"`) {
		t.Fatalf("unexpected structured error: %d %s", recorder.Code, recorder.Body.String())
	}
}
