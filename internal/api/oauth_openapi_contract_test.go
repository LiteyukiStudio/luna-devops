package api

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type oauthTransportContractMode int

const (
	oauthTransportInput oauthTransportContractMode = iota
	oauthTransportOutput
)

func TestOAuthOpenAPITracksRuntimeTransportDTOs(t *testing.T) {
	document := readOpenAPIDocument(t, filepath.Join(apiRepositoryRoot(t), "openapi", "openapi.yaml"))
	tests := []struct {
		name        string
		path        string
		method      string
		status      string
		request     bool
		contentType string
		runtimeType reflect.Type
		mode        oauthTransportContractMode
		fieldTag    string
	}{
		{name: "metadata response", path: "/.well-known/oauth-authorization-server", method: "get", status: "200", runtimeType: reflect.TypeOf(oauthAuthorizationServerMetadataResponse{}), mode: oauthTransportOutput},
		{name: "device authorization input", path: "/api/v1/oauth/device/authorization", method: "post", request: true, contentType: "application/x-www-form-urlencoded", runtimeType: reflect.TypeOf(oauthDeviceAuthorizationInput{}), mode: oauthTransportInput, fieldTag: "form"},
		{name: "device authorization response", path: "/api/v1/oauth/device/authorization", method: "post", status: "200", runtimeType: reflect.TypeOf(oauthDeviceAuthorizationResponse{}), mode: oauthTransportOutput},
		{name: "device verification response", path: "/api/v1/oauth/device/verification", method: "get", status: "200", runtimeType: reflect.TypeOf(oauthDeviceVerificationResponse{}), mode: oauthTransportOutput},
		{name: "device decision input", path: "/api/v1/oauth/device/verification", method: "post", request: true, contentType: "application/json", runtimeType: reflect.TypeOf(oauthDeviceVerificationInput{}), mode: oauthTransportInput},
		{name: "device decision response", path: "/api/v1/oauth/device/verification", method: "post", status: "200", runtimeType: reflect.TypeOf(oauthDeviceVerificationResult{}), mode: oauthTransportOutput},
		{name: "token input", path: "/api/v1/oauth/token", method: "post", request: true, contentType: "application/x-www-form-urlencoded", runtimeType: reflect.TypeOf(oauthTokenInput{}), mode: oauthTransportInput, fieldTag: "form"},
		{name: "token response", path: "/api/v1/oauth/token", method: "post", status: "200", runtimeType: reflect.TypeOf(oauthTokenResponse{}), mode: oauthTransportOutput},
		{name: "token revocation input", path: "/api/v1/oauth/revoke", method: "post", request: true, contentType: "application/x-www-form-urlencoded", runtimeType: reflect.TypeOf(oauthTokenRevocationInput{}), mode: oauthTransportInput, fieldTag: "form"},
		{name: "application page", path: "/api/v1/oauth/applications", method: "get", status: "200", runtimeType: reflect.TypeOf(paginatedResponseBody[oauthApplicationResponse]{}), mode: oauthTransportOutput},
		{name: "create application input", path: "/api/v1/oauth/applications", method: "post", request: true, contentType: "application/json", runtimeType: reflect.TypeOf(oauthApplicationInput{}), mode: oauthTransportInput},
		{name: "create application response", path: "/api/v1/oauth/applications", method: "post", status: "201", runtimeType: reflect.TypeOf(oauthApplicationSecretResponse{}), mode: oauthTransportOutput},
		{name: "update application input", path: "/api/v1/oauth/applications/{applicationId}", method: "put", request: true, contentType: "application/json", runtimeType: reflect.TypeOf(oauthApplicationInput{}), mode: oauthTransportInput},
		{name: "update application response", path: "/api/v1/oauth/applications/{applicationId}", method: "put", status: "200", runtimeType: reflect.TypeOf(oauthApplicationResponse{}), mode: oauthTransportOutput},
		{name: "rotate application response", path: "/api/v1/oauth/applications/{applicationId}/rotate-secret", method: "post", status: "200", runtimeType: reflect.TypeOf(oauthApplicationSecretResponse{}), mode: oauthTransportOutput},
		{name: "grant page", path: "/api/v1/oauth/grants", method: "get", status: "200", runtimeType: reflect.TypeOf(paginatedResponseBody[oauthGrantResponse]{}), mode: oauthTransportOutput},
		{name: "authorization request", path: "/api/v1/oauth/authorize", method: "get", status: "200", runtimeType: reflect.TypeOf(oauthAuthorizationRequest{}), mode: oauthTransportOutput},
		{name: "authorization decision input", path: "/api/v1/oauth/authorize", method: "post", request: true, contentType: "application/json", runtimeType: reflect.TypeOf(oauthAuthorizationDecisionInput{}), mode: oauthTransportInput},
		{name: "authorization decision response", path: "/api/v1/oauth/authorize", method: "post", status: "200", runtimeType: reflect.TypeOf(oauthAuthorizationDecisionResponse{}), mode: oauthTransportOutput},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			operation := oauthOpenAPIOperation(t, document, test.path, test.method)
			var schema map[string]any
			if test.request {
				schema = oauthOpenAPIRequestSchema(t, operation, test.contentType)
			} else {
				schema = oauthOpenAPIResponseSchema(t, operation, test.status)
			}
			fieldTag := test.fieldTag
			if fieldTag == "" {
				fieldTag = "json"
			}
			oauthAssertRuntimeSchema(t, document, schema, test.runtimeType, test.mode, fieldTag)
		})
	}

	oauthAssertRuntimeSchema(
		t,
		document,
		oauthOpenAPIComponentSchema(t, document, "OAuthProtocolError"),
		reflect.TypeOf(oauthProtocolErrorResponse{}),
		oauthTransportOutput,
		"json",
	)
}

