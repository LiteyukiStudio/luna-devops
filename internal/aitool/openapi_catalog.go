package aitool

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/LiteyukiStudio/devops/internal/openapiscope"
	"github.com/LiteyukiStudio/devops/openapi"
	"sigs.k8s.io/yaml"
)

const (
	ApprovalTierL0 = "L0"
	ApprovalTierL1 = "L1"
	ApprovalTierL2 = "L2"
)

type OpenAPIOperation struct {
	OperationID      string                  `json:"operationId"`
	Name             string                  `json:"name"`
	Summary          string                  `json:"summary"`
	Category         string                  `json:"category"`
	Tags             []string                `json:"tags"`
	Aliases          OperationAliases        `json:"aliases"`
	Purpose          LocalizedText           `json:"purpose"`
	AvoidWhen        LocalizedText           `json:"avoidWhen"`
	Preconditions    LocalizedList           `json:"preconditions"`
	SuccessEvidence  LocalizedText           `json:"successEvidence"`
	ApprovalTier     string                  `json:"approvalTier"`
	RequiresApproval bool                    `json:"requiresApproval"`
	Idempotent       bool                    `json:"idempotent"`
	Method           string                  `json:"method"`
	Path             string                  `json:"path"`
	RequiredScopes   []string                `json:"requiredScopes"`
	InputSchema      map[string]any          `json:"inputSchema"`
	OutputSchema     map[string]any          `json:"outputSchema"`
	SensitivePaths   []string                `json:"sensitivePaths,omitempty"`
	Parameters       []OpenAPIParameter      `json:"parameters,omitempty"`
	RequestBody      bool                    `json:"requestBody"`
	RequestRequired  bool                    `json:"requestRequired"`
	RequestType      string                  `json:"requestType,omitempty"`
	Effect           string                  `json:"effect"`
	Verification     []OperationVerification `json:"verification,omitempty"`
	RetryOnUnknown   bool                    `json:"retryOnUnknown,omitempty"`
	VerificationOnly bool                    `json:"verificationOnly,omitempty"`
}

type OperationVerification struct {
	OperationID     string   `json:"operationId"`
	Family          string   `json:"family"`
	Expect          string   `json:"expect,omitempty"`
	WhenResultHas   string   `json:"whenResultHas,omitempty"`
	IDFrom          string   `json:"idFrom,omitempty"`
	ReadIDArgument  string   `json:"readIdArgument,omitempty"`
	Singleton       bool     `json:"singleton,omitempty"`
	Collection      bool     `json:"collection,omitempty"`
	TargetField     string   `json:"targetField,omitempty"`
	StatusField     string   `json:"statusField,omitempty"`
	PendingStatuses []string `json:"pendingStatuses,omitempty"`
	SuccessStatuses []string `json:"successStatuses,omitempty"`
	FailureStatuses []string `json:"failureStatuses,omitempty"`
}

type OperationAliases struct {
	ZH []string `json:"zh"`
	EN []string `json:"en"`
}

type LocalizedText struct {
	ZH string `json:"zh"`
	EN string `json:"en"`
}

