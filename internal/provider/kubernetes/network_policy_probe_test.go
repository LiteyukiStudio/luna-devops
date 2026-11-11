package kubernetes

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestProbeNetworkPolicyEnforcementWithoutImageIsUnknown(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	result, err := NewClientForInterface(clientset).ProbeNetworkPolicyEnforcement(context.Background(), " ", "clu_demo")
	if err != nil || result.Enforcement != NetworkPolicyEnforcementUnknown || result.Code != "runtime_cluster.network_policy_probe_image_missing" {
		t.Fatalf("result = %#v, %v", result, err)
	}
	if namespaces, _ := clientset.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{}); len(namespaces.Items) != 0 {
		t.Fatalf("probe without image must not create resources: %#v", namespaces.Items)
	}
}

func TestNetworkPolicyProbePodIsLeastPrivileged(t *testing.T) {
	pod := networkPolicyProbePod("ns", "server", "example.test/probe:1", SystemComponentLabels(networkPolicyProbeComponent, "clu_demo"), []string{"nc"})
	container := pod.Spec.Containers[0]
	if pod.Spec.AutomountServiceAccountToken == nil || *pod.Spec.AutomountServiceAccountToken ||
		pod.Spec.SecurityContext.RunAsNonRoot == nil || !*pod.Spec.SecurityContext.RunAsNonRoot ||
		container.SecurityContext.AllowPrivilegeEscalation == nil || *container.SecurityContext.AllowPrivilegeEscalation ||
		container.Resources.Limits.Memory().IsZero() {
		t.Fatalf("probe pod must be least privileged with limits: %#v", pod.Spec)
	}
	if pod.Labels[networkPolicyProbeRoleLabel] != "server" || pod.Labels[SystemComponentLabel] != networkPolicyProbeComponent {
		t.Fatalf("probe labels = %#v", pod.Labels)
	}
}

func TestNetworkPolicyProbePodFailureCodes(t *testing.T) {
	pull := &corev1.Pod{Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"}}}}}}
	if code := networkPolicyProbePodFailure(pull); code != "runtime_cluster.network_policy_probe_image_unavailable" {
		t.Fatalf("image pull code = %q", code)
	}
	unschedulable := &corev1.Pod{Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Reason: corev1.PodReasonUnschedulable}}}}
	if code := networkPolicyProbePodFailure(unschedulable); code != "runtime_cluster.network_policy_probe_unschedulable" {
		t.Fatalf("unschedulable code = %q", code)
	}
	if code := networkPolicyProbePodFailure(&corev1.Pod{}); code != "" {
		t.Fatalf("healthy pod code = %q", code)
	}
}

func TestProbeNetworkPolicyEnforcementFailsFastAndCleansUpWhenServerExits(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	// 服务端命令无法启动时 Pod 直接 Failed，探测应立即返回而不是等到超时。
	clientset.PrependReactor("create", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		pod := action.(k8stesting.CreateAction).GetObject().(*corev1.Pod)
		pod.Status.Phase = corev1.PodFailed
		return false, nil, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := NewClientForInterface(clientset).ProbeNetworkPolicyEnforcement(ctx, "example.test/probe:1", "clu_demo")
	if err != nil || result.Enforcement != NetworkPolicyEnforcementUnknown || result.Code != "runtime_cluster.network_policy_probe_pod_failed" {
		t.Fatalf("result = %#v, %v", result, err)
	}
	if namespaces, _ := clientset.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{}); len(namespaces.Items) != 0 {
		t.Fatalf("probe namespace must be deleted: %#v", namespaces.Items)
	}
}