func TestOAuthRevocationOpenAPIDescribesRuntimeFamilyBoundary(t *testing.T) {
	handlerPath := filepath.Join(apiRepositoryRoot(t), "internal", "api", "identityapi", "oauth_authorization_handlers.go")
	if !oauthRuntimeFunctionCalls(t, handlerPath, "RevokeOAuthToken", "revokeOAuthTokenFamily") {
		t.Fatal("RevokeOAuthToken no longer delegates to revokeOAuthTokenFamily")
	}

	document := readOpenAPIDocument(t, filepath.Join(apiRepositoryRoot(t), "openapi", "openapi.yaml"))
	operation := oauthOpenAPIOperation(t, document, "/api/v1/oauth/revoke", "post")
	response := operation["responses"].(map[string]any)["200"].(map[string]any)
	for field, text := range map[string]string{
		"summary":     operation["summary"].(string),
		"description": operation["description"].(string),
		"response":    response["description"].(string),
	} {
		if !strings.Contains(strings.ToLower(text), "family") {
			t.Fatalf("OAuth revoke %s does not describe the runtime token-family boundary: %q", field, text)
		}
	}
}

func oauthOpenAPIOperation(t *testing.T, document map[string]any, path, method string) map[string]any {
	t.Helper()
	paths, ok := document["paths"].(map[string]any)
	if !ok {
		t.Fatal("OpenAPI document has no paths object")
	}
	pathItem, ok := paths[path].(map[string]any)
	if !ok {
		t.Fatalf("OpenAPI path %s is missing", path)
	}
	operation, ok := pathItem[method].(map[string]any)
	if !ok {
		t.Fatalf("OpenAPI operation %s %s is missing", strings.ToUpper(method), path)
	}
	return operation
}

func oauthOpenAPIRequestSchema(t *testing.T, operation map[string]any, contentType string) map[string]any {
	t.Helper()
	requestBody, ok := operation["requestBody"].(map[string]any)
	if !ok {
		t.Fatal("OpenAPI operation has no requestBody")
	}
	content, ok := requestBody["content"].(map[string]any)
	if !ok {
		t.Fatal("OpenAPI requestBody has no content")
	}
	mediaType, ok := content[contentType].(map[string]any)
	if !ok {
		t.Fatalf("OpenAPI requestBody has no %s content", contentType)
	}
	schema, ok := mediaType["schema"].(map[string]any)
	if !ok {
		t.Fatal("OpenAPI requestBody content has no schema")
	}
	return schema
}

