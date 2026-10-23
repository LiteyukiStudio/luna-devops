package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/api/runtimeapi"
	"github.com/gin-gonic/gin"
)

func TestNormalizePublicEnvironmentVariablesDoesNotGuessSecretSemantics(t *testing.T) {
	items := []runtimeapi.RuntimeEnvironmentVariableInput{
		{Key: "DATABASE_PASSWORD", ValueMode: runtimeapi.RuntimeEnvironmentValueModePublic, Value: "value"},
		{Key: "REDIS_PASS", ValueMode: runtimeapi.RuntimeEnvironmentValueModePublic, Value: "value"},
		{Key: "APIKEY", ValueMode: runtimeapi.RuntimeEnvironmentValueModePublic, Value: "value"},
		{Key: "AUTH", ValueMode: runtimeapi.RuntimeEnvironmentValueModePublic, Value: "value"},
		{Key: "DATABASE_DSN", ValueMode: runtimeapi.RuntimeEnvironmentValueModePublic, Value: "postgres://database.internal/app"},
		{Key: "URL_WITH_CREDENTIALS", ValueMode: runtimeapi.RuntimeEnvironmentValueModePublic, Value: "https://user:password@example.com/api"},
		{Key: "URL_WITH_TOKEN_QUERY", ValueMode: runtimeapi.RuntimeEnvironmentValueModePublic, Value: "https://example.com/api?password=value"},
	}
	ctx, _ := testRuntimeSecretContext()
	values, ok := runtimeapi.NormalizePublicEnvironmentVariables(ctx, items)
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
	_, ok := runtimeapi.NormalizePublicEnvironmentVariables(ctx, []runtimeapi.RuntimeEnvironmentVariableInput{{Key: "TOKEN", ValueMode: runtimeapi.RuntimeEnvironmentValueModeSecret}})
	if ok {
		t.Fatal("normalizePublicEnvironmentVariables() accepted a secret item")
	}
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	assertRuntimeSecretErrorCode(t, recorder, "deployment.runtime_environment_value_mode_invalid")
}

func TestNormalizePublicEnvironmentVariablesEnforcesOpenAPILimits(t *testing.T) {
	items := make([]runtimeapi.RuntimeEnvironmentVariableInput, runtimeapi.MaxRuntimeEnvironmentVariables+1)
	for index := range items {
		items[index] = runtimeapi.RuntimeEnvironmentVariableInput{Key: "KEY_" + strconv.Itoa(index), ValueMode: runtimeapi.RuntimeEnvironmentValueModePublic}
	}
	for _, test := range []struct {
		name     string
		items    []runtimeapi.RuntimeEnvironmentVariableInput
		wantCode string
	}{
		{name: "too many items", items: items, wantCode: "deployment.runtime_environment_items_invalid"},
		{name: "value too long", items: []runtimeapi.RuntimeEnvironmentVariableInput{{Key: "LOG_LEVEL", ValueMode: runtimeapi.RuntimeEnvironmentValueModePublic, Value: strings.Repeat("密", runtimeapi.MaxRuntimeEnvironmentValueLength+1)}}, wantCode: "deployment.runtime_environment_value_too_long"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, recorder := testRuntimeSecretContext()
			if _, ok := runtimeapi.NormalizePublicEnvironmentVariables(ctx, test.items); ok {
				t.Fatal("normalizePublicEnvironmentVariables() accepted an over-limit request")
			}
			assertRuntimeSecretErrorCode(t, recorder, test.wantCode)
		})
	}
}

func TestNormalizePublicEnvironmentVariablesAcceptsOpenAPIMultibyteBoundary(t *testing.T) {
	ctx, _ := testRuntimeSecretContext()
	value := strings.Repeat("密", runtimeapi.MaxRuntimeEnvironmentValueLength)
	values, ok := runtimeapi.NormalizePublicEnvironmentVariables(ctx, []runtimeapi.RuntimeEnvironmentVariableInput{{Key: "MESSAGE", ValueMode: runtimeapi.RuntimeEnvironmentValueModePublic, Value: value}})
	if !ok || values["MESSAGE"] != value {
		t.Fatal("normalizePublicEnvironmentVariables() rejected an OpenAPI-valid multibyte value")
	}
}

func TestRuntimeEnvironmentVariablesReturnsBothModesForOverlappingKey(t *testing.T) {
	items := runtimeapi.RuntimeEnvironmentVariables(`{"LOG_LEVEL":"debug","TOKEN":"plaintext"}`, `{"TOKEN":"secret-id:token"}`)
	if len(items) != 3 {
		t.Fatalf("variables = %#v, want public and secret entries", items)
	}
	if items[1].Key != "TOKEN" || items[1].ValueMode != runtimeapi.RuntimeEnvironmentValueModePublic || items[1].Value != "plaintext" {
		t.Fatalf("public TOKEN response = %#v, want retained public value", items[1])
	}
	if items[2].Key != "TOKEN" || items[2].ValueMode != runtimeapi.RuntimeEnvironmentValueModeSecret || items[2].Value != "" || !items[2].Configured {
		t.Fatalf("secret TOKEN response = %#v, want configured secret without value", items[2])
	}
}

