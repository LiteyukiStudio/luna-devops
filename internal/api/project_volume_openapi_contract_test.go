package api

import (
	"reflect"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/aitool"
)

func TestProjectVolumeOperationsAreDerivedFromOpenAPI(t *testing.T) {
	tests := map[string]struct {
		scope            string
		requiresApproval bool
	}{
		"listProjectVolumes":              {scope: "volume:read"},
		"getProjectVolume":                {scope: "volume:read"},
		"listProjectVolumeStorageClasses": {scope: "volume:read"},
		"createProjectVolume":             {scope: "volume:write", requiresApproval: true},
		"updateProjectVolume":             {scope: "volume:write", requiresApproval: true},
		"previewProjectVolumeDeletion":    {scope: "volume:delete", requiresApproval: true},
		"deleteProjectVolume":             {scope: "volume:delete", requiresApproval: true},
		"retryProjectVolumeOperation":     {scope: "volume:read", requiresApproval: true},
		"createVolumeExport":              {scope: "volume:export", requiresApproval: true},
	}
	for operationID, test := range tests {
		operation, ok := aitool.PlatformOperation(operationID)
		if !ok {
			t.Fatalf("regular volume operation %s is missing from Agent catalog", operationID)
		}
		if !reflect.DeepEqual(operation.RequiredScopes, []string{test.scope}) || operation.RequiresApproval != test.requiresApproval {
			t.Fatalf("%s Agent policy = %#v", operationID, operation)
		}
	}
	for _, operationID := range []string{"retryVolumeTransfer", "createVolumeImport"} {
		if _, ok := aitool.PlatformOperation(operationID); ok {
			t.Fatalf("explicitly disabled or protocol volume operation entered Agent catalog: %s", operationID)
		}
	}
}

func TestProjectVolumeOpenAPICLIMetadata(t *testing.T) {
	document := readOpenAPIDocument(t, apiRepositoryRoot(t)+"/openapi/openapi.yaml")
	paths := document["paths"].(map[string]any)
	operations := make(map[string]map[string]any)
	for _, rawPath := range paths {
		path, ok := rawPath.(map[string]any)
		if !ok {
			continue
		}
		for _, rawOperation := range path {
			operation, ok := rawOperation.(map[string]any)
			if !ok {
				continue
			}
			operationID, _ := operation["operationId"].(string)
			if operationID != "" {
				operations[operationID] = operation
			}
		}
	}
	tests := map[string]struct {
		command string
		risk    string
		scope   string
	}{
		"listProjectVolumes":              {"volume.list", "low", "volume:read"},
		"createProjectVolume":             {"volume.create", "high", "volume:write"},
		"getProjectVolume":                {"volume.get", "low", "volume:read"},
		"updateProjectVolume":             {"volume.update", "high", "volume:write"},
		"deleteProjectVolume":             {"volume.delete", "critical", "volume:delete"},
		"retryProjectVolumeOperation":     {"volume.retry", "high", "volume:read"},
		"previewProjectVolumeDeletion":    {"volume.delete-preview", "high", "volume:delete"},
		"listProjectVolumeStorageClasses": {"volume.storage-classes", "low", "volume:read"},
		"createVolumeExport":              {"volume.export", "high", "volume:export"},
		"listVolumeTransfers":             {"volume-transfer.list", "low", "volume:read"},
		"getVolumeTransfer":               {"volume-transfer.get", "low", "volume:read"},
		"retryVolumeTransfer":             {"volume-transfer.retry", "high", "volume:read"},
		"cancelVolumeTransfer":            {"volume-transfer.cancel", "high", "volume:read"},
	}
	for operationID, test := range tests {
		operation := operations[operationID]
		if operation == nil {
			t.Fatalf("missing OpenAPI operation %s", operationID)
		}
		metadata := operation["x-luna-cli"].(map[string]any)
		scopes, _ := schemaStringList(metadata["requiredScopes"])
		if metadata["command"] != test.command || metadata["classification"] != "business-command" || metadata["risk"] != test.risk || !reflect.DeepEqual(scopes, []string{test.scope}) {
			t.Fatalf("%s CLI metadata = %#v", operationID, metadata)
		}
	}
	for _, operationID := range []string{"retryVolumeTransfer"} {
		metadata := operations[operationID]["x-luna-cli"].(map[string]any)
		if metadata["agentAllowed"] != false || metadata["exclusionReason"] == "" {
			t.Fatalf("%s must stay out of the Agent catalog because its effective authorization depends on the original operation", operationID)
		}
	}
}

