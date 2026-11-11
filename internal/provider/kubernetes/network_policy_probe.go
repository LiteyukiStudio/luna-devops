package kubernetes

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	NetworkPolicyEnforcementVerified    = "verified"
	NetworkPolicyEnforcementUnsupported = "unsupported"
	NetworkPolicyEnforcementUnknown     = "unknown"

	networkPolicyProbeComponent = "network-policy-probe"
	networkPolicyProbePort      = 8080
	networkPolicyProbeStaleAge  = time.Hour
)

// NetworkPolicyProbeResult 是一次主动探测的结论；Code 为空表示探测完成且结论可信。
type NetworkPolicyProbeResult struct {
	Enforcement string
	Code        string
}

// ProbeNetworkPolicyEnforcement 在临时命名空间中验证集群是否真正执行 NetworkPolicy：
// 先确认客户端能连通服务端，再对服务端下发拒绝全部入站的策略，确认连接被拦截。
// 无法得出结论（镜像拉取失败、基线不通、超时等）时返回 unknown 与稳定 Code，不返回 error。
func (c *Client) ProbeNetworkPolicyEnforcement(ctx context.Context, image string, clusterID string) (NetworkPolicyProbeResult, error) {
	if strings.TrimSpace(image) == "" {
		return NetworkPolicyProbeResult{Enforcement: NetworkPolicyEnforcementUnknown, Code: "runtime_cluster.network_policy_probe_image_missing"}, nil
	}
	c.deleteStaleNetworkPolicyProbes(ctx)
	suffix := make([]byte, 4)
	if _, err := rand.Read(suffix); err != nil {
		return NetworkPolicyProbeResult{}, err
	}
	namespace := "luna-netpol-probe-" + hex.EncodeToString(suffix)
	labels := SystemComponentLabels(networkPolicyProbeComponent, clusterID)
	if _, err := c.client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace, Labels: labels}}, metav1.CreateOptions{}); err != nil {
		return NetworkPolicyProbeResult{}, err
	}
	defer func() {
		// 清理不受探测超时影响，但保留 Trace 上下文。
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		_ = c.client.CoreV1().Namespaces().Delete(cleanupCtx, namespace, metav1.DeleteOptions{})
	}()

	// busybox nc -lk 不能可靠地接受多次连接，用循环保证每次探测都有监听。
	server := networkPolicyProbePod(namespace, "server", image, labels, []string{"sh", "-c", fmt.Sprintf("while true; do nc -l -p %d </dev/null >/dev/null 2>&1; done", networkPolicyProbePort)})
	if err := ValidatePodIsolation(server.Spec); err != nil {
		return NetworkPolicyProbeResult{}, err
	}
	if _, err := c.client.CoreV1().Pods(namespace).Create(ctx, server, metav1.CreateOptions{}); err != nil {
		return NetworkPolicyProbeResult{}, err
	}
	serverIP, code, err := c.waitNetworkPolicyProbeServer(ctx, namespace)
	if err != nil || code != "" {
		return NetworkPolicyProbeResult{Enforcement: NetworkPolicyEnforcementUnknown, Code: code}, err
	}

	// 基线：未下发策略时必须能连通，否则无法区分“策略生效”与“网络本身不通”。
	connect := fmt.Sprintf("for i in 1 2 3 4 5 6 7 8 9 10; do nc -z -w 2 %s %d && exit 0; sleep 1; done; exit 1", serverIP, networkPolicyProbePort)
	reachable, code, err := c.runNetworkPolicyProbeClient(ctx, namespace, "baseline", image, labels, connect)
	if err != nil || code != "" {
		return NetworkPolicyProbeResult{Enforcement: NetworkPolicyEnforcementUnknown, Code: code}, err
	}
	if !reachable {
		return NetworkPolicyProbeResult{Enforcement: NetworkPolicyEnforcementUnknown, Code: "runtime_cluster.network_policy_probe_baseline_failed"}, nil
	}

	// 服务端只放行对照客户端：被隔离客户端连不上且对照客户端连得上，才说明是策略生效而不是服务端或网络故障。
	deny := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "server-ingress", Namespace: namespace, Labels: labels},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{networkPolicyProbeRoleLabel: "server"}},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress: []networkingv1.NetworkPolicyIngressRule{{From: []networkingv1.NetworkPolicyPeer{{
				PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{networkPolicyProbeRoleLabel: "control"}},
			}}}},
		},
	}
	if _, err := c.client.NetworkingV1().NetworkPolicies(namespace).Create(ctx, deny, metav1.CreateOptions{}); err != nil {
		return NetworkPolicyProbeResult{}, err
	}
	// 等待策略传播后连续尝试；任意一次连通即视为未执行。
	isolated := fmt.Sprintf("sleep 8; for i in 1 2 3; do nc -z -w 2 %s %d && exit 1; sleep 1; done; exit 0", serverIP, networkPolicyProbePort)
	blocked, code, err := c.runNetworkPolicyProbeClient(ctx, namespace, "isolated", image, labels, isolated)
	if err != nil || code != "" {
		return NetworkPolicyProbeResult{Enforcement: NetworkPolicyEnforcementUnknown, Code: code}, err
	}
	if !blocked {
		return NetworkPolicyProbeResult{Enforcement: NetworkPolicyEnforcementUnsupported}, nil
	}
	controlReachable, code, err := c.runNetworkPolicyProbeClient(ctx, namespace, "control", image, labels, connect)
	if err != nil || code != "" {
		return NetworkPolicyProbeResult{Enforcement: NetworkPolicyEnforcementUnknown, Code: code}, err
	}
	if !controlReachable {
		return NetworkPolicyProbeResult{Enforcement: NetworkPolicyEnforcementUnknown, Code: "runtime_cluster.network_policy_probe_baseline_failed"}, nil
	}
	return NetworkPolicyProbeResult{Enforcement: NetworkPolicyEnforcementVerified}, nil
}

