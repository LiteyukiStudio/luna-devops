package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestNormalizePublicEnvironmentVariablesDoesNotGuessSecretSemantics(t *testing.T) {
	items := []runtimeEnvironmentVariableInput{
		{Key: "DATABASE_PASSWORD", ValueMode: runtimeEnvironmentValueModePublic, Value: "value"},
		{Key: "REDIS_PASS", ValueMode: runtimeEnvironmentValueModePublic, Value: "value"},
		{Key: "APIKEY", ValueMode: runtimeEnvironmentValueModePublic, Value: "value"},
		{Key: "AUTH", ValueMode: runtimeEnvironmentValueModePublic, Value: "value"},
		{Key: "DATABASE_DSN", ValueMode: runtimeEnvironmentValueModePublic, Value: "postgres://database.internal/app"},
		{Key: "URL_WITH_CREDENTIALS", ValueMode: runtimeEnvironmentValueModePublic, Value: "https://user:password@example.com/api"},
		{Key: "URL_WITH_TOKEN_QUERY", ValueMode: runtimeEnvironmentValueModePublic, Value: "https://example.com/api?password=value"},
	}
	ctx, _ := testRuntimeSecretContext()
	values, ok := normalizePublicEnvironmentVariables(ctx, items)
	if !ok {
		t.Fatal("normalizePublicEnvironmentVariables() rejected caller-selected public values")
	}
	for _, item := range items {
		if values[item.Key] != item.Value {
			t.Fatalf("value[%q] = %q, want %q", item.Key, values[item.Key], item.Value)
		}
	}
}

func TestNormalizePublicEnvironmentVariablesRequiresExplicitPublicMode(t *testing.T) {
	ctx, recorder := testRuntimeSecretContext()
	_, ok := normalizePublicEnvironmentVariables(ctx, []runtimeEnvironmentVariableInput{{Key: "TOKEN", ValueMode: runtimeEnvironmentValueModeSecret}})
	if ok {
		t.Fatal("normalizePublicEnvironmentVariables() accepted a secret item")
	}
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	assertRuntimeSecretErrorCode(t, recorder, "deployment.runtime_environment_value_mode_invalid")
}

func TestNormalizePublicEnvironmentVariablesEnforcesOpenAPILimits(t *testing.T) {
	items := make([]runtimeEnvironmentVariableInput, maxRuntimeEnvironmentVariables+1)
	for index := range items {
		items[index] = runtimeEnvironmentVariableInput{Key: "KEY_" + strconv.Itoa(index), ValueMode: runtimeEnvironmentValueModePublic}
	}
	for _, test := range []struct {
		name     string
		items    []runtimeEnvironmentVariableInput
		wantCode string
	}{
		{name: "too many items", items: items, wantCode: "deployment.runtime_environment_items_invalid"},
		{name: "value too long", items: []runtimeEnvironmentVariableInput{{Key: "LOG_LEVEL", ValueMode: runtimeEnvironmentValueModePublic, Value: strings.Repeat("密", maxRuntimeEnvironmentValueLength+1)}}, wantCode: "deployment.runtime_environment_value_too_long"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, recorder := testRuntimeSecretContext()
			if _, ok := normalizePublicEnvironmentVariables(ctx, test.items); ok {
				t.Fatal("normalizePublicEnvironmentVariables() accepted an over-limit request")
			}
			assertRuntimeSecretErrorCode(t, recorder, test.wantCode)
		})
	}
}

func TestNormalizePublicEnvironmentVariablesAcceptsOpenAPIMultibyteBoundary(t *testing.T) {
	ctx, _ := testRuntimeSecretContext()
	value := strings.Repeat("密", maxRuntimeEnvironmentValueLength)
	values, ok := normalizePublicEnvironmentVariables(ctx, []runtimeEnvironmentVariableInput{{Key: "MESSAGE", ValueMode: runtimeEnvironmentValueModePublic, Value: value}})
	if !ok || values["MESSAGE"] != value {
		t.Fatal("normalizePublicEnvironmentVariables() rejected an OpenAPI-valid multibyte value")
	}
}

func TestRuntimeEnvironmentVariablesReturnsBothModesForOverlappingKey(t *testing.T) {
	items := runtimeEnvironmentVariables(`{"LOG_LEVEL":"debug","TOKEN":"plaintext"}`, `{"TOKEN":"secret-id:token"}`)
	if len(items) != 3 {
		t.Fatalf("variables = %#v, want public and secret entries", items)
	}
	if items[1].Key != "TOKEN" || items[1].ValueMode != runtimeEnvironmentValueModePublic || items[1].Value != "plaintext" {
		t.Fatalf("public TOKEN response = %#v, want retained public value", items[1])
	}
	if items[2].Key != "TOKEN" || items[2].ValueMode != runtimeEnvironmentValueModeSecret || items[2].Value != "" || !items[2].Configured {
		t.Fatalf("secret TOKEN response = %#v, want configured secret without value", items[2])
	}
}

