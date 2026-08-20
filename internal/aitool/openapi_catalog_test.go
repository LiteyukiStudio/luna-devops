package aitool

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestPlatformCatalogUsesExplicitAgentAdmission(t *testing.T) {
	operations, err := PlatformCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if len(operations) < 40 || len(operations) > 80 {
		t.Fatalf("explicitly admitted Agent catalog contains %d operations", len(operations))
	}
	byID := make(map[string]OpenAPIOperation, len(operations))
	for _, operation := range operations {
		if operation.OperationID == "" || operation.Path == "" || operation.Method == "" {
			t.Fatalf("incomplete operation: %#v", operation)
		}
		if !operation.Contract.Allowed || len(operation.Contract.ResourceTypes) == 0 || len(operation.Contract.Intents) == 0 || len(operation.Contract.UseWhen) == 0 || len(operation.Contract.SuccessEvidence) == 0 {
			t.Fatalf("operation %s has incomplete Agent contract: %#v", operation.OperationID, operation.Contract)
		}
		if len(operation.RequiredScopes) == 0 {
			t.Fatalf("operation %s has no stable delegated scope", operation.OperationID)
		}
		if operation.InputSchema["type"] != "object" || operation.InputSchema["additionalProperties"] != false {
			t.Fatalf("operation %s has unsafe input schema: %#v", operation.OperationID, operation.InputSchema)
		}
		byID[operation.OperationID] = operation
	}
	for _, operationID := range []string{
		"getDashboard", "listProjects", "getProject", "createProject",
		"listApplications", "getApplication", "createApplication",
		"listDeploymentTargets", "createDeploymentTarget", "updateDeploymentTarget",
		"listRuntimeClusters", "testRuntimeCluster", "listRuntimeClusterResources",
		"getRuntimeClusterResourceYAML", "listRuntimeClusterResourceEvents",
		"previewBuildTemplate", "triggerBuildRun", "getBuildRun", "getBuildJobLogs",
		"listReleases", "createRelease", "getRelease", "getReleaseRuntimeLogs",
		"listGatewayRoutes", "createGatewayRoute", "getGatewayRoute",
	} {
		if _, ok := byID[operationID]; !ok {
			t.Errorf("missing explicitly admitted workflow operation %s", operationID)
		}
	}
	for _, operationID := range []string{
		"login", "updateProject", "deleteProject", "deleteApplication",
		"execReleaseRuntimeCommand", "createVolumeImport", "listRegistryCredentials",
		"uploadVolumeImportContent", "streamReleaseRuntimeTerminal",
	} {
		if _, ok := byID[operationID]; ok {
			t.Errorf("unreviewed, legacy, or protocol operation entered catalog: %s", operationID)
		}
	}
}

func TestEveryAdmittedOperationPassesTransportAndReadbackContractValidation(t *testing.T) {
	operations, err := PlatformCatalog()
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]OpenAPIOperation, len(operations))
	for _, operation := range operations {
		byID[operation.OperationID] = operation
		if err := validateAgentOperationContract(operation); err != nil {
			t.Errorf("%s: %v", operation.OperationID, err)
		}
	}
	readbackPairs := 0
	for _, operation := range operations {
		if operation.Contract.Verification.Mode == "response" {
			continue
		}
		readbackPairs++
		if err := validateReadbackPair(operation, byID[operation.Contract.Verification.OperationID]); err != nil {
			t.Errorf("%s: %v", operation.OperationID, err)
		}
	}
	if readbackPairs != 3 {
		t.Fatalf("validated %d readback pairs, want 3", readbackPairs)
	}
}

func TestPlatformCatalogReturnsExactNestedContractJSON(t *testing.T) {
	operation, ok := PlatformOperation("createRelease")
	if !ok {
		t.Fatal("missing operation createRelease")
	}
	encoded, err := json.Marshal(operation)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	contract, ok := payload["contract"].(map[string]any)
	if !ok {
		t.Fatalf("nested contract missing from payload: %s", encoded)
	}
	wantKeys := []string{
		"allowed", "resourceTypes", "action", "sideEffect", "idempotent", "replaySafe", "risk", "approval",
		"intents", "useWhen", "avoidWhen", "prerequisites", "parameterSummary", "successEvidence", "commonErrorCodes",
		"predecessors", "followups", "verification",
	}
	for _, key := range wantKeys {
		if _, exists := contract[key]; !exists {
			t.Errorf("contract JSON is missing %q: %#v", key, contract)
		}
	}
	if _, legacy := payload["resultVerifier"]; legacy {
		t.Fatalf("legacy resultVerifier leaked into catalog JSON: %s", encoded)
	}
}

func TestCreateReleaseUsesAuthoritativeAsyncReadback(t *testing.T) {
	operation, ok := PlatformOperation("createRelease")
	if !ok {
		t.Fatal("missing operation createRelease")
	}
	verification := operation.Contract.Verification
	if verification.Mode != "async-readback" || verification.OperationID != "getRelease" || verification.IDSource != "/id" {
		t.Fatalf("createRelease verification = %#v", verification)
	}
	if !reflect.DeepEqual(verification.ArgumentBindings, map[string]string{"projectId": "/projectId", "releaseId": "/id"}) {
		t.Fatalf("createRelease argument bindings = %#v", verification.ArgumentBindings)
	}
	if verification.Completion == nil || verification.Completion.Path != "/status" || !reflect.DeepEqual(verification.Completion.SuccessStates, []string{"succeeded"}) {
		t.Fatalf("createRelease completion = %#v", verification.Completion)
	}
	readback, ok := PlatformOperation("getRelease")
	if !ok || readback.Contract.Verification.Mode != "response" {
		t.Fatalf("getRelease readback contract = %#v", readback.Contract)
	}
}

