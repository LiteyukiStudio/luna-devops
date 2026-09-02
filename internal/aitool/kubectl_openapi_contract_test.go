package aitool

import (
	"reflect"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/openapi"
	"sigs.k8s.io/yaml"
)

func TestKubectlManagementOpenAPIContract(t *testing.T) {
	document := kubectlOpenAPIDocument(t)
	paths := mapValue(document["paths"])
	want := map[string]map[string]struct {
		operationID string
		scopes      []string
	}{
		"/api/v1/kube-credentials": {
			"get":  {operationID: "listKubeCredentials", scopes: []string{"token:manage"}},
			"post": {operationID: "createKubeCredential", scopes: []string{"token:manage"}},
		},
		"/api/v1/kube-credentials/{credentialId}": {
			"delete": {operationID: "revokeKubeCredential", scopes: []string{"token:manage"}},
		},
		"/api/v1/kube-credentials/{credentialId}/bindings": {
			"get": {operationID: "listKubeCredentialBindings", scopes: []string{"token:manage"}},
		},
		"/api/v1/runtime/clusters/{clusterId}/kube-gateway": {
			"get": {operationID: "getRuntimeClusterKubeGateway", scopes: []string{"cluster:read"}},
			"put": {operationID: "updateRuntimeClusterKubeGateway", scopes: []string{"cluster:manage"}},
		},
		"/api/v1/runtime/clusters/kube-gateway-status": {
			"get": {operationID: "observeRuntimeClusterKubeGatewayStatus", scopes: []string{"cluster:read"}},
		},
	}

	for path, methods := range want {
		pathItem := mapValue(paths[path])
		if len(pathItem) == 0 {
			t.Errorf("missing kubectl management path %s", path)
			continue
		}
		for method, expected := range methods {
			operation := mapValue(pathItem[method])
			if got := stringValue(operation["operationId"]); got != expected.operationID {
				t.Errorf("%s %s operationId = %q, want %q", method, path, got, expected.operationID)
			}
			if got := stringArray(mapValue(operation["x-luna-cli"])["requiredScopes"]); !reflect.DeepEqual(got, expected.scopes) {
				t.Errorf("%s requiredScopes = %#v, want %#v", expected.operationID, got, expected.scopes)
			}
		}
	}

	credentialItem := mapValue(paths["/api/v1/kube-credentials/{credentialId}"])
	if _, exposesPlaintextLookup := credentialItem["get"]; exposesPlaintextLookup {
		t.Fatal("kubectl credentials must not expose a single-item plaintext or kubeconfig lookup")
	}
}

func TestKubectlOpenAPISensitiveAndProtocolBoundaries(t *testing.T) {
	document := kubectlOpenAPIDocument(t)
	paths := mapValue(document["paths"])
	for path := range paths {
		if strings.HasPrefix(path, "/kube/v1/bindings/") {
			t.Fatalf("special Kubernetes protocol route entered the platform OpenAPI: %s", path)
		}
	}

	createOperation := mapValue(mapValue(paths["/api/v1/kube-credentials"])["post"])
	cli := mapValue(createOperation["x-luna-cli"])
	if stringValue(cli["classification"]) != "protocol-adapter" || !boolValue(cli["hidden"]) {
		t.Fatalf("createKubeCredential must use the hidden dedicated protocol adapter: %#v", cli)
	}

	schemas := mapValue(mapValue(document["components"])["schemas"])
	createResult := mapValue(schemas["KubeCredentialCreateResult"])
	kubeconfig := mapValue(mapValue(createResult["properties"])["kubeconfig"])
	if !boolValue(kubeconfig["readOnly"]) || !boolValue(kubeconfig["x-luna-sensitive"]) {
		t.Fatalf("one-time kubeconfig is not marked as sensitive output: %#v", kubeconfig)
	}

	listPage := mapValue(schemas["PaginatedKubeCredentials"])
	if _, leaksKubeconfig := mapValue(listPage["properties"])["kubeconfig"]; leaksKubeconfig {
		t.Fatal("credential list schema leaks kubeconfig material")
	}

	for _, operationID := range []string{
		"createKubeCredential",
		"listKubeCredentials",
		"listKubeCredentialBindings",
		"revokeKubeCredential",
		"getRuntimeClusterKubeGateway",
		"updateRuntimeClusterKubeGateway",
	} {
		if strings.TrimSpace(agentDisabledOperations[operationID]) == "" {
			t.Errorf("missing stable Agent deny reason for %s", operationID)
		}
		if _, allowed := PlatformOperation(operationID); allowed {
			t.Errorf("kubectl boundary operation entered Agent catalog: %s", operationID)
		}
	}

	statusOperation, allowed := PlatformOperation("observeRuntimeClusterKubeGatewayStatus")
	if !allowed {
		t.Fatal("sanitized read-only kubectl gateway status observation is missing from the Agent catalog")
	}
	if statusOperation.RequiresApproval || !reflect.DeepEqual(statusOperation.RequiredScopes, []string{"cluster:read"}) {
		t.Fatalf("gateway status observation policy = %#v", statusOperation)
	}
}