func TestRuntimeSecretMutationRequestRequiresSecretModeAndExplicitOperation(t *testing.T) {
	tests := []runtimeSecretMutationRequest{
		{},
		{Items: []runtimeSecretMutationRequestItem{{Key: "TOKEN", ValueMode: runtimeEnvironmentValueModePublic, Operation: "set", Value: "value"}}},
		{Items: []runtimeSecretMutationRequestItem{{Key: "TOKEN", ValueMode: runtimeEnvironmentValueModeSecret}}},
		{Items: []runtimeSecretMutationRequestItem{{Key: "TOKEN", ValueMode: runtimeEnvironmentValueModeSecret, Operation: "clear", Value: "must-not-be-accepted"}}},
	}
	for _, request := range tests {
		ctx, recorder := testRuntimeSecretContext()
		if _, ok := runtimeSecretMutationInputFromRequest(ctx, request); ok {
			t.Fatalf("runtimeSecretMutationInputFromRequest(%#v) accepted unsafe input", request)
		}
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
		}
	}
}

func TestRuntimeSecretMutationRequestEnforcesOpenAPILimits(t *testing.T) {
	items := make([]runtimeSecretMutationRequestItem, maxRuntimeEnvironmentVariables+1)
	for index := range items {
		items[index] = runtimeSecretMutationRequestItem{Key: "KEY_" + strconv.Itoa(index), ValueMode: runtimeEnvironmentValueModeSecret, Operation: "clear"}
	}
	for _, test := range []struct {
		name     string
		request  runtimeSecretMutationRequest
		wantCode string
	}{
		{name: "too many items", request: runtimeSecretMutationRequest{Items: items}, wantCode: "deployment.secret_items_invalid"},
		{name: "value too long", request: runtimeSecretMutationRequest{Items: []runtimeSecretMutationRequestItem{{Key: "TOKEN", ValueMode: runtimeEnvironmentValueModeSecret, Operation: "set", Value: strings.Repeat("密", maxRuntimeEnvironmentValueLength+1)}}}, wantCode: "deployment.secret_value_too_long"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, recorder := testRuntimeSecretContext()
			if _, ok := runtimeSecretMutationInputFromRequest(ctx, test.request); ok {
				t.Fatal("runtimeSecretMutationInputFromRequest() accepted an over-limit request")
			}
			assertRuntimeSecretErrorCode(t, recorder, test.wantCode)
		})
	}
}

func TestRuntimeSecretMutationRequestAcceptsOpenAPIMultibyteBoundary(t *testing.T) {
	ctx, _ := testRuntimeSecretContext()
	value := strings.Repeat("密", maxRuntimeEnvironmentValueLength)
	input, ok := runtimeSecretMutationInputFromRequest(ctx, runtimeSecretMutationRequest{Items: []runtimeSecretMutationRequestItem{{
		Key: "TOKEN", ValueMode: runtimeEnvironmentValueModeSecret, Operation: "set", Value: value,
	}}})
	if !ok || input.Values["TOKEN"] != value {
		t.Fatal("runtimeSecretMutationInputFromRequest() rejected an OpenAPI-valid multibyte value")
	}
}

func TestRuntimeSecretMutationEmptySetKeepsExistingValue(t *testing.T) {
	ctx, _ := testRuntimeSecretContext()
	input, ok := runtimeSecretMutationInputFromRequest(ctx, runtimeSecretMutationRequest{Items: []runtimeSecretMutationRequestItem{{
		Key: "TOKEN", ValueMode: runtimeEnvironmentValueModeSecret, Operation: "set", Value: "",
	}}})
	if !ok {
		t.Fatal("empty set request should be accepted as retain-existing")
	}
	prepared, err := prepareRuntimeSecretMutation(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.values) != 0 || len(prepared.configuredKey) != 0 {
		t.Fatalf("empty set prepared mutation = %#v, want no-op", prepared)
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

func assertRuntimeSecretErrorCode(t *testing.T, recorder *httptest.ResponseRecorder, want string) {
	t.Helper()
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["code"] != want {
		t.Fatalf("code = %#v, want %q", body["code"], want)
	}
}
