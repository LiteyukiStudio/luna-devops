package aitool

import (
	"encoding/json"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var openAPIPathParameterPattern = regexp.MustCompile(`\{([^}]+)\}`)

func TestPlatformCatalogDefaultsToRegularOpenAPIOperations(t *testing.T) {
	operations, err := PlatformCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if len(operations) < 100 {
		t.Fatalf("OpenAPI-derived Agent catalog is unexpectedly small: %d", len(operations))
	}
	byID := make(map[string]OpenAPIOperation, len(operations))
	for _, operation := range operations {
		if operation.OperationID == "" || operation.Path == "" || operation.Method == "" || operation.Summary == "" {
			t.Fatalf("incomplete operation: %#v", operation)
		}
		if len(operation.RequiredScopes) == 0 {
			t.Fatalf("operation %s has no stable scope", operation.OperationID)
		}
		if strings.TrimSpace(operation.Purpose.ZH) == "" {
			t.Fatalf("operation %s has no Chinese-first purpose", operation.OperationID)
		}
		if operation.InputSchema["type"] != "object" || operation.InputSchema["additionalProperties"] != false {
			t.Fatalf("operation %s has unsafe input schema: %#v", operation.OperationID, operation.InputSchema)
		}
		byID[operation.OperationID] = operation
	}
	for _, operationID := range []string{
		"getDashboard", "listProjects", "getProject", "createProject", "updateProject", "deleteProject",
		"listProjectVolumes", "getProjectVolume", "createProjectVolume", "updateProjectVolume",
		"previewProjectVolumeDeletion", "deleteProjectVolume",
		"retryProjectVolumeOperation", "webSearch", "fetchWebPage",
	} {
		if _, ok := byID[operationID]; !ok {
			t.Errorf("regular platform operation is missing: %s", operationID)
		}
	}
	for _, operationID := range []string{"login", "createAccessToken", "createVolumeImport", "streamReleaseRuntimeTerminal"} {
		if _, ok := byID[operationID]; ok {
			t.Errorf("protocol or credential operation entered catalog: %s", operationID)
		}
	}
}

func TestPlatformCatalogPathParametersExactlyMatchRoutePlaceholders(t *testing.T) {
	operations, err := PlatformCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, operation := range operations {
		placeholders := make([]string, 0)
		for _, match := range openAPIPathParameterPattern.FindAllStringSubmatch(operation.Path, -1) {
			placeholders = append(placeholders, match[1])
		}
		declared := make([]string, 0)
		for _, parameter := range operation.Parameters {
			if parameter.In == "path" {
				declared = append(declared, parameter.WireName)
			}
		}
		sort.Strings(placeholders)
		sort.Strings(declared)
		if !reflect.DeepEqual(placeholders, declared) {
			t.Errorf("%s path parameters = %v, placeholders = %v", operation.OperationID, declared, placeholders)
		}
	}
}

func TestPlatformCatalogJSONIsMinimalAndSelfContained(t *testing.T) {
	operation, ok := PlatformOperation("createProjectVolume")
	if !ok {
		t.Fatal("missing createProjectVolume")
	}
	encoded, err := json.Marshal(operation)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, removed := range []string{"\"contract\"", "predecessors", "followups", "verification", "searchHints", "\"allowed\""} {
		if strings.Contains(text, removed) {
			t.Fatalf("legacy catalog field %s leaked: %s", removed, text)
		}
	}
	for _, required := range []string{"operationId", "summary", "tags", "aliases", "purpose", "avoidWhen", "preconditions", "successEvidence", "requiresApproval", "inputSchema", "outputSchema", "parameters"} {
		if !strings.Contains(text, `"`+required+`"`) {
			t.Fatalf("catalog JSON is missing %s: %s", required, text)
		}
	}
	if !operation.RequiresApproval {
		t.Fatal("high-risk volume creation must require approval")
	}
}

func TestHighFrequencyOperationsCarryExplicitSemanticMetadata(t *testing.T) {
	for _, operationID := range []string{
		"listProjects", "createProject", "getProject", "updateProject", "deleteProject",
		"listProjectVolumes", "createProjectVolume", "getProjectVolume", "updateProjectVolume",
		"deleteProjectVolume", "retryProjectVolumeOperation", "previewProjectVolumeDeletion",
		"listRuntimeClusters", "listRuntimeClusterResources", "getDashboard", "listUsers",
		"getBillingSummary", "listNotificationChannels", "webSearch", "fetchWebPage",
		"listAppTemplates", "getAppTemplate",
		"listApplications", "createApplication", "getApplication", "updateApplication", "deleteApplication",
		"listDeploymentTargets", "createDeploymentTarget", "updateDeploymentTarget", "deleteDeploymentTarget",
		"listBuildRuns", "triggerBuildRun", "getBuildRun", "retryBuildRun",
		"listReleases", "createRelease", "getRelease",
		"listGatewayRoutes", "createGatewayRoute", "getGatewayRoute", "updateGatewayRoute", "deleteGatewayRoute",
		"checkGatewayDomain",
	} {
		operation, ok := PlatformOperation(operationID)
		if !ok {
			t.Fatalf("missing %s", operationID)
		}
		if strings.TrimSpace(operation.Purpose.ZH) == "" || strings.TrimSpace(operation.Purpose.EN) == "" ||
			len(operation.Aliases.ZH) == 0 || len(operation.Aliases.EN) == 0 ||
			strings.TrimSpace(operation.AvoidWhen.ZH) == "" || len(operation.Preconditions.ZH) == 0 ||
			strings.TrimSpace(operation.SuccessEvidence.ZH) == "" {
			t.Errorf("%s semantic metadata is incomplete: %#v", operationID, operation)
		}
	}
}

func TestProjectVolumeCatalogCarriesBilingualSearchTermsAndSchemas(t *testing.T) {
	operation, ok := PlatformOperation("listProjectVolumes")
	if !ok {
		t.Fatal("missing listProjectVolumes")
	}
	if !strings.Contains(strings.Join(operation.Aliases.ZH, " "), "项目数据卷") ||
		!strings.Contains(strings.Join(operation.Aliases.ZH, " "), "持久化存储") ||
		!strings.Contains(strings.Join(operation.Aliases.EN, " "), "list project volumes") ||
		!strings.Contains(strings.Join(operation.Aliases.EN, " "), "ProjectVolume") {
		t.Fatalf("aliases are incomplete: %#v", operation.Aliases)
	}
	if len(mapValue(operation.InputSchema["properties"])) == 0 || len(operation.OutputSchema) == 0 {
		t.Fatalf("schemas are incomplete: %#v", operation)
	}
	if operation.RequiresApproval {
		t.Fatal("read-only volume list must not require approval")
	}
}

func TestPlatformReadOperationsCarryGoalSpecificChineseAliases(t *testing.T) {
	expected := map[string][]string{
		"getDashboard":             {"平台仪表盘", "看板概览"},
		"listUsers":                {"平台用户", "用户列表"},
		"listNotificationChannels": {"通知渠道", "通知渠道列表"},
		"getBillingSummary":        {"平台账单", "费用概览"},
	}
	for operationID, terms := range expected {
		operation, ok := PlatformOperation(operationID)
		if !ok {
			t.Fatalf("missing %s", operationID)
		}
		aliases := strings.Join(operation.Aliases.ZH, " ")
		for _, term := range terms {
			if !strings.Contains(aliases, term) {
				t.Errorf("%s aliases %q do not contain %q", operationID, aliases, term)
			}
		}
	}
}

func TestWebToolsRemainDiscoverableAndServiceRouted(t *testing.T) {
	expected := map[string]struct {
		path  string
		alias string
	}{
		"webSearch":    {path: "/api/v1/ai-tools/web-search", alias: "网络搜索"},
		"fetchWebPage": {path: "/api/v1/ai-tools/fetch-web-page", alias: "读取网页"},
	}
	for operationID, want := range expected {
		operation, ok := PlatformOperation(operationID)
		if !ok {
			t.Fatalf("missing %s", operationID)
		}
		if operation.Method != "POST" || operation.Path != want.path || !operation.Idempotent || operation.RequiresApproval {
			t.Errorf("%s transport/policy = %#v", operationID, operation)
		}
		if !reflect.DeepEqual(operation.RequiredScopes, []string{"web:read"}) {
			t.Errorf("%s scopes = %#v", operationID, operation.RequiredScopes)
		}
		if !strings.Contains(strings.Join(operation.Aliases.ZH, " "), want.alias) {
			t.Errorf("%s aliases = %#v", operationID, operation.Aliases)
		}
		properties := mapValue(operation.InputSchema["properties"])
		body := mapValue(properties["body"])
		if len(body) == 0 || !operation.RequestBody || !operation.RequestRequired {
			t.Errorf("%s request schema = %#v", operationID, operation)
		}
	}
}

func TestRuntimeResourceOperationsExposeDistinctStrictArguments(t *testing.T) {
	list, ok := PlatformOperation("listRuntimeClusterResources")
	if !ok {
		t.Fatal("missing listRuntimeClusterResources")
	}
	listProperties := mapValue(list.InputSchema["properties"])
	category := mapValue(listProperties["resourceCategory"])
	if _, legacy := listProperties["kind"]; legacy || !reflect.DeepEqual(category["enum"], []any{"namespaces", "workloads", "services", "configs", "storage"}) {
		t.Fatalf("resourceCategory schema = %#v", listProperties)
	}
}

func TestPlatformCatalogMarksRuntimeSecretInputsAsSensitive(t *testing.T) {
	for _, operationID := range []string{"updateDeploymentTargetRuntimeSecrets", "updateProjectRuntimeConfigSetRuntimeSecrets"} {
		operation, ok := PlatformOperation(operationID)
		if !ok {
			t.Fatalf("missing secure runtime secret operation %s", operationID)
		}
		if !operation.RequiresApproval {
			t.Fatalf("runtime secret operation must require approval: %#v", operation)
		}
		if len(operation.SensitivePaths) != 1 || operation.SensitivePaths[0] != "body.items.*.value" {
			t.Fatalf("runtime secret sensitive paths = %#v", operation.SensitivePaths)
		}
	}
}

func TestDisabledOperationsAlwaysExplainWhy(t *testing.T) {
	for operationID, reason := range agentDisabledOperations {
		if strings.TrimSpace(operationID) == "" || strings.TrimSpace(reason) == "" {
			t.Fatalf("invalid disabled operation entry %q=%q", operationID, reason)
		}
	}
}
