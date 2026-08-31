package kubepolicy

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

type referenceResolver map[string]metav1.Object

func (resolver referenceResolver) ResolveMetadata(_ context.Context, reference ObjectReference) (metav1.Object, error) {
	return resolver[reference.GVR.Resource+"/"+reference.Name], nil
}

func boolPointer(value bool) *bool { return &value }

func deploymentObject(t *testing.T, spec corev1.PodSpec) *unstructured.Unstructured {
	t.Helper()
	deployment := &appsv1.Deployment{TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"}, ObjectMeta: metav1.ObjectMeta{Namespace: "project-a"}, Spec: appsv1.DeploymentSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{ProjectIDLabel: "p1"}}, Template: corev1.PodTemplateSpec{Spec: spec}}}
	value, err := runtime.DefaultUnstructuredConverter.ToUnstructured(deployment)
	if err != nil {
		t.Fatal(err)
	}
	return &unstructured.Unstructured{Object: value}
}

func TestWorkloadRejectsHostAndPrivilegeEscalation(t *testing.T) {
	object := deploymentObject(t, corev1.PodSpec{
		HostNetwork: true, AutomountServiceAccountToken: boolPointer(false),
		Containers: []corev1.Container{{Name: "app", SecurityContext: &corev1.SecurityContext{AllowPrivilegeEscalation: boolPointer(true)}}},
	})
	errors := NewValidator().Validate(t.Context(), PolicyContext{Namespace: "project-a", ProjectID: "p1", ManagementSource: ManagementSourceKubectl, ServiceAccountOrigin: ServiceAccountAbsent}, object)
	if len(errors) < 2 {
		t.Fatalf("expected host and privilege errors, got %#v", errors)
	}
}

func TestImplicitDefaultServiceAccountIsOnlyAllowedWithSafeOrigin(t *testing.T) {
	spec := corev1.PodSpec{
		ServiceAccountName: "default", AutomountServiceAccountToken: boolPointer(false),
		Containers: []corev1.Container{{Name: "app", SecurityContext: &corev1.SecurityContext{AllowPrivilegeEscalation: boolPointer(false)}}},
	}
	policy := PolicyContext{Namespace: "project-a", ProjectID: "p1", ManagementSource: ManagementSourceKubectl, ServiceAccountOrigin: ServiceAccountAbsent}
	if errors := NewValidator().Validate(t.Context(), policy, deploymentObject(t, spec)); len(errors) != 0 {
		t.Fatalf("implicit default should be allowed: %#v", errors)
	}
	policy.ServiceAccountOrigin = ServiceAccountExplicit
	if errors := NewValidator().Validate(t.Context(), policy, deploymentObject(t, spec)); len(errors) == 0 {
		t.Fatal("explicit default service account must be rejected")
	}
}

func TestInjectedImagePullSecretStillRequiresOwnership(t *testing.T) {
	spec := corev1.PodSpec{
		ServiceAccountName: "default", AutomountServiceAccountToken: boolPointer(false), ImagePullSecrets: []corev1.LocalObjectReference{{Name: "foreign"}},
		Containers: []corev1.Container{{Name: "app", SecurityContext: &corev1.SecurityContext{AllowPrivilegeEscalation: boolPointer(false)}}},
	}
	policy := PolicyContext{
		Namespace: "project-a", ProjectID: "p1", ApplicationID: "a1", ManagementSource: ManagementSourceKubectl,
		ServiceAccountOrigin: ServiceAccountAbsent,
		Resolver:             referenceResolver{"secrets/foreign": &metav1.PartialObjectMetadata{ObjectMeta: metav1.ObjectMeta{Namespace: "project-a", Labels: map[string]string{ManagedByLabel: ManagedByValue, ProjectIDLabel: "p1", ApplicationIDLabel: "a2"}}}},
	}
	if errors := NewValidator().Validate(t.Context(), policy, deploymentObject(t, spec)); len(errors) == 0 {
		t.Fatal("cross-application image pull secret must be rejected")
	}
}

func TestValidatorRequiresFinalPrivilegeEscalationDefault(t *testing.T) {
	spec := corev1.PodSpec{
		AutomountServiceAccountToken: boolPointer(false),
		Containers:                   []corev1.Container{{Name: "app"}},
	}
	policy := PolicyContext{Namespace: "project-a", ProjectID: "p1", ManagementSource: ManagementSourceKubectl, ServiceAccountOrigin: ServiceAccountAbsent}
	if errors := NewValidator().Validate(t.Context(), policy, deploymentObject(t, spec)); len(errors) == 0 {
		t.Fatal("final object without allowPrivilegeEscalation=false must be rejected")
	}
}
