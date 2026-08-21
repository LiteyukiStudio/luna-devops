package aitool

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/LiteyukiStudio/devops/internal/authz"
	"github.com/LiteyukiStudio/devops/openapi"
	"sigs.k8s.io/yaml"
)

type OpenAPIOperation struct {
	OperationID      string             `json:"operationId"`
	Name             string             `json:"name"`
	Summary          string             `json:"summary"`
	Category         string             `json:"category"`
	Tags             []string           `json:"tags"`
	Aliases          OperationAliases   `json:"aliases"`
	RequiresApproval bool               `json:"requiresApproval"`
	Idempotent       bool               `json:"idempotent"`
	Method           string             `json:"method"`
	Path             string             `json:"path"`
	RequiredScopes   []string           `json:"requiredScopes"`
	InputSchema      map[string]any     `json:"inputSchema"`
	OutputSchema     map[string]any     `json:"outputSchema"`
	SensitivePaths   []string           `json:"sensitivePaths,omitempty"`
	Parameters       []OpenAPIParameter `json:"parameters,omitempty"`
	RequestBody      bool               `json:"requestBody"`
	RequestRequired  bool               `json:"requestRequired"`
	RequestType      string             `json:"requestType,omitempty"`
}

type OperationAliases struct {
	ZH []string `json:"zh"`
	EN []string `json:"en"`
}

type OpenAPIParameter struct {
	InputName string `json:"inputName"`
	WireName  string `json:"wireName"`
	In        string `json:"in"`
	Required  bool   `json:"required"`
}

