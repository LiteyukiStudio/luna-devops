package kubernetes

import (
	"context"
	"errors"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func runtimeSelectionTestPod(name string, labels map[string]string, ready bool, createdAt time.Time) *corev1.Pod {
	status := corev1.ConditionFalse
	if ready {
		status = corev1.ConditionTrue
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ns-demo", Labels: labels, CreationTimestamp: metav1.NewTime(createdAt)},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}, {Name: "sidecar"}}},
		Status: corev1.PodStatus{
			Phase:             corev1.PodRunning,
			Conditions:        []corev1.PodCondition{{Type: corev1.PodReady, Status: status}},
			ContainerStatuses: []corev1.ContainerStatus{{Name: "app", RestartCount: 2}, {Name: "sidecar", RestartCount: 1}},
		},
	}
}

func runtimeSelectionLabels(targetID string, releaseID string) map[string]string {
	return map[string]string{ManagedByLabel: ManagedByValue, DeploymentTargetIDLabel: targetID, ReleaseIDLabel: releaseID}
}

func newRuntimeSelectionClient() *Client {
	now := time.Now()
	return NewClientForInterface(fake.NewSimpleClientset(
		runtimeSelectionTestPod("api-new-ready", runtimeSelectionLabels("dplt_api", "rel_new"), true, now),
		runtimeSelectionTestPod("api-old-crashing", runtimeSelectionLabels("dplt_api", "rel_old"), false, now.Add(-time.Hour)),
		runtimeSelectionTestPod("other-target", runtimeSelectionLabels("dplt_other", "rel_other"), true, now),
	))
}

func TestRuntimePodUsesRequestedPod(t *testing.T) {
	client := newRuntimeSelectionClient()
	pod, container, err := client.runtimePod(context.Background(), RuntimePodSelection{
		Namespace: "ns-demo", DeploymentTargetID: "dplt_api", Pod: "api-old-crashing", Container: "sidecar",
	})
	if err != nil {
		t.Fatalf("runtimePod returned error: %v", err)
	}
	if pod.Name != "api-old-crashing" || container != "sidecar" {
		t.Fatalf("selected pod/container = %s/%s", pod.Name, container)
	}
}

func TestRuntimePodRejectsPodOutsideDeploymentTarget(t *testing.T) {
	client := newRuntimeSelectionClient()
	for _, podName := range []string{"other-target", "missing-pod"} {
		_, _, err := client.runtimePod(context.Background(), RuntimePodSelection{Namespace: "ns-demo", DeploymentTargetID: "dplt_api", Pod: podName})
		if !errors.Is(err, ErrRuntimePodNotFound) {
			t.Fatalf("runtimePod(%s) error = %v, want ErrRuntimePodNotFound", podName, err)
		}
	}
}

func TestRuntimePodPrefersRequestedRelease(t *testing.T) {
	client := newRuntimeSelectionClient()
	pod, _, err := client.runtimePod(context.Background(), RuntimePodSelection{Namespace: "ns-demo", DeploymentTargetID: "dplt_api", PreferredReleaseID: "rel_old"})
	if err != nil {
		t.Fatalf("runtimePod returned error: %v", err)
	}
	if pod.Name != "api-old-crashing" {
		t.Fatalf("selected pod = %s, want the release pod even when it is not ready", pod.Name)
	}
	pod, _, err = client.runtimePod(context.Background(), RuntimePodSelection{Namespace: "ns-demo", DeploymentTargetID: "dplt_api"})
	if err != nil {
		t.Fatalf("runtimePod returned error: %v", err)
	}
	if pod.Name != "api-new-ready" {
		t.Fatalf("selected pod = %s, want the ready pod without a release preference", pod.Name)
	}
}

func TestRuntimePodsListsDeploymentTargetReplicas(t *testing.T) {
	client := newRuntimeSelectionClient()
	pods, err := client.RuntimePods(context.Background(), "ns-demo", "dplt_api", "rel_old")
	if err != nil {
		t.Fatalf("RuntimePods returned error: %v", err)
	}
	if len(pods) != 2 || pods[0].Name != "api-old-crashing" || pods[1].Name != "api-new-ready" {
		t.Fatalf("pods = %+v", pods)
	}
	first := pods[0]
	if first.ReleaseID != "rel_old" || first.Ready || first.RestartCount != 3 || len(first.Containers) != 2 || first.Phase != "Running" {
		t.Fatalf("snapshot = %+v", first)
	}
}
