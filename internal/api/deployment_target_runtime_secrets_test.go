package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestValidateDeploymentTargetPublicEnvVarsRejectsSecretSemantics(t *testing.T) {
	for _, raw := range []string{
		`{"DATABASE_PASSWORD":"value"}`,
		`PUBLIC_URL=https://user:password@example.com/api`,
	} {
		ctx, recorder := testRuntimeSecretContext()
		if validateDeploymentTargetPublicEnvVars(ctx, raw) {
			t.Fatalf("validateDeploymentTargetPublicEnvVars(%q) = true, want false", raw)
		}
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
		}
		var body map[string]any
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if body["code"] != "deployment.secret_must_use_secure_input" {
			t.Fatalf("code = %#v, want stable sensitive-input code", body["code"])
		}
	}
}

func TestValidateDeploymentTargetSecretRefsRejectsPlaintext(t *testing.T) {
	ctx, recorder := testRuntimeSecretContext()
	if validateDeploymentTargetSecretRefs(ctx, `{"TOKEN":"plaintext"}`) {
		t.Fatal("validateDeploymentTargetSecretRefs() = true, want false")
	}
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestValidateDeploymentTargetRuntimeSecretMutationRejectsConflictingFields(t *testing.T) {
	ctx, recorder := testRuntimeSecretContext()
	input := deploymentTargetRuntimeSecretsInput{
		Values:   map[string]string{"TOKEN": "value"},
		Generate: map[string]deploymentTargetRuntimeSecretGeneration{"TOKEN": {Length: 32, Encoding: "base64"}},
	}
	if validateDeploymentTargetRuntimeSecretMutation(ctx, &input) {
		t.Fatal("validateDeploymentTargetRuntimeSecretMutation() = true, want false")
	}
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func testRuntimeSecretContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	return ctx, recorder
}
