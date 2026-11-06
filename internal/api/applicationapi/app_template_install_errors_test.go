package applicationapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/appstore"
	"github.com/LiteyukiStudio/devops/internal/volume"
	"github.com/gin-gonic/gin"
)

func templateErrorRecorder(t *testing.T, write func(*gin.Context)) (int, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/projects/prj_demo/app-templates/demo/install", nil)
	write(ctx)
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body: %v (%s)", err, recorder.Body.String())
	}
	return recorder.Code, body
}

func TestTemplateValuesErrorsUseStableCodes(t *testing.T) {
	for _, test := range []struct {
		err    error
		status int
		code   string
	}{
		{err: &appstore.ValueRequiredError{Key: "baseUrl"}, status: http.StatusBadRequest, code: "app_template.value_required"},
		{err: fmt.Errorf("%w: entropy unavailable", appstore.ErrRandomSecretUnavailable), status: http.StatusInternalServerError, code: "app_template.secret_generation_failed"},
	} {
		status, body := templateErrorRecorder(t, func(ctx *gin.Context) { writeTemplateValuesError(ctx, test.err) })
		if status != test.status || body["code"] != test.code {
			t.Fatalf("writeTemplateValuesError(%v) = %d %#v", test.err, status, body)
		}
		if strings.Contains(fmt.Sprint(body), "entropy unavailable") || strings.Contains(fmt.Sprint(body), "baseUrl") {
			t.Fatalf("production error body echoed the raw error: %#v", body)
		}
	}
}

func TestTemplateInstallTransactionErrorsAreClassified(t *testing.T) {
	for _, test := range []struct {
		err  error
		code string
	}{
		{err: errApplicationIdentifierExists, code: "application.identifier_exists"},
		{err: &volume.DomainError{Code: volume.CodeBindingConflict, Message: "conflict"}, code: volume.CodeBindingConflict},
		{err: errors.New(`ERROR: duplicate key value violates unique constraint "idx_deployment_targets_kubernetes_name" (SQLSTATE 23505)`), code: "app_template.install_conflict"},
		{err: errors.New("connection reset by peer"), code: "app_template.install_failed"},
	} {
		if got := appTemplateInstallErrorCode(test.err); got != test.code {
			t.Fatalf("appTemplateInstallErrorCode(%v) = %q, want %q", test.err, got, test.code)
		}
	}
	handlers := &Handlers{}
	for _, test := range []struct {
		err    error
		status int
		code   string
	}{
		{err: errors.New("pq: SQLSTATE 23505"), status: http.StatusConflict, code: "app_template.install_conflict"},
		{err: errors.New("relation does not exist"), status: http.StatusInternalServerError, code: "app_template.install_failed"},
		{err: errApplicationIdentifierExists, status: http.StatusConflict, code: "application.identifier_exists"},
	} {
		status, body := templateErrorRecorder(t, func(ctx *gin.Context) { handlers.writeTemplateInstallError(ctx, test.err) })
		if status != test.status || body["code"] != test.code {
			t.Fatalf("writeTemplateInstallError(%v) = %d %#v", test.err, status, body)
		}
		if strings.Contains(fmt.Sprint(body), "relation does not exist") {
			t.Fatalf("production error body echoed the raw error: %#v", body)
		}
	}
}