func oauthOpenAPIResponseSchema(t *testing.T, operation map[string]any, status string) map[string]any {
	t.Helper()
	responses, ok := operation["responses"].(map[string]any)
	if !ok {
		t.Fatal("OpenAPI operation has no responses")
	}
	response, ok := responses[status].(map[string]any)
	if !ok {
		t.Fatalf("OpenAPI operation has no %s response", status)
	}
	content, ok := response["content"].(map[string]any)
	if !ok {
		t.Fatalf("OpenAPI response %s has no content", status)
	}
	mediaType, ok := content["application/json"].(map[string]any)
	if !ok {
		t.Fatalf("OpenAPI response %s has no application/json content", status)
	}
	schema, ok := mediaType["schema"].(map[string]any)
	if !ok {
		t.Fatalf("OpenAPI response %s has no schema", status)
	}
	return schema
}

func oauthOpenAPIComponentSchema(t *testing.T, document map[string]any, name string) map[string]any {
	t.Helper()
	components, ok := document["components"].(map[string]any)
	if !ok {
		t.Fatal("OpenAPI document has no components object")
	}
	schemas, ok := components["schemas"].(map[string]any)
	if !ok {
		t.Fatal("OpenAPI document has no component schemas")
	}
	schema, ok := schemas[name].(map[string]any)
	if !ok {
		t.Fatalf("OpenAPI component schema %s is missing", name)
	}
	return schema
}

func oauthResolveOpenAPISchema(t *testing.T, document, schema map[string]any) map[string]any {
	t.Helper()
	reference, _ := schema["$ref"].(string)
	if reference == "" {
		return schema
	}
	const prefix = "#/components/schemas/"
	if !strings.HasPrefix(reference, prefix) {
		t.Fatalf("unsupported OpenAPI schema reference %q", reference)
	}
	return oauthOpenAPIComponentSchema(t, document, strings.TrimPrefix(reference, prefix))
}

func oauthAssertRuntimeSchema(
	t *testing.T,
	document map[string]any,
	schema map[string]any,
	runtimeType reflect.Type,
	mode oauthTransportContractMode,
	fieldTag string,
) {
	t.Helper()
	schema = oauthResolveOpenAPISchema(t, document, schema)
	for runtimeType.Kind() == reflect.Pointer {
		runtimeType = runtimeType.Elem()
	}
	if runtimeType.Kind() != reflect.Struct || runtimeType == reflect.TypeOf(time.Time{}) {
		t.Fatalf("runtime transport contract must be a struct, got %s", runtimeType)
	}
	if schema["type"] != "object" {
		t.Fatalf("OpenAPI schema type = %#v, runtime JSON type = object", schema["type"])
	}
	if mode == oauthTransportOutput && schema["additionalProperties"] != false {
		t.Fatalf("OpenAPI output schema for %s must reject undeclared properties", runtimeType)
	}

	runtimeProperties := oauthRuntimeTransportFields(t, runtimeType, fieldTag)
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("OpenAPI schema has no properties")
	}
	if openAPIFields := oauthSortedMapKeys(properties); !reflect.DeepEqual(openAPIFields, oauthSortedMapKeys(runtimeProperties)) {
		t.Fatalf("OpenAPI properties = %#v, runtime JSON properties = %#v", openAPIFields, oauthSortedMapKeys(runtimeProperties))
	}

	runtimeRequired := oauthRuntimeRequiredFields(t, runtimeType, mode, fieldTag)
	openAPIRequired, ok := schemaStringList(schema["required"])
	if !ok {
		t.Fatalf("OpenAPI required fields = %#v", schema["required"])
	}
	sort.Strings(openAPIRequired)
	if !reflect.DeepEqual(openAPIRequired, runtimeRequired) {
		t.Fatalf("OpenAPI required fields = %#v, runtime required fields = %#v", openAPIRequired, runtimeRequired)
	}

	required := make(map[string]bool, len(runtimeRequired))
	for _, name := range runtimeRequired {
		required[name] = true
	}
	for name, runtimeField := range runtimeProperties {
		property, ok := properties[name].(map[string]any)
		if !ok {
			t.Fatalf("OpenAPI property %s is missing", name)
		}
		if mode == oauthTransportOutput && oauthResolveOpenAPISchema(t, document, property)["writeOnly"] == true {
			t.Fatalf("OpenAPI output property %s.%s must not be writeOnly", runtimeType, name)
		}
		oauthAssertRuntimeTypeSchema(t, document, property, runtimeField.Type, mode, required[name], fieldTag)
	}
	if mode == oauthTransportInput && fieldTag == "form" {
		oauthAssertConditionalFormRequirements(t, schema, runtimeType, runtimeProperties)
	}
}

