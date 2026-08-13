package kubernetes

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestRetainManagedPersistentVolumeClaimTransfersOwnership(t *testing.T) {
	claim := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{
		Name: "app-data", Namespace: "project-demo",
		Labels: map[string]string{
			ManagedByLabel: ManagedByValue, ProjectIDLabel: "prj_demo",
			ApplicationIDLabel: "app_demo", EnvironmentIDLabel: "env_demo",
			DeploymentTargetIDLabel: "dplt_demo", ReleaseIDLabel: "rel_demo",
		},
	}}
	client := NewClientForInterface(fake.NewSimpleClientset(claim))
	if err := client.RetainManagedPersistentVolumeClaim(context.Background(), claim.Namespace, claim.Name, "dplt_demo", "rvol_demo"); err != nil {
		t.Fatalf("retain claim: %v", err)
	}
	retained, err := client.client.CoreV1().PersistentVolumeClaims(claim.Namespace).Get(context.Background(), claim.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get retained claim: %v", err)
	}
	if retained.Labels[RetainedVolumeIDLabel] != "rvol_demo" || retained.Labels[ManagedByLabel] != ManagedByValue || retained.Labels[ProjectIDLabel] != "prj_demo" {
		t.Fatalf("retained labels = %#v", retained.Labels)
	}
	for _, key := range []string{ApplicationIDLabel, EnvironmentIDLabel, DeploymentTargetIDLabel, ReleaseIDLabel} {
		if _, ok := retained.Labels[key]; ok {
			t.Fatalf("application ownership label %s was not removed: %#v", key, retained.Labels)
		}
	}
}