func TestVolumeUploadOpenAPIUsesSingleDirectStreamContract(t *testing.T) {
	document := readOpenAPIDocument(t, apiRepositoryRoot(t)+"/openapi/openapi.yaml")
	components := document["components"].(map[string]any)
	schemas := components["schemas"].(map[string]any)
	transfer := schemas["VolumeTransfer"].(map[string]any)
	_, ok := schemaStringList(transfer["required"])
	if !ok {
		t.Fatalf("VolumeTransfer.required = %#v", transfer["required"])
	}
	transferProperties := transfer["properties"].(map[string]any)
	for _, removed := range []string{"chunkSize", "uploadOffset", "objectKey", "multipartUploadId"} {
		if _, exists := transferProperties[removed]; exists {
			t.Fatalf("VolumeTransfer retains legacy property %s", removed)
		}
	}

	projectVolume := schemas["ProjectVolume"].(map[string]any)
	pendingOperations, ok := schemaStringList(projectVolume["properties"].(map[string]any)["pendingOperation"].(map[string]any)["enum"])
	if !ok || !containsSchemaString(pendingOperations, "import") {
		t.Fatalf("ProjectVolume.pendingOperation = %#v", pendingOperations)
	}
	createResponse := schemas["VolumeImportCreateResponse"].(map[string]any)
	if createResponse["properties"].(map[string]any)["transfer"].(map[string]any)["$ref"] != "#/components/schemas/VolumeTransfer" {
		t.Fatalf("VolumeImportCreateResponse.transfer = %#v", createResponse)
	}

	paths := document["paths"].(map[string]any)
	contentPath := paths["/api/v1/projects/{projectId}/volume-imports/{transferId}/content"].(map[string]any)
	if len(contentPath) != 1 || contentPath["put"] == nil {
		t.Fatalf("direct import methods = %#v", contentPath)
	}
	put := contentPath["put"].(map[string]any)
	requestContent := put["requestBody"].(map[string]any)["content"].(map[string]any)
	if _, exists := requestContent["application/octet-stream"]; !exists {
		t.Fatalf("direct import content types = %#v", requestContent)
	}
	parameters := put["parameters"].([]any)
	foundLength := false
	for _, raw := range parameters {
		parameter := raw.(map[string]any)
		if parameter["name"] == "X-Content-SHA256" {
			t.Fatal("direct import must not require a client-declared checksum")
		}
		if parameter["name"] == "Content-Length" && parameter["in"] == "header" && parameter["required"] == true {
			foundLength = true
		}
	}
	if !foundLength {
		t.Fatalf("direct import parameters = %#v", parameters)
	}
	createInput := schemas["VolumeImportCreateInput"].(map[string]any)
	if _, exists := createInput["properties"].(map[string]any)["sha256"]; exists {
		t.Fatal("volume import creation must not accept a client-declared checksum")
	}
	for path := range paths {
		if len(path) >= len("/internal/v1/volume-transfers") && path[:len("/internal/v1/volume-transfers")] == "/internal/v1/volume-transfers" {
			t.Fatalf("legacy internal transfer path remains: %s", path)
		}
	}
}

func TestVolumeByteTransferProtocolIsNotExposedToAgent(t *testing.T) {
	for _, operationID := range []string{
		"createVolumeImport",
		"uploadVolumeImportContent",
		"authorizeVolumeTransferDownload",
		"downloadVolumeTransferContent",
		"downloadVolumeTransferManifest",
	} {
		if operation, ok := aitool.PlatformOperation(operationID); ok {
			t.Fatalf("protocol operation %s leaked into Agent catalog: %#v", operationID, operation)
		}
	}
}

