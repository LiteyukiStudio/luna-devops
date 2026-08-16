package aitool

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/openapi"
	"sigs.k8s.io/yaml"
)

type OpenAPIOperation struct {
	OperationID    string         `json:"operationId"`
	Method         string         `json:"method"`
	Path           string         `json:"path"`
	Category       string         `json:"category"`
	Description    string         `json:"description"`
	SearchHints    []string       `json:"searchHints,omitempty"`
	Risk           string         `json:"risk"`
	RequiredScopes []string       `json:"requiredScopes"`
	Approval       string         `json:"approval"`
	StepUpPurpose  string         `json:"stepUpPurpose,omitempty"`
	Idempotent     bool           `json:"idempotent"`
	TimeoutMS      int            `json:"timeoutMs"`
	InputSchema    map[string]any `json:"inputSchema"`
	SensitivePaths []string       `json:"sensitivePaths,omitempty"`
	MaxItems       int            `json:"maxItems,omitempty"`
	ResultVerifier string         `json:"resultVerifier,omitempty"`

	Parameters      []OpenAPIParameter `json:"-"`
	RequestBody     bool               `json:"-"`
	RequestRequired bool               `json:"-"`
	RequestType     string             `json:"-"`
}

type OpenAPIParameter struct {
	InputName string
	WireName  string
	In        string
	Required  bool
}

type openAPIDocument struct {
	Paths      map[string]map[string]any `json:"paths"`
	Components struct {
		Parameters    map[string]any `json:"parameters"`
		RequestBodies map[string]any `json:"requestBodies"`
		Schemas       map[string]any `json:"schemas"`
	} `json:"components"`
}

var (
	openAPICatalogOnce sync.Once
	openAPICatalog     []OpenAPIOperation
	openAPICatalogErr  error
)

func PlatformCatalog() ([]OpenAPIOperation, error) {
	openAPICatalogOnce.Do(func() {
		openAPICatalog, openAPICatalogErr = buildPlatformCatalog(openapi.SpecYAML)
	})
	if openAPICatalogErr != nil {
		return nil, openAPICatalogErr
	}
	return append([]OpenAPIOperation(nil), openAPICatalog...), nil
}

func PlatformOperation(operationID string) (OpenAPIOperation, bool) {
	operations, err := PlatformCatalog()
	if err != nil {
		return OpenAPIOperation{}, false
	}
	for _, operation := range operations {
		if operation.OperationID == operationID {
			return operation, true
		}
	}
	return OpenAPIOperation{}, false
}

func buildPlatformCatalog(source []byte) ([]OpenAPIOperation, error) {
	jsonSource, err := yaml.YAMLToJSON(source)
	if err != nil {
		return nil, fmt.Errorf("convert OpenAPI document: %w", err)
	}
	var document openAPIDocument
	if err := json.Unmarshal(jsonSource, &document); err != nil {
		return nil, fmt.Errorf("parse OpenAPI document: %w", err)
	}

	operations := make([]OpenAPIOperation, 0, len(document.Paths))
	seen := map[string]struct{}{}
	for path, pathItem := range document.Paths {
		for _, method := range []string{"get", "post", "put", "patch", "delete"} {
			raw, ok := pathItem[method].(map[string]any)
			if !ok {
				continue
			}
			operationID := stringValue(raw["operationId"])
			if operationID == "" || !agentEligibleOperation(path, method, operationID, raw) {
				continue
			}
			if _, exists := seen[operationID]; exists {
				return nil, fmt.Errorf("duplicate Agent operationId %q", operationID)
			}
			operation, err := catalogOperation(document, path, method, operationID, raw)
			if err != nil {
				return nil, err
			}
			if raw["requestBody"] != nil && !operation.RequestBody {
				continue
			}
			seen[operationID] = struct{}{}
			operations = append(operations, operation)
		}
	}
	sort.Slice(operations, func(i, j int) bool {
		return operations[i].OperationID < operations[j].OperationID
	})
	return operations, nil
}

