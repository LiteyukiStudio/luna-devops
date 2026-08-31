package kubeproxy

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/kubepolicy"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestMutatorInjectsControllerAndPodTemplateOwnership(t *testing.T) {
	body := `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"app"},"spec":{"selector":{"matchLabels":{"app":"app"}},"template":{"metadata":{"labels":{"app":"app"}},"spec":{"containers":[{"name":"app","image":"example/app","securityContext":{"allowPrivilegeEscalation":false}}]}}}}`
	access := baseAccess()
	access.ApplicationID = "a1"
	result, err := NewMutator().Prepare(t.Context(), MutationContext{Access: access, Info: RequestInfo{Verb: "create", Namespace: access.Namespace}}, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(result.Body, &object); err != nil {
		t.Fatal(err)
	}
	for _, path := range [][]string{{"metadata", "labels"}, {"spec", "template", "metadata", "labels"}} {
		value := object
		for _, segment := range path {
			value = value[segment].(map[string]any)
		}
		if value[kubepolicy.ProjectIDLabel] != "p1" || value[kubepolicy.ApplicationIDLabel] != "a1" || value[kubepolicy.ManagementSourceLabel] != "kubectl" {
			t.Fatalf("missing ownership at %v: %#v", path, value)
		}
	}
	spec := object["spec"].(map[string]any)["template"].(map[string]any)["spec"].(map[string]any)
	if value, ok := spec["automountServiceAccountToken"].(bool); !ok || value {
		t.Fatalf("automount token was not forced off: %#v", spec)
	}
}

func TestMutatorRejectsReservedJSONPatch(t *testing.T) {
	patch := `[{"op":"replace","path":"/metadata/labels/luna.devops~1project-id","value":"other"}]`
	_, err := NewMutator().Prepare(t.Context(), MutationContext{Access: baseAccess(), Info: RequestInfo{Verb: "patch"}}, "application/json-patch+json", strings.NewReader(patch))
	if err == nil {
		t.Fatal("reserved label patch must be rejected")
	}
}

func TestMutatorBoundsRequestBody(t *testing.T) {
	mutator := NewMutator()
	mutator.MaxBodyBytes = 4
	_, err := mutator.Prepare(t.Context(), MutationContext{Access: baseAccess()}, "application/json", bytes.NewBufferString("12345"))
	if err == nil {
		t.Fatal("oversized body must be rejected")
	}
}