func TestDeploymentVolumeOpenAPIUsesTypedMountContractOnly(t *testing.T) {
	document := readOpenAPIDocument(t, apiRepositoryRoot(t)+"/openapi/openapi.yaml")
	components := document["components"].(map[string]any)
	schemas := components["schemas"].(map[string]any)
	input := schemas["DeploymentTargetInput"].(map[string]any)
	properties := input["properties"].(map[string]any)
	dataVolumes := properties["dataVolumes"].(map[string]any)
	if dataVolumes["type"] != "array" || dataVolumes["maxItems"] != float64(maxDeploymentDataVolumes) {
		t.Fatalf("DeploymentTargetInput.dataVolumes = %#v", dataVolumes)
	}
	items := dataVolumes["items"].(map[string]any)
	if items["$ref"] != "#/components/schemas/DeploymentDataVolumeInput" {
		t.Fatalf("DeploymentTargetInput.dataVolumes.items = %#v", items)
	}
	for _, legacy := range []string{"dataRetentionEnabled", "dataCapacity", "dataMountPath", "dataStorageClassName", "dataAccessMode", "dataVolumeMode"} {
		if _, exists := properties[legacy]; exists {
			t.Fatalf("legacy deployment storage property %s remains public", legacy)
		}
	}
	installProperties := schemas["AppTemplateInstallInput"].(map[string]any)["properties"].(map[string]any)
	if _, exists := installProperties["projectVolumeId"]; !exists {
		t.Fatal("AppTemplateInstallInput is missing projectVolumeId")
	}
	for _, legacy := range []string{"dataCapacity", "retainedVolumeId"} {
		if _, exists := installProperties[legacy]; exists {
			t.Fatalf("legacy app-template storage property %s remains public", legacy)
		}
	}
	templateProperties := schemas["AppTemplate"].(map[string]any)["properties"].(map[string]any)
	templateDataVolumes := templateProperties["dataVolumes"].(map[string]any)
	if templateDataVolumes["type"] != "array" || templateDataVolumes["items"].(map[string]any)["$ref"] != "#/components/schemas/AppTemplateDataVolume" {
		t.Fatalf("AppTemplate.dataVolumes = %#v", templateDataVolumes)
	}
	for _, legacy := range []string{"dataRetentionEnabled", "dataMountPath", "dataCapacity"} {
		if _, exists := templateProperties[legacy]; exists {
			t.Fatalf("legacy app-template volume property %s remains public", legacy)
		}
	}
	paths := document["paths"].(map[string]any)
	applicationPath := paths["/api/v1/projects/{projectId}/applications/{applicationId}"].(map[string]any)
	if _, exists := applicationPath["delete"].(map[string]any)["requestBody"]; exists {
		t.Fatal("deleteApplication still accepts the legacy dataAction body")
	}
	for _, legacyPath := range []string{
		"/api/v1/projects/{projectId}/retained-volumes",
		"/api/v1/projects/{projectId}/retained-volumes/{retainedVolumeId}",
		"/api/v1/projects/{projectId}/applications/{applicationId}/deployment-targets/{targetId}/data-export",
		"/api/v1/projects/{projectId}/applications/{applicationId}/deployment-targets/{targetId}/data-export/authorize",
	} {
		if _, exists := paths[legacyPath]; exists {
			t.Fatalf("legacy deployment-volume path remains public: %s", legacyPath)
		}
	}
}

func TestVolumeManifestOpenAPIContract(t *testing.T) {
	document := readOpenAPIDocument(t, apiRepositoryRoot(t)+"/openapi/openapi.yaml")
	paths := document["paths"].(map[string]any)
	manifestPath := paths["/api/v1/projects/{projectId}/volume-transfers/{transferId}/manifest"].(map[string]any)
	if _, exists := manifestPath["head"]; exists {
		t.Fatal("manifest retains legacy HEAD operation")
	}
	operation := manifestPath["get"].(map[string]any)
	extension := operation["x-luna-cli"].(map[string]any)
	if extension["classification"] != "protocol-adapter" || extension["hidden"] != true || extension["agentAllowed"] != false {
		t.Fatalf("manifest GET CLI metadata = %#v", extension)
	}
	if reason, _ := extension["exclusionReason"].(string); reason == "" {
		t.Fatalf("manifest GET CLI exclusion reason = %#v", extension["exclusionReason"])
	}
	components := document["components"].(map[string]any)
	schemas := components["schemas"].(map[string]any)
	manifest := schemas["VolumeTransferManifest"].(map[string]any)
	required, ok := schemaStringList(manifest["required"])
	want := []string{"schemaVersion", "volumeMode", "format", "exportedAt", "logicalBytes", "fileCount", "dataSHA256", "consistencyMode"}
	if !ok || !reflect.DeepEqual(required, want) {
		t.Fatalf("VolumeTransferManifest.required = %v, want %v", required, want)
	}
}