func TestKubectlOpenAPIClosesLegacyDeploymentOverrides(t *testing.T) {
	document := kubectlOpenAPIDocument(t)
	schemas := mapValue(mapValue(document["components"])["schemas"])
	deploymentInput := mapValue(mapValue(schemas["DeploymentTargetInput"])["properties"])
	for _, field := range []string{
		"namespace",
		"allowPrivilegeEscalation",
		"capabilityAdd",
		"serviceAccountName",
		"automountServiceAccountToken",
		"serviceType",
		"serviceExternalTrafficPolicy",
	} {
		if _, exposed := deploymentInput[field]; exposed {
			t.Errorf("DeploymentTargetInput still exposes forbidden field %s", field)
		}
	}
	if _, exposed := mapValue(mapValue(schemas["DeploymentTargetBundleOverrides"])["properties"])["namespace"]; exposed {
		t.Fatal("deployment bundle overrides still expose namespace")
	}
	if _, exposed := mapValue(mapValue(schemas["AppTemplateInstallInput"])["properties"])["namespace"]; exposed {
		t.Fatal("app template install still exposes namespace")
	}
}

func TestKubectlOpenAPIRuntimeClusterLifecycleContract(t *testing.T) {
	document := kubectlOpenAPIDocument(t)
	paths := mapValue(document["paths"])
	deleteOperation := mapValue(mapValue(paths["/api/v1/runtime/clusters/{clusterId}"])["delete"])
	deleteResponses := mapValue(deleteOperation["responses"])
	for _, status := range []string{"204", "409", "503"} {
		if _, exists := deleteResponses[status]; !exists {
			t.Errorf("deleteRuntimeCluster is missing %s response", status)
		}
	}

	schemas := mapValue(mapValue(document["components"])["schemas"])
	runtimeCluster := mapValue(schemas["RuntimeCluster"])
	allOf := arrayValue(runtimeCluster["allOf"])
	if len(allOf) != 2 {
		t.Fatalf("RuntimeCluster allOf = %#v", allOf)
	}
	properties := mapValue(mapValue(allOf[1])["properties"])
	for _, field := range []string{"kubeGatewayEnabled", "deleteStatus", "deleteStartedAt", "deleteFinishedAt", "deleteObservationCode"} {
		if _, exists := properties[field]; !exists {
			t.Errorf("RuntimeCluster response is missing %s", field)
		}
	}
	for _, internalField := range []string{"kubeGatewayExtraResourceRules", "deleteMessage", "kubeGatewayDrainUntil", "kubeGatewayCleanupCompletedAt", "kubeconfigRef"} {
		if _, exposed := properties[internalField]; exposed {
			t.Errorf("RuntimeCluster response exposes internal field %s", internalField)
		}
	}

	features := mapValue(mapValue(mapValue(mapValue(mapValue(paths["/api/v1/meta"])["get"])["responses"])["200"])["content"])
	metaSchema := mapValue(mapValue(features["application/json"])["schema"])
	featureProperties := mapValue(mapValue(mapValue(metaSchema["properties"])["features"])["properties"])
	if _, exists := featureProperties["kubectlGateway"]; !exists {
		t.Fatal("API meta feature contract is missing kubectlGateway")
	}
}

func kubectlOpenAPIDocument(t *testing.T) map[string]any {
	t.Helper()
	document := make(map[string]any)
	if err := yaml.Unmarshal(openapi.SpecYAML, &document); err != nil {
		t.Fatalf("parse embedded OpenAPI: %v", err)
	}
	return document
}