type LocalizedList struct {
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
	scopeResolver, err := openapiscope.New(source)
	if err != nil {
		return nil, err
	}
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
			operation, err := catalogOperation(document, scopeResolver, path, method, operationID, pathItem, raw)
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

func catalogOperation(document openAPIDocument, scopeResolver *openapiscope.Resolver, path, method, operationID string, pathItem, raw map[string]any) (OpenAPIOperation, error) {
	parameters := make([]OpenAPIParameter, 0)
	properties := map[string]any{}
	required := make([]string, 0)
	for _, parameterRaw := range mergedOpenAPIParameters(document, pathItem["parameters"], raw["parameters"]) {
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
	cliExtension := mapValue(raw["x-luna-cli"])
	scopes, err := scopeResolver.RequiredScopes(path, method)
	if err != nil {
		return OpenAPIOperation{}, fmt.Errorf(
			"Agent operation %q must declare x-luna-cli.requiredScopes: %w",
			operationID,
			err,
		)
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
	effect, verification, err := operationExecutionSemantics(method, extension)
	if err != nil {
		return OpenAPIOperation{}, fmt.Errorf("Agent operation %q execution semantics: %w", operationID, err)
	}
	approvalTier, err := operationApprovalTier(operationID, method, effect, tags, scopes, sensitivePaths, cliExtension, extension)
	if err != nil {
		return OpenAPIOperation{}, fmt.Errorf("Agent operation %q approval tier: %w", operationID, err)
	}
	requiresApproval := approvalTier != ApprovalTierL0
	aliases := operationAliases(operationID, tags, extension)
	purpose := localizedText(extension["purpose"])
	if purpose.EN == "" {
		purpose.EN = summary
	}
	return OpenAPIOperation{
		OperationID:      operationID,
		Name:             operationID,
		Summary:          summary,
		Category:         category,
		Tags:             tags,
		Aliases:          aliases,
		Purpose:          purpose,
		AvoidWhen:        localizedText(extension["avoidWhen"]),
		Preconditions:    localizedList(extension["preconditions"]),
		SuccessEvidence:  localizedText(extension["successEvidence"]),
		ApprovalTier:     approvalTier,
		RequiresApproval: requiresApproval,
		Idempotent:       strings.EqualFold(method, http.MethodGet) || boolValue(cliExtension["idempotent"]),
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
		Effect:           effect,
		Verification:     verification,
		RetryOnUnknown:   boolValue(extension["retryOnUnknown"]),
		VerificationOnly: boolValue(extension["verificationOnly"]),
	}, nil
}

func operationExecutionSemantics(method string, extension map[string]any) (string, []OperationVerification, error) {
	effect := stringValue(extension["effect"])
	if effect == "" {
		effect = "write"
		if strings.EqualFold(method, http.MethodGet) {
			effect = "read"
		}
	}
	switch effect {
	case "discovery", "read", "write", "async", "execute", "interactive", "presentation":
	default:
		return "", nil, fmt.Errorf("unsupported effect %q", effect)
	}
	verification, err := operationVerifications(extension["verification"])
	if err != nil {
		return "", nil, err
	}
	if len(verification) == 0 {
		if effect == "async" {
			return "", nil, fmt.Errorf("async effect requires verification")
		}
		return effect, nil, nil
	}
	return effect, verification, nil
}

func operationVerifications(value any) ([]OperationVerification, error) {
	values := make([]any, 0)
	switch typed := value.(type) {
	case nil:
		return nil, nil
	case []any:
		values = typed
	case map[string]any:
		values = append(values, typed)
	default:
		return nil, fmt.Errorf("verification must be an object or array")
	}
	verifications := make([]OperationVerification, 0, len(values))
	for _, item := range values {
		raw := mapValue(item)
		verification := OperationVerification{
			OperationID:    stringValue(raw["operationId"]),
			Family:         stringValue(raw["family"]),
			Expect:         stringValue(raw["expect"]),
			WhenResultHas:  stringValue(raw["whenResultHas"]),
			IDFrom:         stringValue(raw["idFrom"]),
			ReadIDArgument: stringValue(raw["readIdArgument"]),
			Singleton:      boolValue(raw["singleton"]),
			Collection:     boolValue(raw["collection"]),
			TargetField:    stringValue(raw["targetField"]),
			StatusField:    stringValue(raw["statusField"]),
		}
		if values := stringArray(raw["pendingStatuses"]); len(values) > 0 {
			verification.PendingStatuses = values
		}
		if values := stringArray(raw["successStatuses"]); len(values) > 0 {
			verification.SuccessStatuses = values
		}
		if values := stringArray(raw["failureStatuses"]); len(values) > 0 {
			verification.FailureStatuses = values
		}
		if verification.OperationID == "" {
			return nil, fmt.Errorf("verification operationId is required")
		}
		switch verification.Family {
		case "build_run", "release", "gateway_route", "application", "deployment_target", "runtime_secrets", "runtime_config_secrets", "project_order", "billing_rate_rules", "resource":
		default:
			return nil, fmt.Errorf("unsupported verification family %q", verification.Family)
		}
		switch verification.Expect {
		case "", "present", "absent", "canceled":
		default:
			return nil, fmt.Errorf("unsupported verification expectation %q", verification.Expect)
		}
		if verification.Family == "resource" && verification.IDFrom == "" && !verification.Singleton {
			return nil, fmt.Errorf("resource verification idFrom is required")
		}
		verifications = append(verifications, verification)
	}
	return verifications, nil
}

func mergedOpenAPIParameters(document openAPIDocument, pathParameters, operationParameters any) []any {
	merged := make([]any, 0, len(arrayValue(pathParameters))+len(arrayValue(operationParameters)))
	indexByKey := map[string]int{}
	for _, source := range [][]any{arrayValue(pathParameters), arrayValue(operationParameters)} {
		for _, raw := range source {
			parameter := resolveOpenAPIObject(document, raw)
			key := stringValue(parameter["in"]) + "\x00" + stringValue(parameter["name"])
			if key == "\x00" {
				continue
			}
			if index, exists := indexByKey[key]; exists {
				merged[index] = raw
				continue
			}
			indexByKey[key] = len(merged)
			merged = append(merged, raw)
		}
	}
	return merged
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
	agentExtension := mapValue(raw["x-luna-agent"])
	// CLI exposure and the platform-internal Agent catalog are independent
	// boundaries. An operation with explicit Agent semantics can remain hidden
	// from external CLI users while still being discoverable by luna-agent.
	if len(agentExtension) == 0 {
		switch stringValue(extension["classification"]) {
		case "protocol-adapter", "browser-callback", "webhook-receiver":
			return false
		}
		if boolValue(extension["hidden"]) {
			return false
		}
	}
	switch stringValue(extension["transport"]) {
	case "sse", "websocket", "download", "upload":
		return false
	}
	if boolValue(extension["streaming"]) {
		return false
	}
	if strings.Contains(path, "/stream") || strings.Contains(path, "/terminal") {
		return false
	}
	if strings.HasSuffix(path, "/runtime-secrets/reveal") ||
		(strings.HasSuffix(path, "/runtime-secrets") && method != "put" && !boolValue(agentExtension["verificationOnly"])) {
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
	"getPublicConfigs":                 "public client configuration protocol",
	"login":                            "interactive authentication protocol",
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
	"retryVolumeTransfer":              "effective permission depends on the original transfer direction",
}

var agentTagAliasesZH = map[string][]string{
	"AccessTokens":      {"访问令牌", "个人令牌", "API 令牌"},
	"AppTemplates":      {"应用模板", "应用市场", "模板安装"},
	"Applications":      {"应用", "应用服务"},
	"Auth":              {"认证", "登录配置", "单点登录", "身份提供商", "OIDC"},
	"Billing":           {"账单", "费用", "额度"},
	"Builds":            {"构建", "构建任务"},
	"Configs":           {"运行时配置集", "共享配置", "环境变量集", "配置模板"},
	"Dashboard":         {"仪表盘", "看板", "平台概览"},
	"DataRetention":     {"数据保留", "保留策略", "历史清理", "过期清理"},
	"Deployments":       {"部署", "部署目标"},
	"Events":            {"平台事件", "事件流", "操作动态"},
	"Gateway":           {"网关", "域名", "路由"},
	"Git":               {"代码仓库", "Git 仓库", "代码源", "分支", "Webhook"},
	"Inbox":             {"站内信", "收件箱", "消息中心", "未读消息"},
	"Notifications":     {"通知", "通知渠道", "消息渠道"},
	"OAuthApplications": {"OAuth 应用", "第三方授权", "OAuth 客户端"},
	"Projects":          {"项目空间", "项目"},
	"Registries":        {"镜像仓库", "镜像"},
	"Releases":          {"发布", "版本"},
	"Runtime":           {"运行时", "集群"},
	"Settings":          {"平台设置", "系统配置", "全局配置"},
	"System":            {"系统状态", "系统信息", "平台版本"},
	"Telemetry":         {"遥测", "可观测性", "链路追踪", "指标监控"},
	"Topology":          {"服务拓扑", "拓扑图", "服务依赖", "拓扑连线"},
	"Users":             {"用户", "平台用户", "账号"},
	"Volumes":           {"项目数据卷", "持久卷", "存储卷"},
	"Web":               {"网络搜索", "互联网", "网页", "公开资料"},
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

func localizedText(value any) LocalizedText {
	raw := mapValue(value)
	return LocalizedText{
		ZH: strings.Join(strings.Fields(stringValue(raw["zh"])), " "),
		EN: strings.Join(strings.Fields(stringValue(raw["en"])), " "),
	}
}

func localizedList(value any) LocalizedList {
	raw := mapValue(value)
	return LocalizedList{ZH: uniqueStrings(stringArray(raw["zh"])), EN: uniqueStrings(stringArray(raw["en"]))}
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

var l2MandatoryOperations = map[string]struct{}{
	"createAIModel":                 {},
	"updateAIModel":                 {},
	"updateConfigs":                 {},
	"createUser":                    {},
	"updateUser":                    {},
	"createProjectMember":           {},
	"updateProjectMember":           {},
	"createRuntimeCluster":          {},
	"updateRuntimeCluster":          {},
	"installSystemAppTemplate":      {},
	"updateProjectNetworkPolicy":    {},
	"createProjectHookConfig":       {},
	"updateProjectHookConfig":       {},
	"createProjectRuntimeConfigSet": {},
	"updateProjectRuntimeConfigSet": {},
	"createRelease":                 {},
	"rollbackRelease":               {},
	"cancelRelease":                 {},
	"restartDeploymentTarget":       {},
	"updateDeploymentTarget":        {},
	"createGatewayRoute":            {},
	"updateGatewayRoute":            {},
	"createArtifactRegistry":        {},
	"updateArtifactRegistry":        {},
}

func operationApprovalTier(
	operationID string,
	method string,
	effect string,
	tags []string,
	scopes []string,
	sensitivePaths []string,
	cli map[string]any,
	agent map[string]any,
) (string, error) {
	if explicitTier := strings.ToUpper(stringValue(agent["approvalTier"])); explicitTier != "" {
		switch explicitTier {
		case ApprovalTierL0, ApprovalTierL1, ApprovalTierL2:
			return explicitTier, nil
		default:
			return "", fmt.Errorf("unsupported approvalTier %q", explicitTier)
		}
	}
	if explicitReq, ok := agent["requiresApproval"].(bool); ok && !explicitReq {
		return ApprovalTierL0, nil
	}
	risk := strings.ToLower(stringValue(cli["risk"]))
	highRisk := risk == "high" || risk == "critical" || risk == "destructive" || risk == "sensitive"
	explicitRequiresApproval := boolValue(agent["requiresApproval"])

	if (strings.EqualFold(method, http.MethodGet) || effect == "read" || effect == "discovery" || effect == "presentation" || effect == "interactive") && !explicitRequiresApproval && !highRisk {
		return ApprovalTierL0, nil
	}
	if effect == "execute" && !explicitRequiresApproval && !highRisk && operationID == "testRuntimeCluster" {
		return ApprovalTierL0, nil
	}
	if strings.EqualFold(method, http.MethodDelete) || highRisk {
		return ApprovalTierL2, nil
	}
	if effect == "execute" && explicitRequiresApproval {
		return ApprovalTierL2, nil
	}
	if len(sensitivePaths) > 0 && operationID != "installAppTemplate" {
		return ApprovalTierL2, nil
	}
	for _, protectedScope := range []string{
		"cluster:manage",
		"user:manage",
		"config:write",
		"retention:manage",
		"deployment:exec",
		"deployment:release",
		"secret:update",
		"secret:view_value",
		"registry:write",
		"gateway:manage",
	} {
		if slices.Contains(scopes, protectedScope) {
			return ApprovalTierL2, nil
		}
	}
	for _, protectedTag := range []string{"Billing", "Auth", "AccessTokens", "OAuthApplications", "DataRetention", "Releases"} {
		if slices.Contains(tags, protectedTag) {
			return ApprovalTierL2, nil
		}
	}
	if _, mandatory := l2MandatoryOperations[operationID]; mandatory {
		return ApprovalTierL2, nil
	}
	return ApprovalTierL1, nil
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

var openAPIScalarSchemaKeys = []string{
	"type", "format", "description", "const", "pattern", "minimum", "maximum",
	"exclusiveMinimum", "exclusiveMaximum", "multipleOf",
	"minLength", "maxLength", "minItems", "maxItems", "uniqueItems",
	"minProperties", "maxProperties", "default",
	"writeOnly", "readOnly", "x-luna-sensitive",
}

func resolveOpenAPISchemaRef(document openAPIDocument, schema map[string]any) map[string]any {
	ref := stringValue(schema["$ref"])
	if !strings.HasPrefix(ref, "#/components/schemas/") {
		return schema
	}
	target := mapValue(document.Components.Schemas[strings.TrimPrefix(ref, "#/components/schemas/")])
	if len(schema) == 1 {
		return target
	}
	merged := make(map[string]any, len(target)+len(schema))
	for key, value := range target {
		merged[key] = value
	}
	for key, value := range schema {
		if key == "$ref" {
			continue
		}
		merged[key] = value
	}
	return merged
}

func normalizeOpenAPISchema(document openAPIDocument, schema map[string]any, depth int) map[string]any {
	return normalizeOpenAPISchemaWithMode(document, schema, depth, false)
}

func normalizeOpenAPISchemaWithMode(document openAPIDocument, schema map[string]any, depth int, isConstraint bool) map[string]any {
	if depth > 8 || len(schema) == 0 {
		if isConstraint {
			return map[string]any{}
		}
		return map[string]any{"type": "object"}
	}
	schema = resolveOpenAPISchemaRef(document, schema)
	if len(schema) == 0 {
		if isConstraint {
			return map[string]any{}
		}
		return map[string]any{"type": "object"}
	}

	result := map[string]any{}
	mergedProperties := map[string]any{}
	mergedRequired := make([]string, 0)
	remainingAllOf := make([]any, 0)

	if values := arrayValue(schema["allOf"]); len(values) > 0 {
		for _, rawValue := range values {
			rawBranch := resolveOpenAPISchemaRef(document, mapValue(rawValue))
			if isPureConstraintBranch(rawBranch) {
				if normalized := normalizeOpenAPISchemaWithMode(document, rawBranch, depth+1, true); len(normalized) > 0 {
					remainingAllOf = append(remainingAllOf, normalized)
				}
				continue
			}
			branch := normalizeOpenAPISchemaWithMode(document, rawBranch, depth+1, isConstraint)
			for _, key := range openAPIScalarSchemaKeys {
				if value, ok := branch[key]; ok {
					if (key == "writeOnly" || key == "x-luna-sensitive") && boolValue(result[key]) {
						continue
					}
					result[key] = value
				}
			}
			if enum := arrayValue(branch["enum"]); len(enum) > 0 {
				result["enum"] = enum
			}
			if items := mapValue(branch["items"]); len(items) > 0 {
				result["items"] = items
			}
			if additionalProperties, exists := branch["additionalProperties"]; exists {
				if boolVal, isBool := additionalProperties.(bool); !isBool || boolVal {
					result["additionalProperties"] = additionalProperties
				}
			}
			for name, property := range mapValue(branch["properties"]) {
				mergedProperties[name] = property
			}
			mergedRequired = append(mergedRequired, stringArray(branch["required"])...)
			if nestedAllOf := arrayValue(branch["allOf"]); len(nestedAllOf) > 0 {
				remainingAllOf = append(remainingAllOf, nestedAllOf...)
			}
			conditionalGroup := map[string]any{}
			for _, conditional := range []string{"if", "then", "else"} {
				if sub, exists := branch[conditional]; exists {
					conditionalGroup[conditional] = sub
				}
			}
			if len(conditionalGroup) > 0 {
				remainingAllOf = append(remainingAllOf, conditionalGroup)
			}
			for _, combinator := range []string{"oneOf", "anyOf", "not"} {
				if sub, exists := branch[combinator]; exists {
					remainingAllOf = append(remainingAllOf, map[string]any{combinator: sub})
				}
			}
		}
	}

	for _, key := range openAPIScalarSchemaKeys {
		if value, ok := schema[key]; ok {
			result[key] = value
		}
	}
	if enum := arrayValue(schema["enum"]); len(enum) > 0 {
		result["enum"] = enum
	}
	mergedRequired = append(mergedRequired, stringArray(schema["required"])...)
	if len(mergedRequired) > 0 {
		result["required"] = uniqueStrings(mergedRequired)
	}
	for name, property := range mapValue(schema["properties"]) {
		mergedProperties[name] = normalizeOpenAPISchemaWithMode(document, mapValue(property), depth+1, isConstraint)
	}
	if len(mergedProperties) > 0 {
		if _, hasType := result["type"]; !hasType && !isConstraint {
			result["type"] = "object"
		}
		result["properties"] = mergedProperties
		if !isConstraint {
			if _, hasExplicitAdditional := result["additionalProperties"]; !hasExplicitAdditional {
				result["additionalProperties"] = false
			}
		}
	}
	if additionalProperties, exists := schema["additionalProperties"]; exists {
		switch value := additionalProperties.(type) {
		case bool:
			result["additionalProperties"] = value
		case map[string]any:
			result["additionalProperties"] = normalizeOpenAPISchemaWithMode(document, value, depth+1, isConstraint)
		}
	}
	if items := mapValue(schema["items"]); len(items) > 0 {
		result["items"] = normalizeOpenAPISchemaWithMode(document, items, depth+1, isConstraint)
	}
	branchConstraint := isConstraint || len(mergedProperties) > 0
	for _, key := range []string{"oneOf", "anyOf"} {
		if values := arrayValue(schema[key]); len(values) > 0 {
			normalized := make([]any, 0, len(values))
			for _, value := range values {
				if sub := normalizeOpenAPISchemaWithMode(document, mapValue(value), depth+1, branchConstraint); len(sub) > 0 {
					normalized = append(normalized, sub)
				}
			}
			if len(normalized) > 0 {
				result[key] = normalized
			}
		}
	}
	for _, key := range []string{"if", "then", "else", "not"} {
		if sub := mapValue(schema[key]); len(sub) > 0 {
			if normalized := normalizeOpenAPISchemaWithMode(document, sub, depth+1, true); len(normalized) > 0 {
				result[key] = normalized
			}
		}
	}
	if len(remainingAllOf) > 0 {
		result["allOf"] = remainingAllOf
	}
	if boolValue(schema["nullable"]) {
		applyNullableSchema(result)
	}
	if len(result) == 0 && !isConstraint {
		result["type"] = "object"
	}
	return result
}

func isPureConstraintBranch(schema map[string]any) bool {
	if len(mapValue(schema["properties"])) > 0 || stringValue(schema["type"]) != "" || len(stringArray(schema["type"])) > 0 {
		return false
	}
	if _, hasItems := schema["items"]; hasItems {
		return false
	}
	if _, hasAdditional := schema["additionalProperties"]; hasAdditional {
		return false
	}
	for _, key := range []string{"if", "then", "else", "not", "oneOf", "anyOf"} {
		if _, exists := schema[key]; exists {
			return true
		}
	}
	return false
}

func applyNullableSchema(result map[string]any) {
	if enumValues, ok := result["enum"].([]any); ok && len(enumValues) > 0 {
		hasNull := false
		for _, item := range enumValues {
			if item == nil {
				hasNull = true
				break
			}
		}
		if !hasNull {
			result["enum"] = append(append([]any(nil), enumValues...), nil)
		}
	}
	switch typed := result["type"].(type) {
	case string:
		if typed != "" && typed != "null" {
			result["type"] = []any{typed, "null"}
		} else if typed == "" {
			result["type"] = "null"
		}
	case []any:
		for _, item := range typed {
			if item == "null" {
				return
			}
		}
		result["type"] = append(append([]any(nil), typed...), "null")
	case nil:
		if anyOf, ok := result["anyOf"].([]any); ok {
			result["anyOf"] = append(append([]any(nil), anyOf...), map[string]any{"type": "null"})
			return
		}
		if oneOf, ok := result["oneOf"].([]any); ok {
			result["oneOf"] = append(append([]any(nil), oneOf...), map[string]any{"type": "null"})
			return
		}
		if len(result) == 0 {
			result["type"] = []any{"object", "null"}
			return
		}
		branchCopy := make(map[string]any, len(result))
		for key, value := range result {
			branchCopy[key] = value
			delete(result, key)
		}
		result["anyOf"] = []any{branchCopy, map[string]any{"type": "null"}}
	}
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
	if additional := mapValue(schema["additionalProperties"]); len(additional) > 0 {
		path := prefix + ".*"
		if prefix == "" {
			path = "*"
		}
		paths = append(paths, schemaSensitivePaths(additional, path)...)
	}
	if items := mapValue(schema["items"]); len(items) > 0 {
		path := prefix + ".*"
		if prefix == "" {
			path = "*"
		}
		paths = append(paths, schemaSensitivePaths(items, path)...)
	}
	for _, combinator := range []string{"oneOf", "anyOf", "allOf"} {
		for _, branch := range arrayValue(schema[combinator]) {
			paths = append(paths, schemaSensitivePaths(mapValue(branch), prefix)...)
		}
	}
	for _, conditional := range []string{"then", "else"} {
		if branch := mapValue(schema[conditional]); len(branch) > 0 {
			paths = append(paths, schemaSensitivePaths(branch, prefix)...)
		}
	}
	return uniqueStrings(paths)
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
	if typed, ok := value.([]string); ok {
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(item); text != "" {
				result = append(result, text)
			}
		}
		return result
	}
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