func TestRuntimeSecretMutationRequestRequiresSecretModeAndExplicitOperation(t *testing.T) {
	tests := []runtimeapi.RuntimeSecretMutationRequest{
		{},
		{Items: []runtimeapi.RuntimeSecretMutationRequestItem{{Key: "TOKEN", ValueMode: runtimeapi.RuntimeEnvironmentValueModePublic, Operation: "set", Value: "value"}}},
		{Items: []runtimeapi.RuntimeSecretMutationRequestItem{{Key: "TOKEN", ValueMode: runtimeapi.RuntimeEnvironmentValueModeSecret}}},
		{Items: []runtimeapi.RuntimeSecretMutationRequestItem{{Key: "TOKEN", ValueMode: runtimeapi.RuntimeEnvironmentValueModeSecret, Operation: "clear", Value: "must-not-be-accepted"}}},
	}
	for _, request := range tests {
		ctx, recorder := testRuntimeSecretContext()
		if _, ok := runtimeapi.RuntimeSecretMutationInputFromRequest(ctx, request); ok {
			t.Fatalf("runtimeSecretMutationInputFromRequest(%#v) accepted unsafe input", request)
		}
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
		}
	}
}

func TestRuntimeSecretMutationRequestEnforcesOpenAPILimits(t *testing.T) {
	items := make([]runtimeapi.RuntimeSecretMutationRequestItem, runtimeapi.MaxRuntimeEnvironmentVariables+1)
	for index := range items {
		items[index] = runtimeapi.RuntimeSecretMutationRequestItem{Key: "KEY_" + strconv.Itoa(index), ValueMode: runtimeapi.RuntimeEnvironmentValueModeSecret, Operation: "clear"}
	}
	for _, test := range []struct {
		name     string
		request  runtimeapi.RuntimeSecretMutationRequest
		wantCode string
	}{
		{name: "too many items", request: runtimeapi.RuntimeSecretMutationRequest{Items: items}, wantCode: "deployment.secret_items_invalid"},
		{name: "value too long", request: runtimeapi.RuntimeSecretMutationRequest{Items: []runtimeapi.RuntimeSecretMutationRequestItem{{Key: "TOKEN", ValueMode: runtimeapi.RuntimeEnvironmentValueModeSecret, Operation: "set", Value: strings.Repeat("密", runtimeapi.MaxRuntimeEnvironmentValueLength+1)}}}, wantCode: "deployment.secret_value_too_long"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, recorder := testRuntimeSecretContext()
			if _, ok := runtimeapi.RuntimeSecretMutationInputFromRequest(ctx, test.request); ok {
				t.Fatal("runtimeSecretMutationInputFromRequest() accepted an over-limit request")
			}
			assertRuntimeSecretErrorCode(t, recorder, test.wantCode)
		})
	}
}

func TestRuntimeSecretMutationRequestAcceptsOpenAPIMultibyteBoundary(t *testing.T) {
	ctx, _ := testRuntimeSecretContext()
	value := strings.Repeat("密", runtimeapi.MaxRuntimeEnvironmentValueLength)
	input, ok := runtimeapi.RuntimeSecretMutationInputFromRequest(ctx, runtimeapi.RuntimeSecretMutationRequest{Items: []runtimeapi.RuntimeSecretMutationRequestItem{{
		Key: "TOKEN", ValueMode: runtimeapi.RuntimeEnvironmentValueModeSecret, Operation: "set", Value: value,
	}}})
	if !ok || input.Values["TOKEN"] != value {
		t.Fatal("runtimeSecretMutationInputFromRequest() rejected an OpenAPI-valid multibyte value")
	}
}

func TestRuntimeSecretMutationEmptySetKeepsExistingValue(t *testing.T) {
	ctx, _ := testRuntimeSecretContext()
	input, ok := runtimeapi.RuntimeSecretMutationInputFromRequest(ctx, runtimeapi.RuntimeSecretMutationRequest{Items: []runtimeapi.RuntimeSecretMutationRequestItem{{
		Key: "TOKEN", ValueMode: runtimeapi.RuntimeEnvironmentValueModeSecret, Operation: "set", Value: "",
	}}})
	if !ok {
		t.Fatal("empty set request should be accepted as retain-existing")
	}
	prepared, err := runtimeapi.PrepareRuntimeSecretMutation(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.Values) != 0 || len(prepared.ConfiguredKeys) != 0 {
		t.Fatalf("empty set prepared mutation = %#v, want no-op", prepared)
	}
}