type openAPIDocument struct {
	Paths      map[string]map[string]any `json:"paths"`
	Components struct {
		Parameters    map[string]any `json:"parameters"`
		RequestBodies map[string]any `json:"requestBodies"`
		Schemas       map[string]any `json:"schemas"`
		Responses     map[string]any `json:"responses"`
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
	summary := strings.Join(strings.Fields(stringValue(raw["summary"])), " ")
	description := strings.Join(strings.Fields(stringValue(raw["description"])), " ")
	if summary == "" {
		summary = description
	}
	if summary == "" {
		summary = operationID
	}
	extension := mapValue(raw["x-luna-agent"])
	requiresApproval := operationRequiresApproval(method, mapValue(raw["x-luna-cli"]), extension)
	aliases := operationAliases(operationID, tags, extension)
	return OpenAPIOperation{
		OperationID:      operationID,
		Name:             operationID,
		Summary:          summary,
		Category:         category,
		Tags:             tags,
		Aliases:          aliases,
		RequiresApproval: requiresApproval,
		Idempotent:       strings.EqualFold(method, http.MethodGet) || boolValue(mapValue(raw["x-luna-cli"])["idempotent"]),
		Method:           strings.ToUpper(method),
		Path:             path,
		RequiredScopes:   scopes,
		InputSchema:      inputSchema,
		OutputSchema:     preferredOutputSchema(document, raw),
		SensitivePaths:   sensitivePaths,
		Parameters:       parameters,
		RequestBody:      len(requestSchema) > 0,
		RequestRequired:  requestRequired,
		RequestType:      requestType,
	}, nil
}

func preferredOutputSchema(document openAPIDocument, operation map[string]any) map[string]any {
	responses := mapValue(operation["responses"])
	codes := make([]int, 0, len(responses))
	for rawCode := range responses {
		code, err := strconv.Atoi(rawCode)
		if err == nil && code >= 200 && code < 300 {
			codes = append(codes, code)
		}
	}
	sort.Ints(codes)
	for _, code := range codes {
		response := resolveOpenAPIResponse(document, responses[strconv.Itoa(code)])
		content := mapValue(response["content"])
		media := mapValue(content["application/json"])
		if schema := mapValue(media["schema"]); len(schema) > 0 {
			return normalizeOpenAPISchema(document, schema, 0)
		}
	}
	return map[string]any{"type": "object"}
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
	if strings.HasSuffix(path, "/runtime-secrets/reveal") || (strings.HasSuffix(path, "/runtime-secrets") && method != "put") {
		return false
	}
	if method == "get" && (strings.HasSuffix(path, "/start") || strings.Contains(path, "/callback")) {
		return false
	}
	_, denied := agentDisabledOperations[operationID]
	return !denied
}

// agentDisabledOperations is the one explicit deny list for otherwise regular
// JSON platform operations. Every entry carries a reviewable reason; OpenAPI no
// longer needs a second per-operation `allowed` admission flag.
var agentDisabledOperations = map[string]string{
	"getPublicConfigs":                 "public bootstrap protocol",
	"getBootstrapStatus":               "public bootstrap protocol",
	"initializeAdmin":                  "initial trust bootstrap",
	"login":                            "interactive authentication protocol",
	"resumeLogin":                      "interactive authentication protocol",
	"logout":                           "interactive authentication protocol",
	"requestEmailRegistrationCode":     "out-of-band authentication protocol",
	"completeEmailRegistration":        "out-of-band authentication protocol",
	"startOIDC":                        "browser redirect protocol",
	"completeOIDC":                     "browser callback protocol",
	"startGitOAuth":                    "browser redirect protocol",
	"completeGitOAuth":                 "browser callback protocol",
	"receiveGitWebhook":                "webhook receiver",
	"startOAuthDeviceAuthorization":    "interactive OAuth protocol",
	"getOAuthDeviceVerification":       "interactive OAuth protocol",
	"decideOAuthDeviceVerification":    "interactive OAuth protocol",
	"exchangeOAuthToken":               "token protocol",
	"revokeOAuthToken":                 "token protocol",
	"getOAuthAuthorizationRequest":     "interactive OAuth protocol",
	"decideOAuthAuthorization":         "interactive OAuth protocol",
	"createAccessToken":                "credential material must be created directly by the user",
	"rotateOAuthApplicationSecret":     "credential material must be rotated directly by the user",
	"updateMyPassword":                 "credential material must be changed directly by the user",
	"createGatewayTrafficProbeHello":   "metering ingestion protocol",
	"createGatewayTrafficUsage":        "metering ingestion protocol",
	"createExternalBillingTransaction": "external billing ingestion protocol",
	"retryProjectVolumeOperation":      "effective permission depends on the failed operation being retried",
	"retryVolumeTransfer":              "effective permission depends on the original transfer direction",
}

var agentTagAliasesZH = map[string][]string{
	"Applications": {"应用", "应用服务"},
	"Builds":       {"构建", "构建任务"},
	"Deployments":  {"部署", "部署目标"},
	"Gateway":      {"网关", "域名", "路由"},
	"Projects":     {"项目空间", "项目"},
	"Registries":   {"镜像仓库", "镜像"},
	"Releases":     {"发布", "版本"},
	"Runtime":      {"运行时", "集群"},
	"Volumes":      {"项目数据卷", "数据卷", "持久卷", "存储卷"},
}

func operationAliases(operationID string, tags []string, extension map[string]any) OperationAliases {
	aliasesRaw := mapValue(extension["aliases"])
	zh := append([]string(nil), stringArray(aliasesRaw["zh"])...)
	en := append([]string(nil), stringArray(aliasesRaw["en"])...)
	for _, tag := range tags {
		en = append(en, tag)
		zh = append(zh, agentTagAliasesZH[tag]...)
	}
	en = append(en, operationID, strings.Join(splitOperationID(operationID), " "))
	return OperationAliases{ZH: uniqueStrings(zh), EN: uniqueStrings(en)}
}

func splitOperationID(value string) []string {
	parts := make([]string, 0, 6)
	start := 0
	runes := []rune(value)
	for index := 1; index < len(runes); index++ {
		if runes[index] >= 'A' && runes[index] <= 'Z' {
			parts = append(parts, strings.ToLower(string(runes[start:index])))
			start = index
		}
	}
	if start < len(runes) {
		parts = append(parts, strings.ToLower(string(runes[start:])))
	}
	return parts
}

func operationRequiresApproval(method string, cli, agent map[string]any) bool {
	if value, ok := agent["requiresApproval"].(bool); ok {
		return value
	}
	switch strings.ToLower(stringValue(cli["risk"])) {
	case "high", "critical", "destructive", "sensitive":
		return true
	}
	return strings.EqualFold(method, http.MethodDelete)
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

func resolveOpenAPIResponse(document openAPIDocument, raw any) map[string]any {
	value := mapValue(raw)
	const responsePrefix = "#/components/responses/"
	if ref := stringValue(value["$ref"]); strings.HasPrefix(ref, responsePrefix) {
		return mapValue(document.Components.Responses[strings.TrimPrefix(ref, responsePrefix)])
	}
	return value
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
	for _, key := range []string{"oneOf", "anyOf", "allOf"} {
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