func oauthRuntimeTransportFields(t *testing.T, runtimeType reflect.Type, fieldTag string) map[string]reflect.StructField {
	t.Helper()
	fields := make(map[string]reflect.StructField)
	for index := 0; index < runtimeType.NumField(); index++ {
		field := runtimeType.Field(index)
		if field.PkgPath != "" {
			continue
		}
		if field.Anonymous {
			t.Fatalf("transport DTO %s embeds %s instead of declaring its JSON contract", runtimeType, field.Type)
		}
		parts := strings.Split(field.Tag.Get(fieldTag), ",")
		if len(parts) == 0 || parts[0] == "" || parts[0] == "-" {
			t.Fatalf("transport DTO field %s.%s has no explicit %s name", runtimeType, field.Name, fieldTag)
		}
		fields[parts[0]] = field
	}

	serializedFields := oauthSerializedTransportFields(t, runtimeType, fieldTag, fields)
	if !reflect.DeepEqual(serializedFields, oauthSortedMapKeys(fields)) {
		t.Fatalf("serialized %s properties = %#v, reflected DTO properties = %#v", fieldTag, serializedFields, oauthSortedMapKeys(fields))
	}
	return fields
}

func oauthSerializedTransportFields(
	t *testing.T,
	runtimeType reflect.Type,
	fieldTag string,
	fields map[string]reflect.StructField,
) []string {
	t.Helper()
	populated := oauthPopulatedJSONValue(t, runtimeType)
	if fieldTag == "json" {
		return oauthSortedMapKeys(oauthSerializedObjectFields(t, populated.Interface()))
	}
	if fieldTag != "form" {
		t.Fatalf("unsupported OAuth transport field tag %q", fieldTag)
	}

	values := make(url.Values, len(fields))
	for name, field := range fields {
		value := fmt.Sprint(populated.FieldByIndex(field.Index).Interface())
		for _, rule := range strings.Split(field.Tag.Get("binding"), ",") {
			if choicesText, ok := strings.CutPrefix(rule, "oneof="); ok {
				choices := strings.Fields(choicesText)
				if len(choices) > 0 {
					value = choices[0]
				}
			}
		}
		values.Set(name, value)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(values.Encode()))
	ctx.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	target := reflect.New(runtimeType)
	if err := ctx.ShouldBind(target.Interface()); err != nil {
		t.Fatalf("bind populated OAuth form DTO %s: %v", runtimeType, err)
	}
	for name, field := range fields {
		if got := fmt.Sprint(target.Elem().FieldByIndex(field.Index).Interface()); got != values.Get(name) {
			t.Fatalf("bound OAuth form property %s = %q, serialized value = %q", name, got, values.Get(name))
		}
	}
	return oauthSortedMapKeys(values)
}

func oauthRuntimeRequiredFields(t *testing.T, runtimeType reflect.Type, mode oauthTransportContractMode, fieldTag string) []string {
	t.Helper()
	if mode == oauthTransportOutput {
		return oauthSortedMapKeys(oauthSerializedObjectFields(t, reflect.Zero(runtimeType).Interface()))
	}

	required := make([]string, 0)
	for name, field := range oauthRuntimeTransportFields(t, runtimeType, fieldTag) {
		for _, rule := range strings.Split(field.Tag.Get("binding"), ",") {
			if rule == "required" {
				required = append(required, name)
				break
			}
		}
	}
	sort.Strings(required)
	return required
}

func oauthSerializedObjectFields(t *testing.T, value any) map[string]json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal runtime OAuth DTO: %v", err)
	}
	fields := make(map[string]json.RawMessage)
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("unmarshal runtime OAuth DTO: %v", err)
	}
	return fields
}