func TestValidateDeploymentTargetRuntimeSecretMutationRejectsConflictingFields(t *testing.T) {
	ctx, recorder := testRuntimeSecretContext()
	input := runtimeapi.RuntimeSecretMutationInput{
		Values:   map[string]string{"TOKEN": "value"},
		Generate: map[string]runtimeapi.RuntimeSecretGeneration{"TOKEN": {Length: 32, Encoding: "base64"}},
	}
	if runtimeapi.ValidateRuntimeSecretMutation(ctx, &input) {
		t.Fatal("validateDeploymentTargetRuntimeSecretMutation() = true, want false")
	}
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestRuntimeSecretKeysExposeOnlyConfiguredBuildEnvironmentKeys(t *testing.T) {
	keys := runtimeapi.RuntimeSecretKeys(`{"TOKEN":"secret-id:token","EMPTY":"","bad.key":"secret-id:bad","PASSWORD":"secret:v1:password"}`)
	if got, want := len(keys), 2; got != want {
		t.Fatalf("runtimeSecretKeys() returned %d keys, want %d: %#v", got, want, keys)
	}
	if keys[0] != "PASSWORD" || keys[1] != "TOKEN" {
		t.Fatalf("runtimeSecretKeys() = %#v, want sorted configured keys", keys)
	}
}

func TestRuntimeSecretResponsesSetNoStoreHeaders(t *testing.T) {
	ctx, recorder := testRuntimeSecretContext()
	runtimeapi.SetRuntimeSecretNoStoreHeaders(ctx)
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

func TestNormalizeEnvironmentVariablesSplitsUnifiedTable(t *testing.T) {
	ctx, _ := testRuntimeSecretContext()
	public, secrets, ok := runtimeapi.NormalizeEnvironmentVariables(ctx, []runtimeapi.RuntimeEnvironmentVariableInput{
		{Key: "LOG_LEVEL", ValueMode: runtimeapi.RuntimeEnvironmentValueModePublic, Value: "info"},
		{Key: "TOKEN", ValueMode: runtimeapi.RuntimeEnvironmentValueModePublic, Value: "now-public"},
		{Key: "TOKEN", ValueMode: runtimeapi.RuntimeEnvironmentValueModeSecret, SecretOperation: "clear"},
		{Key: "PASSWORD", ValueMode: runtimeapi.RuntimeEnvironmentValueModeSecret, SecretOperation: "set", Value: "secret"},
		{Key: "SESSION_KEY", ValueMode: runtimeapi.RuntimeEnvironmentValueModeSecret, SecretOperation: "generate"},
	})
	if !ok {
		t.Fatal("NormalizeEnvironmentVariables() rejected a valid unified table")
	}
	if public["LOG_LEVEL"] != "info" || public["TOKEN"] != "now-public" || len(public) != 2 {
		t.Fatalf("public = %#v", public)
	}
	if secrets.Values["PASSWORD"] != "secret" || len(secrets.Clear) != 1 || secrets.Clear[0] != "TOKEN" {
		t.Fatalf("secret mutation = %#v", secrets)
	}
	if generation, exists := secrets.Generate["SESSION_KEY"]; !exists || generation.Length != 32 || generation.Encoding != "base64" {
		t.Fatalf("generate = %#v, want defaulted generation", secrets.Generate)
	}
}

func TestNormalizeEnvironmentVariablesRejectsInvalidUnifiedItems(t *testing.T) {
	tests := map[string]struct {
		items []runtimeapi.RuntimeEnvironmentVariableInput
		code  string
	}{
		"public and secret set share a name": {
			items: []runtimeapi.RuntimeEnvironmentVariableInput{
				{Key: "TOKEN", ValueMode: runtimeapi.RuntimeEnvironmentValueModePublic, Value: "a"},
				{Key: "TOKEN", ValueMode: runtimeapi.RuntimeEnvironmentValueModeSecret, SecretOperation: "set", Value: "b"},
			},
			code: "deployment.runtime_environment_key_conflict",
		},
		"secret set without value": {
			items: []runtimeapi.RuntimeEnvironmentVariableInput{{Key: "TOKEN", ValueMode: runtimeapi.RuntimeEnvironmentValueModeSecret, SecretOperation: "set"}},
			code:  "deployment.secret_value_required",
		},
		"secret without operation": {
			items: []runtimeapi.RuntimeEnvironmentVariableInput{{Key: "TOKEN", ValueMode: runtimeapi.RuntimeEnvironmentValueModeSecret}},
			code:  "deployment.secret_operation_invalid",
		},
		"public item with secret operation": {
			items: []runtimeapi.RuntimeEnvironmentVariableInput{{Key: "TOKEN", ValueMode: runtimeapi.RuntimeEnvironmentValueModePublic, SecretOperation: "clear"}},
			code:  "deployment.runtime_environment_value_mode_invalid",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctx, recorder := testRuntimeSecretContext()
			if _, _, ok := runtimeapi.NormalizeEnvironmentVariables(ctx, test.items); ok {
				t.Fatal("NormalizeEnvironmentVariables() accepted an invalid item")
			}
			if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), test.code) {
				t.Fatalf("response = %d %s, want %s", recorder.Code, recorder.Body.String(), test.code)
			}
		})
	}
}