func TestVolumeTransferProtocolOpenAPIContract(t *testing.T) {
	document := readOpenAPIDocument(t, apiRepositoryRoot(t)+"/openapi/openapi.yaml")
	paths := document["paths"].(map[string]any)
	operations := []struct {
		method string
		path   string
	}{
		{method: "put", path: "/api/v1/projects/{projectId}/volume-imports/{transferId}/content"},
		{method: "post", path: "/api/v1/projects/{projectId}/volume-transfers/{transferId}/download-authorizations"},
		{method: "get", path: "/api/v1/projects/{projectId}/volume-transfers/{transferId}/content"},
	}
	for _, item := range operations {
		operation := paths[item.path].(map[string]any)[item.method].(map[string]any)
		extension := operation["x-luna-cli"].(map[string]any)
		reason, _ := extension["exclusionReason"].(string)
		if extension["classification"] != "protocol-adapter" || extension["hidden"] != true || extension["agentAllowed"] != false || reason == "" {
			t.Fatalf("%s %s CLI metadata = %#v", item.method, item.path, extension)
		}
	}
}

func TestDeploymentRuntimeSecretSummaryOpenAPIContract(t *testing.T) {
	document := readOpenAPIDocument(t, apiRepositoryRoot(t)+"/openapi/openapi.yaml")
	paths := document["paths"].(map[string]any)
	path := paths["/api/v1/projects/{projectId}/applications/{applicationId}/deployment-targets/{targetId}/runtime-secrets"].(map[string]any)
	operation := path["get"].(map[string]any)
	extension := operation["x-luna-cli"].(map[string]any)
	if extension["classification"] != "browser-workflow" || extension["hidden"] != true || extension["agentAllowed"] != false {
		t.Fatalf("runtime secret summary CLI metadata = %#v", extension)
	}
}

func TestProjectVolumeOpenAPIPaginationContract(t *testing.T) {
	document := readOpenAPIDocument(t, apiRepositoryRoot(t)+"/openapi/openapi.yaml")
	components := document["components"].(map[string]any)
	schemas := components["schemas"].(map[string]any)
	for _, schemaName := range []string{"PaginatedProjectVolumes", "PaginatedProjectVolumeStorageClasses", "PaginatedVolumeTransfers"} {
		schema := schemas[schemaName].(map[string]any)
		required, ok := schemaStringList(schema["required"])
		if !ok {
			t.Fatalf("%s has no required fields", schemaName)
		}
		want := []string{"items", "page", "pageSize", "sortBy", "sortOrder", "total", "totalPages"}
		if !reflect.DeepEqual(required, want) {
			t.Fatalf("%s required = %v, want %v", schemaName, required, want)
		}
		properties := schema["properties"].(map[string]any)
		pageSize := properties["pageSize"].(map[string]any)
		if pageSize["maximum"] != float64(100) {
			t.Fatalf("%s pageSize maximum = %#v", schemaName, pageSize["maximum"])
		}
	}
}

func TestVolumeDownloadOpenAPIUsesRequiredOneTimeTicket(t *testing.T) {
	document := readOpenAPIDocument(t, apiRepositoryRoot(t)+"/openapi/openapi.yaml")
	components := document["components"].(map[string]any)
	parameters := components["parameters"].(map[string]any)
	ticket := parameters["DownloadTicket"].(map[string]any)
	if required, _ := ticket["required"].(bool); !required {
		t.Fatal("DownloadTicket must be required for the one-time stream")
	}
	if _, exists := parameters["DownloadSessionCookie"]; exists {
		t.Fatal("legacy download session cookie remains in OpenAPI")
	}

	paths := document["paths"].(map[string]any)
	contentPath := paths["/api/v1/projects/{projectId}/volume-transfers/{transferId}/content"].(map[string]any)
	if _, exists := contentPath["head"]; exists {
		t.Fatal("content retains legacy HEAD operation")
	}
	operation := contentPath["get"].(map[string]any)
	operationParameters := operation["parameters"].([]any)
	foundTicket := false
	for _, raw := range operationParameters {
		parameter := raw.(map[string]any)
		if parameter["$ref"] == "#/components/parameters/DownloadTicket" {
			foundTicket = true
		}
	}
	if !foundTicket {
		t.Fatal("download GET is missing the one-time ticket")
	}
	if _, ok := operation["responses"].(map[string]any)["401"]; !ok {
		t.Fatal("download GET is missing the stable unauthorized response")
	}
}

func containsSchemaString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