func TestMutatorFinalValidationRejectsAdmissionOwnershipLoss(t *testing.T) {
	body := `{"apiVersion":"v1","kind":"Pod","metadata":{"name":"app"},"spec":{"containers":[{"name":"app","image":"example/app","securityContext":{"allowPrivilegeEscalation":false}}]}}`
	mutator := NewMutator()
	result, err := mutator.Prepare(t.Context(), MutationContext{Access: baseAccess(), Info: RequestInfo{Verb: "create", Namespace: "project-a"}}, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(result.Body, &object); err != nil {
		t.Fatal(err)
	}
	labels := object["metadata"].(map[string]any)["labels"].(map[string]any)
	delete(labels, kubepolicy.ManagementSourceLabel)
	final, _ := json.Marshal(object)
	if err := mutator.ValidateFinal(t.Context(), result.PolicyContext, final); err == nil {
		t.Fatal("final admission object without reserved ownership must be rejected")
	}
}

func TestMutatorPreservesPlatformManagementSourceOnUpdate(t *testing.T) {
	body := `{"apiVersion":"v1","kind":"Pod","metadata":{"name":"app","namespace":"project-a","labels":{"app.kubernetes.io/managed-by":"luna-devops","luna.devops/project-id":"p1","luna.devops/management-source":"platform"}},"spec":{"automountServiceAccountToken":false,"containers":[{"name":"app","image":"example/app","securityContext":{"allowPrivilegeEscalation":false}}]}}`
	mutator := NewMutator()
	result, err := mutator.Prepare(t.Context(), MutationContext{
		Access: baseAccess(), Info: RequestInfo{Verb: "update", Namespace: "project-a"},
		ExistingLabels: map[string]string{kubepolicy.ManagedByLabel: kubepolicy.ManagedByValue, kubepolicy.ProjectIDLabel: "p1", kubepolicy.ManagementSourceLabel: string(kubepolicy.ManagementSourcePlatform)},
	}, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if result.PolicyContext.ManagementSource != kubepolicy.ManagementSourcePlatform {
		t.Fatalf("platform source was not preserved: %q", result.PolicyContext.ManagementSource)
	}
	if err := mutator.ValidateFinal(t.Context(), result.PolicyContext, result.Body); err != nil {
		t.Fatalf("valid platform update failed final validation: %v", err)
	}
}

func TestMutatorPreservesBinaryObjectContentTypes(t *testing.T) {
	allowPrivilegeEscalation := false
	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"}, ObjectMeta: metav1.ObjectMeta{Name: "app"},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "example/app", SecurityContext: &corev1.SecurityContext{AllowPrivilegeEscalation: &allowPrivilegeEscalation}}}},
	}
	for _, contentType := range []string{runtime.ContentTypeProtobuf, runtime.ContentTypeCBOR} {
		t.Run(contentType, func(t *testing.T) {
			body, _, err := EncodeNegotiatedObject(contentType, pod)
			if err != nil {
				t.Fatal(err)
			}
			result, err := NewMutator().Prepare(t.Context(), MutationContext{Access: baseAccess(), Info: RequestInfo{Verb: "create", Namespace: "project-a"}}, contentType, bytes.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			object, err := decodeMutationObject(contentType, result.Body)
			if err != nil {
				t.Fatal(err)
			}
			metadata := object["metadata"].(map[string]any)
			labels := metadata["labels"].(map[string]any)
			if metadata["namespace"] != "project-a" || labels[kubepolicy.ProjectIDLabel] != "p1" {
				t.Fatalf("binary mutation lost ownership: %#v", metadata)
			}
		})
	}
}

func TestMutatorDefaultsKubectlRunContainerSecurity(t *testing.T) {
	body := `{"apiVersion":"v1","kind":"Pod","metadata":{"name":"app"},"spec":{"initContainers":[{"name":"init","image":"example/init"}],"containers":[{"name":"app","image":"example/app"}],"ephemeralContainers":[{"name":"debug","image":"example/debug"}]}}`
	mutator := NewMutator()
	result, err := mutator.Prepare(t.Context(), MutationContext{Access: baseAccess(), Info: RequestInfo{Verb: "create", Namespace: "project-a"}}, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(result.Body, &object); err != nil {
		t.Fatal(err)
	}
	spec := object["spec"].(map[string]any)
	for _, fieldName := range []string{"initContainers", "containers", "ephemeralContainers"} {
		container := spec[fieldName].([]any)[0].(map[string]any)
		security := container["securityContext"].(map[string]any)
		if value, ok := security["allowPrivilegeEscalation"].(bool); !ok || value {
			t.Fatalf("%s was not safely defaulted: %#v", fieldName, security)
		}
	}
}

func TestMutatorKeepsExplicitPrivilegeEscalationForValidatorToReject(t *testing.T) {
	body := `{"apiVersion":"v1","kind":"Pod","metadata":{"name":"app"},"spec":{"containers":[{"name":"app","image":"example/app","securityContext":{"allowPrivilegeEscalation":true}}]}}`
	mutator := NewMutator()
	result, err := mutator.Prepare(t.Context(), MutationContext{Access: baseAccess(), Info: RequestInfo{Verb: "create", Namespace: "project-a"}}, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if err := mutator.ValidateFinal(t.Context(), result.PolicyContext, result.Body); err == nil {
		t.Fatal("explicit privilege escalation must remain visible to final validation")
	}
}

func TestProjectBindingCannotForgeApplicationOwnership(t *testing.T) {
	body := `{"apiVersion":"v1","kind":"Pod","metadata":{"name":"app","labels":{"luna.devops/application-id":"foreign"}},"spec":{"containers":[{"name":"app","image":"example/app"}]}}`
	if _, err := NewMutator().Prepare(t.Context(), MutationContext{Access: baseAccess(), Info: RequestInfo{Verb: "create", Namespace: "project-a"}}, "application/json", strings.NewReader(body)); err == nil {
		t.Fatal("project binding must not assign an application ownership label on create")
	}
	patch := `{"metadata":{"labels":{"luna.devops/application-id":"foreign"}}}`
	if _, err := NewMutator().Prepare(t.Context(), MutationContext{Access: baseAccess(), Info: RequestInfo{Verb: "patch", Namespace: "project-a"}}, "application/merge-patch+json", strings.NewReader(patch)); err == nil {
		t.Fatal("project binding must not assign an application ownership label by merge patch")
	}
}

func TestProjectBindingPreservesExistingApplicationSelector(t *testing.T) {
	body := `{"apiVersion":"v1","kind":"Service","metadata":{"name":"app","namespace":"project-a","labels":{"app.kubernetes.io/managed-by":"luna-devops","luna.devops/project-id":"p1","luna.devops/application-id":"a1","luna.devops/management-source":"kubectl"}},"spec":{"type":"ClusterIP","selector":{"luna.devops/project-id":"p1"},"ports":[{"port":80}]}}`
	mutator := NewMutator()
	result, err := mutator.Prepare(t.Context(), MutationContext{
		Access: baseAccess(), Info: RequestInfo{Verb: "update", Namespace: "project-a"},
		ExistingLabels: map[string]string{kubepolicy.ManagedByLabel: kubepolicy.ManagedByValue, kubepolicy.ProjectIDLabel: "p1", kubepolicy.ApplicationIDLabel: "a1", kubepolicy.ManagementSourceLabel: string(kubepolicy.ManagementSourceKubectl)},
	}, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(result.Body, &object); err != nil {
		t.Fatal(err)
	}
	selector := object["spec"].(map[string]any)["selector"].(map[string]any)
	if selector[kubepolicy.ApplicationIDLabel] != "a1" {
		t.Fatalf("existing application selector was not preserved: %#v", selector)
	}
	if err := mutator.ValidateFinal(t.Context(), result.PolicyContext, result.Body); err != nil {
		t.Fatalf("preserved application selector failed validation: %v", err)
	}
}

func TestMutatorRejectsForgedPlatformLifecycleLabels(t *testing.T) {
	for _, key := range kubepolicy.ProtectedLifecycleLabelKeys() {
		t.Run(key, func(t *testing.T) {
			body := `{"apiVersion":"v1","kind":"Pod","metadata":{"name":"app","labels":{` + strconv.Quote(key) + `:"forged"}},"spec":{"containers":[{"name":"app","image":"example/app"}]}}`
			if _, err := NewMutator().Prepare(t.Context(), MutationContext{Access: baseAccess(), Info: RequestInfo{Verb: "create", Namespace: "project-a"}}, "application/json", strings.NewReader(body)); err == nil {
				t.Fatalf("platform lifecycle label %s must not be assignable on create", key)
			}
		})
	}
}

func TestMutatorPreservesPlatformLifecycleLabelsOnReplaceAndApply(t *testing.T) {
	existing := map[string]string{
		kubepolicy.ManagedByLabel: kubepolicy.ManagedByValue, kubepolicy.ProjectIDLabel: "p1",
		kubepolicy.ApplicationIDLabel: "a1", kubepolicy.ManagementSourceLabel: string(kubepolicy.ManagementSourcePlatform),
		kubepolicy.DeploymentTargetIDLabel: "target-1", kubepolicy.ReleaseIDLabel: "release-1",
		kubepolicy.EnvironmentIDLabel: "environment-1", kubepolicy.GatewayRouteIDLabel: "route-1",
		kubepolicy.ProjectVolumeIDLabel: "volume-1",
	}
	body := `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"app"},"spec":{"selector":{"matchLabels":{"app":"app"}},"template":{"metadata":{"labels":{"app":"app"}},"spec":{"containers":[{"name":"app","image":"example/app"}]}}}}`
	for _, test := range []struct {
		name        string
		verb        string
		contentType string
	}{
		{name: "replace", verb: "update", contentType: "application/json"},
		{name: "server-side-apply", verb: "patch", contentType: "application/apply-patch+yaml"},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := NewMutator().Prepare(t.Context(), MutationContext{
				Access: baseAccess(), Info: RequestInfo{Verb: test.verb, Namespace: "project-a", IsApplyPatch: test.verb == "patch"}, ExistingLabels: existing,
			}, test.contentType, strings.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			object, err := decodeMutationObject(test.contentType, result.Body)
			if err != nil {
				t.Fatal(err)
			}
			metadata := object["metadata"].(map[string]any)["labels"].(map[string]any)
			template := object["spec"].(map[string]any)["template"].(map[string]any)["metadata"].(map[string]any)["labels"].(map[string]any)
			for _, key := range kubepolicy.ProtectedLifecycleLabelKeys() {
				if metadata[key] != existing[key] || template[key] != existing[key] {
					t.Fatalf("lifecycle label %s was not preserved: metadata=%#v template=%#v", key, metadata, template)
				}
			}
			if err := NewMutator().ValidateFinal(t.Context(), result.PolicyContext, result.Body); err != nil {
				t.Fatalf("preserved lifecycle labels failed final validation: %v", err)
			}
		})
	}
}

func TestMutatorRejectsLifecycleLabelDeletionAndAdmissionForgery(t *testing.T) {
	existing := map[string]string{
		kubepolicy.ManagedByLabel: kubepolicy.ManagedByValue, kubepolicy.ProjectIDLabel: "p1",
		kubepolicy.ManagementSourceLabel: string(kubepolicy.ManagementSourcePlatform), kubepolicy.DeploymentTargetIDLabel: "target-1",
	}
	patch := `{"metadata":{"labels":{"luna.devops/deployment-target-id":null}}}`
	if _, err := NewMutator().Prepare(t.Context(), MutationContext{Access: baseAccess(), Info: RequestInfo{Verb: "patch", Namespace: "project-a"}, ExistingLabels: existing}, "application/merge-patch+json", strings.NewReader(patch)); err == nil {
		t.Fatal("merge patch must not delete a platform lifecycle label")
	}

	body := `{"apiVersion":"v1","kind":"Pod","metadata":{"name":"app"},"spec":{"containers":[{"name":"app","image":"example/app"}]}}`
	mutator := NewMutator()
	result, err := mutator.Prepare(t.Context(), MutationContext{Access: baseAccess(), Info: RequestInfo{Verb: "create", Namespace: "project-a"}}, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(result.Body, &object); err != nil {
		t.Fatal(err)
	}
	object["metadata"].(map[string]any)["labels"].(map[string]any)[kubepolicy.DeploymentTargetIDLabel] = "admission-forged"
	final, _ := json.Marshal(object)
	if err := mutator.ValidateFinal(t.Context(), result.PolicyContext, final); err == nil {
		t.Fatal("final admission object must not acquire a platform lifecycle label")
	}
}
