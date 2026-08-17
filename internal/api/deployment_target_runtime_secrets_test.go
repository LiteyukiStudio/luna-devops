package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestValidateDeploymentTargetPublicEnvVarsRejectsSecretSemantics(t *testing.T) {
	for _, values := range []map[string]string{
		{"DATABASE_PASSWORD": "value"},
		{"PUBLIC_URL": "https://user:password@example.com/api"},
	} {
		ctx, recorder := testRuntimeSecretContext()
		if validateDeploymentTargetPublicEnvVars(ctx, values) {
			t.Fatalf("validateDeploymentTargetPublicEnvVars(%#v) = true, want false", values)
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

func TestValidateDeploymentTargetRuntimeSecretMutationRejectsConflictingFields(t *testing.T) {
	ctx, recorder := testRuntimeSecretContext()
	input := runtimeSecretMutationInput{
		Values:   map[string]string{"TOKEN": "value"},
		Generate: map[string]runtimeSecretGeneration{"TOKEN": {Length: 32, Encoding: "base64"}},
	}
	if validateRuntimeSecretMutation(ctx, &input) {
		t.Fatal("validateDeploymentTargetRuntimeSecretMutation() = true, want false")
	}
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestRuntimeSecretKeysExposeOnlyConfiguredBuildEnvironmentKeys(t *testing.T) {
	keys := runtimeSecretKeys(`{"TOKEN":"secret-id:token","EMPTY":"","bad.key":"secret-id:bad","PASSWORD":"secret:v1:password"}`)
	if got, want := len(keys), 2; got != want {
		t.Fatalf("runtimeSecretKeys() returned %d keys, want %d: %#v", got, want, keys)
	}
	if keys[0] != "PASSWORD" || keys[1] != "TOKEN" {
		t.Fatalf("runtimeSecretKeys() = %#v, want sorted configured keys", keys)
	}
}

func TestRuntimeSecretResponsesSetNoStoreHeaders(t *testing.T) {
	ctx, recorder := testRuntimeSecretContext()
	setRuntimeSecretNoStoreHeaders(ctx)
	if got := recorder.Header().Get("Cache-Control"); got != "no-store, private" {
		t.Fatalf("Cache-Control = %q, want no-store, private", got)
	}
	if got := recorder.Header().Get("Pragma"); got != "no-cache" {
		t.Fatalf("Pragma = %q, want no-cache", got)
	}
	if got := recorder.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
}

func testRuntimeSecretContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	return ctx, recorder
}
