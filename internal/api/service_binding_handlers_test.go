package api

import (
	"encoding/json"
	"github.com/LiteyukiStudio/devops/internal/api/projectapi"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/api/applicationapi"
	"github.com/LiteyukiStudio/devops/internal/dependency"
	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/gin-gonic/gin"
)

func TestWriteDependencyErrorUsesStableCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	applicationapi.WriteDependencyError(ctx, &dependency.DomainError{Code: dependency.CodePortNotFound})
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", recorder.Code)
	}
	if body := recorder.Body.String(); !strings.Contains(body, `"code":"service_binding_port_not_found"`) {
		t.Fatalf("response = %s", body)
	}
}

func TestTopologyOriginsOnlyAcceptsKnownValues(t *testing.T) {
	origins := projectapi.TopologyOrigins("manual,service_binding,unknown")
	if len(origins) != 2 || !origins["manual"] || !origins["service_binding"] {
		t.Fatalf("origins = %#v", origins)
	}
}

func TestDependencyPaginationUsesSortWhitelist(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/?sortBy=created_at%20desc%3Bdrop%20table%20users&sortOrder=asc", nil)
	pagination := applicationapi.DependencyPagination(ctx, map[string]bool{"updatedAt": true}, "updatedAt")
	if pagination.SortBy != "updatedAt" || pagination.SortOrder != "asc" {
		t.Fatalf("pagination = %#v", pagination)
	}
}

func TestServiceBindingMutationResponseMatchesAPIContract(t *testing.T) {
	binding := model.ServiceBinding{
		ID: "sbind_1", ProjectID: "prj_1", SourceApplicationID: "app_1", SourceDeploymentTargetID: "dplt_1",
	}
	body, err := json.Marshal(applicationapi.ServiceBindingMutationResponseFor(binding))
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	encoded := string(body)
	for _, expected := range []string{`"item"`, `"requiresRedeploy":true`, `"affectedDeploymentTargets"`, `"applicationId":"app_1"`, `"deploymentTargetId":"dplt_1"`} {
		if !strings.Contains(encoded, expected) {
			t.Fatalf("response %s does not contain %s", encoded, expected)
		}
	}
	for _, obsolete := range []string{`"binding"`, `"affectedDeploymentTargetIds"`} {
		if strings.Contains(encoded, obsolete) {
			t.Fatalf("response %s contains obsolete field %s", encoded, obsolete)
		}
	}
}