func catalogOperation(document openAPIDocument, path, method, operationID string, raw map[string]any) (OpenAPIOperation, error) {
	parameters := make([]OpenAPIParameter, 0)
	properties := map[string]any{}
	required := make([]string, 0)
	for _, parameterRaw := range arrayValue(raw["parameters"]) {
		parameter := resolveOpenAPIObject(document, parameterRaw)
		wireName, location := stringValue(parameter["name"]), stringValue(parameter["in"])
		inputName := wireName
		if location == "header" {
			inputName = stringValue(parameter["x-luna-agent-input-name"])
		}
		if wireName == "" || inputName == "" || (location != "path" && location != "query" && location != "header") {
			continue
		}
		isRequired := boolValue(parameter["required"]) || location == "path"
		parameters = append(parameters, OpenAPIParameter{InputName: inputName, WireName: wireName, In: location, Required: isRequired})
		properties[inputName] = normalizeOpenAPISchema(document, mapValue(parameter["schema"]), 0)
		if isRequired {
			required = append(required, inputName)
		}
	}

	requestBody := resolveOpenAPIObject(document, raw["requestBody"])
	requestSchema, requestType := requestBodySchema(requestBody)
	requestRequired := boolValue(requestBody["required"])
	if len(requestSchema) > 0 {
		properties["body"] = normalizeOpenAPISchema(document, requestSchema, 0)
		if requestRequired {
			required = append(required, "body")
		}
	}

	tags := stringArray(raw["tags"])
	category := "platform"
	if len(tags) > 0 {
		category = strings.ToLower(tags[0])
	}
	extension := mapValue(raw["x-luna-cli"])
	risk := agentRisk(method, stringValue(extension["risk"]), operationID)
	approval := "never"
	if risk == "sensitive" || risk == "destructive" {
		approval = "always"
	}
	scope := authz.RequiredAccessTokenScope(openAPIPathToGin(path), strings.ToUpper(method))
	if scope == "" || scope == string(authz.ActionSystemUnmapped) {
		scope = fallbackAgentScope(category, method)
	}
	scopes := []string{}
	if scope != "" && scope != string(authz.ActionSystemUnmapped) {
		scopes = append(scopes, scope)
	}
	inputSchema := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             uniqueStrings(required),
		"additionalProperties": false,
	}
	sensitivePaths := schemaSensitivePaths(inputSchema, "")
	description := fmt.Sprintf(
		"调用 Luna DevOps 的 %s 平台操作（%s %s）。参数与结果遵循平台 OpenAPI。",
		operationID,
		strings.ToUpper(method),
		path,
	)
	searchHints := compactSearchHints(stringValue(raw["summary"]), stringValue(raw["description"]))
	return OpenAPIOperation{
		OperationID: operationID, Method: strings.ToUpper(method), Path: path,
		Category: category, Description: description, SearchHints: searchHints, Risk: risk,
		RequiredScopes: scopes, Approval: approval,
		StepUpPurpose: stringValue(extension["mfaPurpose"]),
		Idempotent:    method == "get" || method == "put" || method == "delete",
		TimeoutMS:     30000, InputSchema: inputSchema, SensitivePaths: sensitivePaths,
		MaxItems: 100, ResultVerifier: resultVerifierFor(operationID, method),
		Parameters: parameters, RequestBody: len(requestSchema) > 0,
		RequestRequired: requestRequired, RequestType: requestType,
	}, nil
}

// compactSearchHints keeps OpenAPI prose out of the model-visible description while
// retaining concise semantic terms for the Agent-side tool retriever.
func compactSearchHints(values ...string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.Join(strings.Fields(value), " ")
		if value == "" {
			continue
		}
		characters := []rune(value)
		if len(characters) > 500 {
			value = string(characters[:500])
		}
		result = append(result, value)
	}
	return result
}

func agentEligibleOperation(path, method, operationID string, raw map[string]any) bool {
	if !strings.HasPrefix(path, "/api/v1/") {
		return false
	}
	tags := stringArray(raw["tags"])
	for _, tag := range tags {
		if tag == "Health" || tag == "AI Assistant" || tag == "AI Assistant Internal" {
			return false
		}
	}
	extension := mapValue(raw["x-luna-cli"])
	if value, exists := extension["agentAllowed"]; exists && !boolValue(value) {
		return false
	}
	switch stringValue(extension["classification"]) {
	case "protocol-adapter", "browser-callback", "webhook-receiver":
		return false
	}
	switch stringValue(extension["transport"]) {
	case "sse", "websocket", "download", "upload":
		return false
	}
	if boolValue(extension["streaming"]) || boolValue(extension["hidden"]) {
		return false
	}
	if strings.Contains(path, "/stream") || strings.Contains(path, "/terminal") {
		return false
	}
	if method == "get" && (strings.HasSuffix(path, "/start") || strings.Contains(path, "/callback")) {
		return false
	}
	_, denied := agentDeniedOperationIDs[operationID]
	return !denied
}

