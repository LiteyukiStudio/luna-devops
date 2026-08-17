package kubernetes

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestWorkloadResourceSnapshotsDistinguishScaleToZero(t *testing.T) {
	replicas := int32(0)
	deployment := deploymentSnapshot(appsv1.Deployment{Spec: appsv1.DeploymentSpec{Replicas: &replicas}})
	if deployment.Status != "scaled-to-zero" {
		t.Fatalf("deployment status = %q, want scaled-to-zero", deployment.Status)
	}

	statefulSet := statefulSetSnapshot(appsv1.StatefulSet{Spec: appsv1.StatefulSetSpec{Replicas: &replicas}})
	if statefulSet.Status != "scaled-to-zero" {
		t.Fatalf("statefulset status = %q, want scaled-to-zero", statefulSet.Status)
	}

	deployment = deploymentSnapshot(appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Generation: 2}, Spec: appsv1.DeploymentSpec{Replicas: &replicas}, Status: appsv1.DeploymentStatus{ObservedGeneration: 1}})
	if deployment.Status != "progressing" {
		t.Fatalf("unobserved deployment status = %q, want progressing", deployment.Status)
	}
}
