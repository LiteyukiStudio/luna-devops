package kubernetes

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestGetDeploymentSnapshotReadsSucceededDeployment(t *testing.T) {
	replicas := int32(2)
	client := NewClientForInterface(fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api-dev", Namespace: "project-demo", Generation: 3},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 3,
			UpdatedReplicas:    2,
			ReadyReplicas:      2,
			AvailableReplicas:  2,
			Conditions: []appsv1.DeploymentCondition{{
				Type:    appsv1.DeploymentAvailable,
				Status:  corev1.ConditionTrue,
				Message: "available",
			}},
		},
	}))

	snapshot, err := client.GetDeploymentSnapshot(context.Background(), "project-demo", "api-dev")
	if err != nil {
		t.Fatalf("GetDeploymentSnapshot returned error: %v", err)
	}
	if snapshot.Phase != DeploymentSucceeded || snapshot.Message != "available" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestDeploymentSnapshotWaitsUntilScaleToZeroCompletes(t *testing.T) {
	replicas := int32(0)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Generation: 3},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 3,
			Replicas:           1,
			UpdatedReplicas:    1,
			ReadyReplicas:      1,
			AvailableReplicas:  1,
			Conditions: []appsv1.DeploymentCondition{{
				Type:   appsv1.DeploymentProgressing,
				Status: corev1.ConditionFalse,
				Reason: "ProgressDeadlineExceeded",
			}},
		},
	}
	if snapshot := deploymentStatusSnapshot(deployment); snapshot.Phase != DeploymentRunning {
		t.Fatalf("phase while pod remains = %q, want %q", snapshot.Phase, DeploymentRunning)
	}
	deployment.Status.Replicas = 0
	deployment.Status.UpdatedReplicas = 0
	deployment.Status.ReadyReplicas = 0
	deployment.Status.AvailableReplicas = 0
	if snapshot := deploymentStatusSnapshot(deployment); snapshot.Phase != DeploymentSucceeded {
		t.Fatalf("phase after scale to zero = %q, want %q", snapshot.Phase, DeploymentSucceeded)
	}
}

func TestGetDeploymentSnapshotReadsProgressDeadlineFailure(t *testing.T) {
	replicas := int32(2)
	client := NewClientForInterface(fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api-dev", Namespace: "project-demo", Generation: 3},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 3,
			UpdatedReplicas:    1,
			Conditions: []appsv1.DeploymentCondition{{
				Type:    appsv1.DeploymentProgressing,
				Status:  corev1.ConditionFalse,
				Reason:  "ProgressDeadlineExceeded",
				Message: "timed out",
			}},
		},
	}))

	snapshot, err := client.GetDeploymentSnapshot(context.Background(), "project-demo", "api-dev")
	if err != nil {
		t.Fatalf("GetDeploymentSnapshot returned error: %v", err)
	}
	if snapshot.Phase != DeploymentFailed || snapshot.Message != "timed out" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestGetDeploymentSnapshotFallsBackToStatefulSet(t *testing.T) {
	replicas := int32(2)
	client := NewClientForInterface(fake.NewSimpleClientset(&appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "api-stateful", Namespace: "project-demo", Generation: 3},
		Spec:       appsv1.StatefulSetSpec{Replicas: &replicas},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 3,
			UpdatedReplicas:    2,
			ReadyReplicas:      2,
			AvailableReplicas:  2,
		},
	}))

	snapshot, err := client.GetDeploymentSnapshot(context.Background(), "project-demo", "api-stateful")
	if err != nil {
		t.Fatalf("GetDeploymentSnapshot returned error: %v", err)
	}
	if snapshot.Phase != DeploymentSucceeded || snapshot.Message != "StatefulSet rollout completed" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestStatefulSetSnapshotWaitsUntilScaleToZeroCompletes(t *testing.T) {
	replicas := int32(0)
	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "api-stateful", Namespace: "project-demo", Generation: 3},
		Spec:       appsv1.StatefulSetSpec{Replicas: &replicas},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 3,
			Replicas:           1,
			UpdatedReplicas:    1,
			ReadyReplicas:      1,
			AvailableReplicas:  1,
		},
	}
	client := NewClientForInterface(fake.NewSimpleClientset(statefulSet))
	snapshot, err := client.GetWorkloadSnapshot(context.Background(), "project-demo", "api-stateful", "StatefulSet")
	if err != nil {
		t.Fatalf("GetWorkloadSnapshot returned error: %v", err)
	}
	if snapshot.Phase != DeploymentRunning {
		t.Fatalf("phase while pod remains = %q, want %q", snapshot.Phase, DeploymentRunning)
	}
	statefulSet.Status.Replicas = 0
	statefulSet.Status.UpdatedReplicas = 0
	statefulSet.Status.ReadyReplicas = 0
	statefulSet.Status.AvailableReplicas = 0
	if _, err := client.client.AppsV1().StatefulSets("project-demo").UpdateStatus(context.Background(), statefulSet, metav1.UpdateOptions{}); err != nil {
		t.Fatalf("UpdateStatus returned error: %v", err)
	}
	snapshot, err = client.GetWorkloadSnapshot(context.Background(), "project-demo", "api-stateful", "StatefulSet")
	if err != nil {
		t.Fatalf("GetWorkloadSnapshot returned error: %v", err)
	}
	if snapshot.Phase != DeploymentSucceeded {
		t.Fatalf("phase after scale to zero = %q, want %q", snapshot.Phase, DeploymentSucceeded)
	}
}

func TestRestartDeploymentUpdatesPodTemplateAnnotation(t *testing.T) {
	client := NewClientForInterface(fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api-dev", Namespace: "project-demo"},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"existing": "keep"}},
			},
		},
	}))

	if err := client.RestartDeployment(context.Background(), "project-demo", "api-dev"); err != nil {
		t.Fatalf("RestartDeployment returned error: %v", err)
	}
	deployment, err := client.client.AppsV1().Deployments("project-demo").Get(context.Background(), "api-dev", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get deployment returned error: %v", err)
	}
	if deployment.Spec.Template.Annotations["existing"] != "keep" {
		t.Fatalf("existing annotation was not preserved: %#v", deployment.Spec.Template.Annotations)
	}
	if deployment.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] == "" {
		t.Fatalf("restart annotation was not set: %#v", deployment.Spec.Template.Annotations)
	}
}

func TestRestartDeploymentFallsBackToStatefulSet(t *testing.T) {
	client := NewClientForInterface(fake.NewSimpleClientset(&appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "api-stateful", Namespace: "project-demo"},
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"existing": "keep"}},
			},
		},
	}))

	if err := client.RestartDeployment(context.Background(), "project-demo", "api-stateful"); err != nil {
		t.Fatalf("RestartDeployment returned error: %v", err)
	}
	statefulSet, err := client.client.AppsV1().StatefulSets("project-demo").Get(context.Background(), "api-stateful", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get statefulset returned error: %v", err)
	}
	if statefulSet.Spec.Template.Annotations["existing"] != "keep" {
		t.Fatalf("existing annotation was not preserved: %#v", statefulSet.Spec.Template.Annotations)
	}
	if statefulSet.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] == "" {
		t.Fatalf("restart annotation was not set: %#v", statefulSet.Spec.Template.Annotations)
	}
}