var agentDeniedOperationIDs = map[string]struct{}{
	"getPublicConfigs": {}, "getBootstrapStatus": {}, "initializeAdmin": {},
	"login": {}, "resumeLogin": {}, "logout": {},
	"requestEmailRegistrationCode": {}, "completeEmailRegistration": {},
	"enrollMFA": {}, "confirmMFA": {}, "verifyMFA": {},
	"regenerateMFARecoveryCodes": {}, "disableMFA": {},
	"startOIDC": {}, "completeOIDC": {},
	"startGitOAuth": {}, "completeGitOAuth": {}, "receiveGitWebhook": {},
	"startOAuthDeviceAuthorization": {}, "getOAuthDeviceVerification": {},
	"decideOAuthDeviceVerification": {}, "exchangeOAuthToken": {}, "revokeOAuthToken": {},
	"getOAuthAuthorizationRequest": {}, "decideOAuthAuthorization": {},
	"createAccessToken": {}, "rotateOAuthApplicationSecret": {},
	"updateMyPassword": {}, "createGatewayTrafficProbeHello": {},
	"createGatewayTrafficUsage": {}, "createExternalBillingTransaction": {},
}

func requestBodySchema(requestBody map[string]any) (map[string]any, string) {
	content := mapValue(requestBody["content"])
	for _, contentType := range []string{"application/json", "application/merge-patch+json"} {
		media := mapValue(content[contentType])
		if schema := mapValue(media["schema"]); len(schema) > 0 {
			return schema, contentType
		}
	}
	return nil, ""
}

func resolveOpenAPIObject(document openAPIDocument, raw any) map[string]any {
	value := mapValue(raw)
	ref := stringValue(value["$ref"])
	if ref == "" {
		return value
	}
	const parameterPrefix = "#/components/parameters/"
	const requestBodyPrefix = "#/components/requestBodies/"
	switch {
	case strings.HasPrefix(ref, parameterPrefix):
		return mapValue(document.Components.Parameters[strings.TrimPrefix(ref, parameterPrefix)])
	case strings.HasPrefix(ref, requestBodyPrefix):
		return mapValue(document.Components.RequestBodies[strings.TrimPrefix(ref, requestBodyPrefix)])
	default:
		return value
	}
}

func normalizeOpenAPISchema(document openAPIDocument, schema map[string]any, depth int) map[string]any {
	if depth > 8 || len(schema) == 0 {
		return map[string]any{"type": "object"}
	}
	if ref := stringValue(schema["$ref"]); strings.HasPrefix(ref, "#/components/schemas/") {
		schema = mapValue(document.Components.Schemas[strings.TrimPrefix(ref, "#/components/schemas/")])
	}
	result := map[string]any{}
	for _, key := range []string{
		"type", "format", "description", "pattern", "minimum", "maximum",
		"minLength", "maxLength", "minItems", "maxItems", "default",
		"writeOnly", "readOnly", "x-luna-sensitive",
	} {
		if value, ok := schema[key]; ok {
			result[key] = value
		}
	}
	if enum := arrayValue(schema["enum"]); len(enum) > 0 {
		result["enum"] = enum
	}
	if required := stringArray(schema["required"]); len(required) > 0 {
		result["required"] = required
	}
	if properties := mapValue(schema["properties"]); len(properties) > 0 {
		normalized := map[string]any{}
		for name, property := range properties {
			normalized[name] = normalizeOpenAPISchema(document, mapValue(property), depth+1)
		}
		result["properties"] = normalized
		result["additionalProperties"] = false
	}
	if additionalProperties, exists := schema["additionalProperties"]; exists {
		switch value := additionalProperties.(type) {
		case bool:
			result["additionalProperties"] = value
		case map[string]any:
			result["additionalProperties"] = normalizeOpenAPISchema(document, value, depth+1)
		}
	}
	if items := mapValue(schema["items"]); len(items) > 0 {
		result["items"] = normalizeOpenAPISchema(document, items, depth+1)
	}
	for _, key := range []string{"oneOf", "anyOf"} {
		if values := arrayValue(schema[key]); len(values) > 0 {
			normalized := make([]any, 0, len(values))
			for _, value := range values {
				normalized = append(normalized, normalizeOpenAPISchema(document, mapValue(value), depth+1))
			}
			result[key] = normalized
		}
	}
	if len(result) == 0 {
		result["type"] = "object"
	}
	return result
}