func oauthPopulatedJSONValue(t *testing.T, runtimeType reflect.Type) reflect.Value {
	t.Helper()
	if runtimeType == reflect.TypeOf(time.Time{}) {
		return reflect.ValueOf(time.Unix(1, 0).UTC())
	}
	switch runtimeType.Kind() {
	case reflect.Pointer:
		value := reflect.New(runtimeType.Elem())
		value.Elem().Set(oauthPopulatedJSONValue(t, runtimeType.Elem()))
		return value
	case reflect.Struct:
		value := reflect.New(runtimeType).Elem()
		for index := 0; index < runtimeType.NumField(); index++ {
			field := runtimeType.Field(index)
			if field.PkgPath == "" {
				value.Field(index).Set(oauthPopulatedJSONValue(t, field.Type))
			}
		}
		return value
	case reflect.Slice:
		value := reflect.MakeSlice(runtimeType, 1, 1)
		value.Index(0).Set(oauthPopulatedJSONValue(t, runtimeType.Elem()))
		return value
	case reflect.String:
		return reflect.ValueOf("value").Convert(runtimeType)
	case reflect.Bool:
		return reflect.ValueOf(true).Convert(runtimeType)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		value := reflect.New(runtimeType).Elem()
		value.SetInt(1)
		return value
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		value := reflect.New(runtimeType).Elem()
		value.SetUint(1)
		return value
	default:
		t.Fatalf("unsupported runtime OAuth DTO type %s", runtimeType)
		return reflect.Value{}
	}
}

func oauthAssertConditionalFormRequirements(
	t *testing.T,
	schema map[string]any,
	runtimeType reflect.Type,
	runtimeProperties map[string]reflect.StructField,
) {
	t.Helper()
	goNameToFormName := make(map[string]string, len(runtimeProperties))
	for formName, field := range runtimeProperties {
		goNameToFormName[field.Name] = formName
	}

	runtimeConditions := make(map[string][]string)
	for formName, field := range runtimeProperties {
		for _, rule := range strings.Split(field.Tag.Get("binding"), ",") {
			conditionText, ok := strings.CutPrefix(rule, "required_if=")
			if !ok {
				continue
			}
			conditionParts := strings.Fields(conditionText)
			if len(conditionParts) == 0 || len(conditionParts)%2 != 0 {
				t.Fatalf("transport DTO %s.%s has invalid binding rule %q", runtimeType, field.Name, rule)
			}
			for index := 0; index < len(conditionParts); index += 2 {
				conditionField, ok := goNameToFormName[conditionParts[index]]
				if !ok {
					t.Fatalf("transport DTO %s.%s references unknown binding field %q", runtimeType, field.Name, conditionParts[index])
				}
				key := conditionField + "\x00" + conditionParts[index+1]
				runtimeConditions[key] = append(runtimeConditions[key], formName)
			}
		}
	}
	for key := range runtimeConditions {
		sort.Strings(runtimeConditions[key])
	}

	openAPIConditions := make(map[string][]string)
	allOf, ok := schema["allOf"].([]any)
	if !ok && len(runtimeConditions) > 0 {
		t.Fatalf("OpenAPI form schema for %s has no conditional requirements", runtimeType)
	}
	for _, rawClause := range allOf {
		clause, ok := rawClause.(map[string]any)
		if !ok {
			t.Fatalf("OpenAPI form conditional clause = %#v", rawClause)
		}
		ifSchema, ok := clause["if"].(map[string]any)
		if !ok {
			t.Fatalf("OpenAPI form conditional clause has no if schema: %#v", clause)
		}
		conditionProperties, ok := ifSchema["properties"].(map[string]any)
		if !ok || len(conditionProperties) != 1 {
			t.Fatalf("OpenAPI form conditional properties = %#v, want exactly one property", ifSchema["properties"])
		}
		thenSchema, ok := clause["then"].(map[string]any)
		if !ok {
			t.Fatalf("OpenAPI form conditional clause has no then schema: %#v", clause)
		}
		requiredFields, ok := schemaStringList(thenSchema["required"])
		if !ok {
			t.Fatalf("OpenAPI form conditional required fields = %#v", thenSchema["required"])
		}
		sort.Strings(requiredFields)
		for conditionField, rawCondition := range conditionProperties {
			condition, ok := rawCondition.(map[string]any)
			if !ok {
				t.Fatalf("OpenAPI form condition %s = %#v", conditionField, rawCondition)
			}
			conditionValue, ok := condition["const"].(string)
			if !ok || conditionValue == "" {
				t.Fatalf("OpenAPI form condition %s const = %#v", conditionField, condition["const"])
			}
			openAPIConditions[conditionField+"\x00"+conditionValue] = requiredFields
		}
	}

	if !reflect.DeepEqual(openAPIConditions, runtimeConditions) {
		t.Fatalf("OpenAPI form conditional requirements = %#v, runtime requirements = %#v", openAPIConditions, runtimeConditions)
	}
}

