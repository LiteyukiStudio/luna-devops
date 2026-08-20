package api

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/aitool"
)

func TestProjectVolumeOperationsRequireExplicitAgentAdmission(t *testing.T) {
	for _, operationID := range []string{
		"listProjectVolumes", "getProjectVolume", "listProjectVolumeStorageClasses",
		"createProjectVolume", "updateProjectVolume", "previewProjectVolumeDeletion",
		"deleteProjectVolume", "createVolumeExport",
	} {
		if operation, ok := aitool.PlatformOperation(operationID); ok {
			t.Fatalf("unreviewed volume operation %s entered Agent catalog: %#v", operationID, operation.Contract)
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
	for _, operationID := range []string{"retryProjectVolumeOperation", "retryVolumeTransfer"} {
		metadata := operations[operationID]["x-luna-cli"].(map[string]any)
		if metadata["agentAllowed"] != false || metadata["exclusionReason"] == "" {
			t.Fatalf("%s must stay out of the Agent catalog because its effective authorization depends on the original operation", operationID)
		}
	}
}

func TestVolumeUploadOpenAPIUsesServerSelectedMultipartContract(t *testing.T) {
	document := readOpenAPIDocument(t, apiRepositoryRoot(t)+"/openapi/openapi.yaml")
	components := document["components"].(map[string]any)
	schemas := components["schemas"].(map[string]any)
	transfer := schemas["VolumeTransfer"].(map[string]any)
	required, ok := schemaStringList(transfer["required"])
	if !ok || !slices.Contains(required, "chunkSize") {
		t.Fatalf("VolumeTransfer.required = %#v", transfer["required"])
	}
	chunkSize := transfer["properties"].(map[string]any)["chunkSize"].(map[string]any)
	if chunkSize["minimum"] != float64(64*1024*1024) || chunkSize["maximum"] != float64(5*1024*1024*1024) {
		t.Fatalf("VolumeTransfer.chunkSize = %#v", chunkSize)
	}

	projectVolume := schemas["ProjectVolume"].(map[string]any)
	pendingOperations, ok := schemaStringList(projectVolume["properties"].(map[string]any)["pendingOperation"].(map[string]any)["enum"])
	if !ok || !slices.Contains(pendingOperations, "import") {
		t.Fatalf("ProjectVolume.pendingOperation = %#v", pendingOperations)
	}
	createResponse := schemas["VolumeImportCreateResponse"].(map[string]any)
	if createResponse["properties"].(map[string]any)["transfer"].(map[string]any)["$ref"] != "#/components/schemas/VolumeTransfer" {
		t.Fatalf("VolumeImportCreateResponse.transfer = %#v", createResponse)
	}

	paths := document["paths"].(map[string]any)
	for _, path := range []string{
		"/api/v1/projects/{projectId}/volume-imports/{transferId}/content",
		"/internal/v1/volume-transfers/{transferId}/content",
	} {
		content := paths[path].(map[string]any)
		headHeaders := content["head"].(map[string]any)["responses"].(map[string]any)["200"].(map[string]any)["headers"].(map[string]any)
		if _, exists := headHeaders["Upload-Chunk-Size"]; !exists {
			t.Fatalf("%s HEAD is missing Upload-Chunk-Size", path)
		}
		patch := content["patch"].(map[string]any)
		patchHeaders := patch["responses"].(map[string]any)["204"].(map[string]any)["headers"].(map[string]any)
		if _, exists := patchHeaders["Upload-Chunk-Size"]; !exists {
			t.Fatalf("%s PATCH is missing Upload-Chunk-Size", path)
		}
		binary := patch["requestBody"].(map[string]any)["content"].(map[string]any)["application/offset+octet-stream"].(map[string]any)["schema"].(map[string]any)
		if binary["maxLength"] != float64(5*1024*1024*1024) {
			t.Fatalf("%s PATCH maxLength = %#v", path, binary["maxLength"])
		}
		for _, status := range []string{"413", "429", "503", "507"} {
			if _, exists := patch["responses"].(map[string]any)[status]; !exists {
				t.Fatalf("%s PATCH is missing %s", path, status)
			}
		}
		conflictHeaders := patch["responses"].(map[string]any)["409"].(map[string]any)["headers"].(map[string]any)
		if _, exists := conflictHeaders["Retry-After"]; !exists {
			t.Fatalf("%s PATCH 409 is missing Retry-After", path)
		}
		if _, exists := conflictHeaders["Upload-Offset"]; !exists {
			t.Fatalf("%s PATCH 409 is missing Upload-Offset", path)
		}
	}

	lastError := transfer["properties"].(map[string]any)["lastErrorCode"].(map[string]any)
	if !strings.Contains(lastError["description"].(string), "volume_transfer.completion_missing") {
		t.Fatalf("VolumeTransfer.lastErrorCode description = %#v", lastError)
	}
	failCodes, ok := schemaStringList(schemas["VolumeTransferFailInput"].(map[string]any)["properties"].(map[string]any)["errorCode"].(map[string]any)["enum"])
	if !ok || slices.Contains(failCodes, "volume_transfer.completion_missing") {
		t.Fatalf("VolumeTransferFailInput.errorCode must not accept completion_missing: %#v", failCodes)
	}
}

func TestVolumeByteTransferProtocolIsNotExposedToAgent(t *testing.T) {
	for _, operationID := range []string{
		"createVolumeImport",
		"getVolumeImportUploadOffset",
		"uploadVolumeImportContent",
		"completeVolumeImportUpload",
		"authorizeVolumeTransferDownload",
		"headVolumeTransferContent",
		"downloadVolumeTransferContent",
		"headInternalVolumeTransferContent",
		"uploadInternalVolumeTransferContent",
		"downloadInternalVolumeTransferContent",
		"reportInternalVolumeTransferProgress",
		"completeInternalVolumeTransfer",
		"failInternalVolumeTransfer",
		"headVolumeTransferManifest",
		"downloadVolumeTransferManifest",
		"retryProjectVolumeOperation",
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
	for _, method := range []string{"head", "get"} {
		operation := manifestPath[method].(map[string]any)
		extension := operation["x-luna-cli"].(map[string]any)
		if extension["classification"] != "protocol-adapter" || extension["hidden"] != true || extension["agentAllowed"] != false {
			t.Fatalf("manifest %s CLI metadata = %#v", method, extension)
		}
		headers := operation["responses"].(map[string]any)["200"].(map[string]any)["headers"].(map[string]any)
		contentType := headers["Content-Type"].(map[string]any)["schema"].(map[string]any)
		if contentType["const"] != "application/json; charset=utf-8" {
			t.Fatalf("manifest %s Content-Type = %#v", method, contentType)
		}
		if _, exists := operation["responses"].(map[string]any)["422"]; !exists {
			t.Fatalf("manifest %s is missing checksum_invalid response", method)
		}
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

func TestVolumeMFAPurposesReplaceLegacyDeploymentExport(t *testing.T) {
	document := readOpenAPIDocument(t, apiRepositoryRoot(t)+"/openapi/openapi.yaml")
	schemas := document["components"].(map[string]any)["schemas"].(map[string]any)
	purposes, ok := schemaStringList(schemas["MFAPurpose"].(map[string]any)["enum"])
	if !ok {
		t.Fatalf("MFAPurpose.enum = %#v", schemas["MFAPurpose"])
	}
	for _, required := range []string{"billing_owner_transfer", "volume_import", "volume_export", "volume_adopt", "volume_delete"} {
		if !slices.Contains(purposes, required) {
			t.Fatalf("MFAPurpose is missing %s: %#v", required, purposes)
		}
	}
	if slices.Contains(purposes, "data_export") {
		t.Fatalf("MFAPurpose still exposes legacy data_export: %#v", purposes)
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

func TestVolumeDownloadOpenAPIUsesTransferBoundSessionCookie(t *testing.T) {
	document := readOpenAPIDocument(t, apiRepositoryRoot(t)+"/openapi/openapi.yaml")
	components := document["components"].(map[string]any)
	parameters := components["parameters"].(map[string]any)
	ticket := parameters["DownloadTicket"].(map[string]any)
	if required, _ := ticket["required"].(bool); required {
		t.Fatal("DownloadTicket must be optional after the initial download session exchange")
	}
	cookie := parameters["DownloadSessionCookie"].(map[string]any)
	if cookie["in"] != "cookie" || cookie["name"] != "luna_volume_download_session" {
		t.Fatalf("download session cookie contract = %#v", cookie)
	}

	paths := document["paths"].(map[string]any)
	contentPath := paths["/api/v1/projects/{projectId}/volume-transfers/{transferId}/content"].(map[string]any)
	for _, method := range []string{"head", "get"} {
		operation := contentPath[method].(map[string]any)
		operationParameters := operation["parameters"].([]any)
		refs := make(map[string]bool, len(operationParameters))
		for _, raw := range operationParameters {
			parameter := raw.(map[string]any)
			if ref, ok := parameter["$ref"].(string); ok {
				refs[ref] = true
			}
		}
		for _, ref := range []string{"#/components/parameters/DownloadTicket", "#/components/parameters/DownloadSessionCookie"} {
			if !refs[ref] {
				t.Fatalf("%s download contract is missing %s", method, ref)
			}
		}
		responses := operation["responses"].(map[string]any)
		if _, ok := responses["401"]; !ok {
			t.Fatalf("%s download contract is missing the stable unauthorized response", method)
		}
	}
}