const networkPolicyProbeRoleLabel = "luna.devops/network-policy-probe-role"

func networkPolicyProbePod(namespace, role, image string, labels map[string]string, command []string) *corev1.Pod {
	podLabels := mergeStringMaps(labels, map[string]string{networkPolicyProbeRoleLabel: role})
	falseValue := false
	trueValue := true
	user := int64(65534)
	quantity := resource.MustParse
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "probe-" + role, Namespace: namespace, Labels: podLabels},
		Spec: corev1.PodSpec{
			RestartPolicy:                corev1.RestartPolicyNever,
			AutomountServiceAccountToken: &falseValue,
			SecurityContext: &corev1.PodSecurityContext{
				RunAsNonRoot:   &trueValue,
				RunAsUser:      &user,
				RunAsGroup:     &user,
				SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
			},
			Containers: []corev1.Container{{
				Name:            "probe",
				Image:           image,
				ImagePullPolicy: corev1.PullIfNotPresent,
				Command:         command,
				SecurityContext: &corev1.SecurityContext{
					AllowPrivilegeEscalation: &falseValue,
					ReadOnlyRootFilesystem:   &trueValue,
					Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
				},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceCPU: quantity("10m"), corev1.ResourceMemory: quantity("16Mi")},
					Limits:   corev1.ResourceList{corev1.ResourceCPU: quantity("100m"), corev1.ResourceMemory: quantity("32Mi")},
				},
			}},
		},
	}
}

func (c *Client) waitNetworkPolicyProbeServer(ctx context.Context, namespace string) (string, string, error) {
	for {
		pod, err := c.client.CoreV1().Pods(namespace).Get(ctx, "probe-server", metav1.GetOptions{})
		if err != nil {
			return "", "", err
		}
		if code := networkPolicyProbePodFailure(pod); code != "" {
			return "", code, nil
		}
		// RestartPolicy=Never：命令无法启动或服务端退出时 Pod 直接终止，不能等到超时。
		if pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodSucceeded {
			return "", "runtime_cluster.network_policy_probe_pod_failed", nil
		}
		if pod.Status.Phase == corev1.PodRunning && pod.Status.PodIP != "" {
			return pod.Status.PodIP, "", nil
		}
		if err := sleepContext(ctx, time.Second); err != nil {
			return "", "runtime_cluster.network_policy_probe_timeout", nil
		}
	}
}

// runNetworkPolicyProbeClient 返回客户端是否以 0 退出。
func (c *Client) runNetworkPolicyProbeClient(ctx context.Context, namespace, role, image string, labels map[string]string, script string) (bool, string, error) {
	pod := networkPolicyProbePod(namespace, role, image, labels, []string{"sh", "-c", script})
	if err := ValidatePodIsolation(pod.Spec); err != nil {
		return false, "", err
	}
	if _, err := c.client.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		return false, "", err
	}
	for {
		current, err := c.client.CoreV1().Pods(namespace).Get(ctx, pod.Name, metav1.GetOptions{})
		if err != nil {
			return false, "", err
		}
		switch current.Status.Phase {
		case corev1.PodSucceeded:
			return true, "", nil
		case corev1.PodFailed:
			return false, "", nil
		}
		if code := networkPolicyProbePodFailure(current); code != "" {
			return false, code, nil
		}
		if err := sleepContext(ctx, time.Second); err != nil {
			return false, "runtime_cluster.network_policy_probe_timeout", nil
		}
	}
}

func networkPolicyProbePodFailure(pod *corev1.Pod) string {
	for _, status := range pod.Status.ContainerStatuses {
		if waiting := status.State.Waiting; waiting != nil {
			switch waiting.Reason {
			case "ErrImagePull", "ImagePullBackOff", "InvalidImageName":
				return "runtime_cluster.network_policy_probe_image_unavailable"
			case "CreateContainerConfigError", "CreateContainerError", "RunContainerError":
				return "runtime_cluster.network_policy_probe_pod_failed"
			}
		}
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodScheduled && condition.Status == corev1.ConditionFalse && condition.Reason == corev1.PodReasonUnschedulable {
			return "runtime_cluster.network_policy_probe_unschedulable"
		}
	}
	return ""
}

// deleteStaleNetworkPolicyProbes 清理上次进程中断后残留的探测命名空间。
func (c *Client) deleteStaleNetworkPolicyProbes(ctx context.Context) {
	list, err := c.client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{LabelSelector: SystemComponentLabel + "=" + networkPolicyProbeComponent})
	if err != nil {
		return
	}
	for _, item := range list.Items {
		if time.Since(item.CreationTimestamp.Time) > networkPolicyProbeStaleAge {
			if err := c.client.CoreV1().Namespaces().Delete(ctx, item.Name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
				continue
			}
		}
	}
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