func oauthAssertRuntimeTypeSchema(
	t *testing.T,
	document map[string]any,
	schema map[string]any,
	runtimeType reflect.Type,
	mode oauthTransportContractMode,
	required bool,
	fieldTag string,
) {
	t.Helper()
	pointer := runtimeType.Kind() == reflect.Pointer
	for runtimeType.Kind() == reflect.Pointer {
		runtimeType = runtimeType.Elem()
	}
	expectedType := oauthJSONTypeForGoType(t, runtimeType)
	openAPITypes := oauthOpenAPITypes(t, schema)
	if !openAPITypes[expectedType] {
		t.Fatalf("OpenAPI property types = %#v, runtime JSON type = %q", oauthSortedMapKeys(openAPITypes), expectedType)
	}
	expectedNullable := mode == oauthTransportOutput && pointer && required
	if openAPITypes["null"] != expectedNullable {
		t.Fatalf("OpenAPI property nullable = %t, runtime JSON nullable = %t", openAPITypes["null"], expectedNullable)
	}

	if runtimeType == reflect.TypeOf(time.Time{}) {
		if schema = oauthResolveOpenAPISchema(t, document, schema); schema["format"] != "date-time" {
			t.Fatalf("OpenAPI time property format = %#v, want date-time", schema["format"])
		}
		return
	}
	switch runtimeType.Kind() {
	case reflect.Struct:
		oauthAssertRuntimeSchema(t, document, schema, runtimeType, mode, fieldTag)
	case reflect.Slice:
		resolved := oauthResolveOpenAPISchema(t, document, schema)
		items, ok := resolved["items"].(map[string]any)
		if !ok {
			t.Fatal("OpenAPI array property has no item schema")
		}
		oauthAssertRuntimeTypeSchema(t, document, items, runtimeType.Elem(), mode, true, fieldTag)
	}
}

func oauthJSONTypeForGoType(t *testing.T, runtimeType reflect.Type) string {
	t.Helper()
	if runtimeType == reflect.TypeOf(time.Time{}) {
		return "string"
	}
	switch runtimeType.Kind() {
	case reflect.Bool:
		return "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "integer"
	case reflect.String:
		return "string"
	case reflect.Slice:
		return "array"
	case reflect.Struct:
		return "object"
	default:
		t.Fatalf("unsupported runtime OAuth DTO type %s", runtimeType)
		return ""
	}
}

func oauthOpenAPITypes(t *testing.T, schema map[string]any) map[string]bool {
	t.Helper()
	types := make(map[string]bool)
	switch value := schema["type"].(type) {
	case string:
		types[value] = true
	case []any:
		for _, item := range value {
			name, ok := item.(string)
			if !ok {
				t.Fatalf("OpenAPI property type = %#v", schema["type"])
			}
			types[name] = true
		}
	case nil:
		if reference, ok := schema["$ref"].(string); !ok || reference == "" {
			t.Fatalf("OpenAPI property type = %#v", schema["type"])
		}
	default:
		t.Fatalf("OpenAPI property type = %#v", schema["type"])
	}
	if schema["nullable"] == true {
		types["null"] = true
	}
	if len(types) == 0 {
		types["object"] = true
	}
	return types
}

func oauthSortedMapKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func oauthRuntimeFunctionCalls(t *testing.T, filename, functionName, calleeName string) bool {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), filename, nil, 0)
	if err != nil {
		t.Fatalf("parse runtime handler: %v", err)
	}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != functionName || function.Body == nil {
			continue
		}
		found := false
		ast.Inspect(function.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			identifier, ok := call.Fun.(*ast.Ident)
			if ok && identifier.Name == calleeName {
				found = true
				return false
			}
			return true
		})
		return found
	}
	return false
}