func schemaSensitivePaths(schema map[string]any, prefix string) []string {
	paths := make([]string, 0)
	if boolValue(schema["writeOnly"]) || boolValue(schema["x-luna-sensitive"]) {
		if prefix != "" {
			paths = append(paths, prefix)
		}
	}
	for name, property := range mapValue(schema["properties"]) {
		path := name
		if prefix != "" {
			path = prefix + "." + name
		}
		paths = append(paths, schemaSensitivePaths(mapValue(property), path)...)
	}
	if items := mapValue(schema["items"]); len(items) > 0 {
		path := prefix + ".*"
		if prefix == "" {
			path = "*"
		}
		paths = append(paths, schemaSensitivePaths(items, path)...)
	}
	return uniqueStrings(paths)
}

func agentRisk(method, declared, operationID string) string {
	switch declared {
	case "critical":
		return "destructive"
	case "high":
		return "sensitive"
	case "medium":
		return "write"
	case "low":
		if method == "get" {
			return "read"
		}
		return "write"
	}
	if method == "get" {
		return "read"
	}
	if method == "delete" || strings.HasPrefix(operationID, "delete") ||
		strings.HasPrefix(operationID, "revoke") || strings.HasPrefix(operationID, "cleanup") ||
		strings.Contains(strings.ToLower(operationID), "rollback") {
		return "destructive"
	}
	return "write"
}

func resultVerifierFor(operationID, method string) string {
	if strings.EqualFold(method, http.MethodGet) {
		return ""
	}
	return operationID + "_accepted"
}

func fallbackAgentScope(category, method string) string {
	read := strings.EqualFold(method, http.MethodGet)
	switch category {
	case "auth":
		return string(authz.ActionAuthManage)
	case "accesstokens", "oauthapplications":
		return string(authz.ActionTokenManage)
	case "users":
		if read {
			return string(authz.ActionUserRead)
		}
		return string(authz.ActionUserManage)
	case "dashboard":
		return string(authz.ActionDashboardRead)
	case "notifications", "events":
		if read {
			return string(authz.ActionEventRead)
		}
		return string(authz.ActionConfigWrite)
	case "builds":
		if read {
			return string(authz.ActionBuildRead)
		}
		return string(authz.ActionBuildTrigger)
	case "deployments", "releases":
		if read {
			return string(authz.ActionDeploymentRead)
		}
		return string(authz.ActionDeploymentUpdate)
	case "gateway":
		if read {
			return string(authz.ActionGatewayRead)
		}
		return string(authz.ActionGatewayManage)
	case "runtime":
		if read {
			return string(authz.ActionClusterRead)
		}
		return string(authz.ActionClusterManage)
	case "git":
		if read {
			return string(authz.ActionGitRead)
		}
		return string(authz.ActionGitWrite)
	case "registries":
		if read {
			return string(authz.ActionRegistryRead)
		}
		return string(authz.ActionRegistryWrite)
	case "billing":
		if read {
			return string(authz.ActionBillingRead)
		}
		return string(authz.ActionBillingAdjust)
	case "dataretention":
		if read {
			return string(authz.ActionDataRetentionRead)
		}
		return string(authz.ActionDataRetentionManage)
	case "apptemplates", "applications":
		if read {
			return string(authz.ActionApplicationRead)
		}
		return string(authz.ActionApplicationCreate)
	case "topology", "projects":
		if read {
			return string(authz.ActionProjectRead)
		}
		return string(authz.ActionProjectWrite)
	case "system", "configs":
		if read {
			return string(authz.ActionConfigRead)
		}
		return string(authz.ActionConfigWrite)
	default:
		return ""
	}
}

func openAPIPathToGin(path string) string {
	parts := strings.Split(path, "/")
	for index, part := range parts {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			parts[index] = ":" + strings.TrimSuffix(strings.TrimPrefix(part, "{"), "}")
		}
	}
	return strings.Join(parts, "/")
}

func mapValue(value any) map[string]any {
	result, _ := value.(map[string]any)
	return result
}

func arrayValue(value any) []any {
	result, _ := value.([]any)
	return result
}

func stringValue(value any) string {
	result, _ := value.(string)
	return strings.TrimSpace(result)
}

func boolValue(value any) bool {
	result, _ := value.(bool)
	return result
}

func stringArray(value any) []string {
	values := arrayValue(value)
	result := make([]string, 0, len(values))
	for _, item := range values {
		if text := stringValue(item); text != "" {
			result = append(result, text)
		}
	}
	return result
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