func TestBuildAndGatewayWritesUseAuthoritativeAsyncReadback(t *testing.T) {
	testCases := []struct {
		operationID string
		readbackID  string
		idSource    string
		bindings    map[string]string
	}{
		{operationID: "triggerBuildRun", readbackID: "getBuildRun", idSource: "/id", bindings: map[string]string{"projectId": "/projectId", "runId": "/id"}},
		{operationID: "createGatewayRoute", readbackID: "getGatewayRoute", idSource: "/id", bindings: map[string]string{"projectId": "/projectId", "routeId": "/id"}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.operationID, func(t *testing.T) {
			operation, ok := PlatformOperation(testCase.operationID)
			if !ok {
				t.Fatalf("missing operation %s", testCase.operationID)
			}
			verification := operation.Contract.Verification
			if verification.Mode != "async-readback" || verification.OperationID != testCase.readbackID || verification.IDSource != testCase.idSource {
				t.Fatalf("%s verification = %#v", testCase.operationID, verification)
			}
			if !reflect.DeepEqual(verification.ArgumentBindings, testCase.bindings) {
				t.Fatalf("%s argument bindings = %#v", testCase.operationID, verification.ArgumentBindings)
			}
			if verification.Completion == nil || verification.Completion.Mode != "state" || verification.Completion.Path != "/status" || len(verification.Completion.SuccessStates) == 0 || len(verification.Completion.FailureStates) == 0 {
				t.Fatalf("%s completion = %#v", testCase.operationID, verification.Completion)
			}
			readback, ok := PlatformOperation(testCase.readbackID)
			if !ok || readback.Contract.Verification.Mode != "response" {
				t.Fatalf("%s readback contract = %#v", testCase.readbackID, readback.Contract)
			}
		})
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
	for _, operationID := range []string{"getRuntimeClusterResourceYAML", "listRuntimeClusterResourceEvents", "deleteRuntimeClusterResource"} {
		operation, ok := PlatformOperation(operationID)
		if !ok {
			t.Fatalf("missing %s", operationID)
		}
		properties := mapValue(operation.InputSchema["properties"])
		kind := mapValue(properties["resourceKind"])
		if _, legacy := properties["kind"]; legacy || len(arrayValue(kind["enum"])) != 11 {
			t.Fatalf("%s resourceKind schema = %#v", operationID, properties)
		}
	}
}

func TestPlatformCatalogProvidesChineseSemanticContract(t *testing.T) {
	operation, ok := PlatformOperation("listProjects")
	if !ok {
		t.Fatal("missing operation listProjects")
	}
	joined := strings.Join(operation.SearchHints, " ")
	if !strings.Contains(joined, "查找项目空间") || !strings.Contains(joined, "scope") {
		t.Fatalf("semantic hints are missing: %#v", operation.SearchHints)
	}
	if !strings.HasPrefix(operation.Description, "用途：") {
		t.Fatalf("model-facing description must remain Chinese: %q", operation.Description)
	}
}

func TestPlatformCatalogMarksRuntimeSecretInputsAsSensitive(t *testing.T) {
	for _, operationID := range []string{"updateDeploymentTargetRuntimeSecrets", "updateProjectRuntimeConfigSetRuntimeSecrets"} {
		operation, ok := PlatformOperation(operationID)
		if !ok {
			t.Fatalf("missing secure runtime secret operation %s", operationID)
		}
		if operation.Risk != "sensitive" || operation.Approval != "always" || operation.StepUpPurpose != "secret_update" {
			t.Fatalf("runtime secret policy = %#v", operation)
		}
		if len(operation.SensitivePaths) != 1 || operation.SensitivePaths[0] != "body.items.*.value" {
			t.Fatalf("runtime secret sensitive paths = %#v", operation.SensitivePaths)
		}
	}
}

func TestAgentContractValidationFailsClosed(t *testing.T) {
	base := AgentToolContract{
		Allowed: true, ResourceTypes: []string{"project"}, Action: "read", SideEffect: "none",
		Idempotent: true, ReplaySafe: true, Risk: "low", Approval: "never",
		Intents: []string{"读取项目"}, UseWhen: []string{"已有项目标识时"}, SuccessEvidence: []string{"响应返回项目"},
		Verification: AgentToolVerification{Mode: "response", SuccessCodes: []int{200}},
	}
	if err := validateAgentToolContract("getProject", base); err != nil {
		t.Fatalf("valid contract failed: %v", err)
	}
	writeWithoutBoundaries := base
	writeWithoutBoundaries.Action = "create"
	writeWithoutBoundaries.SideEffect = "platform-write"
	if err := validateAgentToolContract("createProject", writeWithoutBoundaries); err == nil || !strings.Contains(err.Error(), "avoidWhen") {
		t.Fatalf("write contract without boundary was accepted: %v", err)
	}
	highWithoutApproval := base
	highWithoutApproval.Risk = "high"
	if err := validateAgentToolContract("sensitiveRead", highWithoutApproval); err == nil || !strings.Contains(err.Error(), "approval") {
		t.Fatalf("high-risk contract without approval was accepted: %v", err)
	}
}

func TestAgentContractReferencesMustResolveToAdmittedTools(t *testing.T) {
	operation := OpenAPIOperation{OperationID: "createProject", Contract: AgentToolContract{Followups: []string{"getProject"}}}
	if err := validateAgentContractReferences([]OpenAPIOperation{operation}); err == nil || !strings.Contains(err.Error(), "getProject") {
		t.Fatalf("dangling Agent reference was accepted: %v", err)
	}
}
