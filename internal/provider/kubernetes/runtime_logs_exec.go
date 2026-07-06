package kubernetes

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

func (c *Client) RuntimeMetrics(ctx context.Context, options RuntimeMetricsOptions) (RuntimeMetricsSnapshot, error) {
	if c.dynamic == nil {
		return RuntimeMetricsSnapshot{Available: false, Reason: "metrics_unavailable", UpdatedAt: time.Now()}, nil
	}
	namespace := strings.TrimSpace(options.Namespace)
	deploymentTargetID := strings.TrimSpace(options.DeploymentTargetID)
	if namespace == "" {
		return RuntimeMetricsSnapshot{}, fmt.Errorf("resource namespace is required")
	}
	if deploymentTargetID == "" {
		return RuntimeMetricsSnapshot{}, fmt.Errorf("deployment target is required")
	}
	selector := strings.Join([]string{
		ManagedByLabel + "=" + ManagedByValue,
		DeploymentTargetIDLabel + "=" + deploymentTargetID,
		ScopeLabel + "!=build",
	}, ",")
	gvr := schema.GroupVersionResource{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "pods"}
	list, err := c.dynamic.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return RuntimeMetricsSnapshot{Available: false, Reason: "metrics_unavailable", UpdatedAt: time.Now()}, nil
	}
	snapshot := RuntimeMetricsSnapshot{Available: true, PodCount: len(list.Items), UpdatedAt: time.Now()}
	for _, item := range list.Items {
		containers, _, _ := unstructured.NestedSlice(item.Object, "containers")
		for _, rawContainer := range containers {
			container, ok := rawContainer.(map[string]any)
			if !ok {
				continue
			}
			usage, ok, _ := unstructured.NestedStringMap(container, "usage")
			if !ok {
				continue
			}
			snapshot.ContainerCount++
			if value := strings.TrimSpace(usage["cpu"]); value != "" {
				if quantity, err := resource.ParseQuantity(value); err == nil {
					snapshot.CPUUsageMilli += quantity.MilliValue()
				}
			}
			if value := strings.TrimSpace(usage["memory"]); value != "" {
				if quantity, err := resource.ParseQuantity(value); err == nil {
					snapshot.MemoryUsageBytes += quantity.Value()
				}
			}
		}
	}
	return snapshot, nil
}

func (c *Client) RuntimePodLogs(ctx context.Context, options RuntimePodLogsOptions) (RuntimePodLogsResult, error) {
	pod, container, err := c.runtimePod(ctx, options.Namespace, options.DeploymentTargetID, options.Container)
	if err != nil {
		return RuntimePodLogsResult{}, err
	}
	logOptions := &corev1.PodLogOptions{Container: container}
	if options.TailLines > 0 {
		logOptions.TailLines = &options.TailLines
	}
	stream, err := c.client.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, logOptions).Stream(ctx)
	if err != nil {
		return RuntimePodLogsResult{}, err
	}
	defer stream.Close()
	content, err := io.ReadAll(stream)
	if err != nil {
		return RuntimePodLogsResult{}, err
	}
	return RuntimePodLogsResult{Pod: pod.Name, Container: container, Content: string(content)}, nil
}

func (c *Client) RuntimeExec(ctx context.Context, options RuntimeExecOptions) (RuntimeExecResult, error) {
	if c.restConfig == nil {
		return RuntimeExecResult{}, fmt.Errorf("runtime exec requires a REST config")
	}
	command := strings.TrimSpace(options.Command)
	if command == "" {
		return RuntimeExecResult{}, fmt.Errorf("command is required")
	}
	pod, container, err := c.runtimePod(ctx, options.Namespace, options.DeploymentTargetID, options.Container)
	if err != nil {
		return RuntimeExecResult{}, err
	}
	req := c.client.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod.Name).
		Namespace(pod.Namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: container,
			Command:   []string{"/bin/sh", "-lc", command},
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)
	executor, err := remotecommand.NewSPDYExecutor(c.restConfig, "POST", req.URL())
	if err != nil {
		return RuntimeExecResult{}, err
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	streamErr := executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
		Tty:    false,
	})
	exitCode := 0
	if streamErr != nil {
		exitCode = 1
		if stderr.Len() > 0 {
			stderr.WriteByte('\n')
		}
		stderr.WriteString(streamErr.Error())
	}
	return RuntimeExecResult{
		Pod:       pod.Name,
		Container: container,
		Stdout:    stdout.String(),
		Stderr:    stderr.String(),
		ExitCode:  exitCode,
	}, nil
}

func (c *Client) RuntimeTerminal(ctx context.Context, options RuntimeTerminalOptions) error {
	if c.restConfig == nil {
		return fmt.Errorf("runtime terminal requires a REST config")
	}
	if options.Stdin == nil || options.Stdout == nil {
		return fmt.Errorf("runtime terminal streams are required")
	}
	pod, container, err := c.runtimePod(ctx, options.Namespace, options.DeploymentTargetID, options.Container)
	if err != nil {
		return err
	}
	req := c.client.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod.Name).
		Namespace(pod.Namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: container,
			Command:   []string{"/bin/sh"},
			Stdin:     true,
			Stdout:    true,
			Stderr:    true,
			TTY:       true,
		}, scheme.ParameterCodec)
	executor, err := remotecommand.NewSPDYExecutor(c.restConfig, "POST", req.URL())
	if err != nil {
		return err
	}
	return executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:             options.Stdin,
		Stdout:            options.Stdout,
		Stderr:            options.Stdout,
		Tty:               true,
		TerminalSizeQueue: options.SizeQueue,
	})
}

func (c *Client) PodTerminal(ctx context.Context, options PodTerminalOptions) error {
	if c.restConfig == nil {
		return fmt.Errorf("pod terminal requires a REST config")
	}
	if options.Stdin == nil || options.Stdout == nil {
		return fmt.Errorf("pod terminal streams are required")
	}
	pod, container, err := c.namedPod(ctx, options.Namespace, options.PodName, options.Container)
	if err != nil {
		return err
	}
	req := c.client.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod.Name).
		Namespace(pod.Namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: container,
			Command:   []string{"/bin/sh"},
			Stdin:     true,
			Stdout:    true,
			Stderr:    true,
			TTY:       true,
		}, scheme.ParameterCodec)
	executor, err := remotecommand.NewSPDYExecutor(c.restConfig, "POST", req.URL())
	if err != nil {
		return err
	}
	return executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:             options.Stdin,
		Stdout:            options.Stdout,
		Stderr:            options.Stdout,
		Tty:               true,
		TerminalSizeQueue: options.SizeQueue,
	})
}

func (c *Client) namedPod(ctx context.Context, namespace string, podName string, container string) (corev1.Pod, string, error) {
	namespace = strings.TrimSpace(namespace)
	podName = strings.TrimSpace(podName)
	if namespace == "" {
		return corev1.Pod{}, "", fmt.Errorf("resource namespace is required")
	}
	if podName == "" {
		return corev1.Pod{}, "", fmt.Errorf("pod name is required")
	}
	pod, err := c.client.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return corev1.Pod{}, "", err
	}
	selectedContainer, err := selectPodContainer(*pod, container)
	if err != nil {
		return corev1.Pod{}, "", err
	}
	return *pod, selectedContainer, nil
}

func (c *Client) runtimePod(ctx context.Context, namespace string, deploymentTargetID string, container string) (corev1.Pod, string, error) {
	namespace = strings.TrimSpace(namespace)
	deploymentTargetID = strings.TrimSpace(deploymentTargetID)
	if namespace == "" {
		return corev1.Pod{}, "", fmt.Errorf("resource namespace is required")
	}
	if deploymentTargetID == "" {
		return corev1.Pod{}, "", fmt.Errorf("deployment target is required")
	}
	selector := strings.Join([]string{
		ManagedByLabel + "=" + ManagedByValue,
		DeploymentTargetIDLabel + "=" + deploymentTargetID,
		ScopeLabel + "!=build",
	}, ",")
	pods, err := c.client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return corev1.Pod{}, "", err
	}
	if len(pods.Items) == 0 {
		return corev1.Pod{}, "", fmt.Errorf("runtime pod not found")
	}
	sort.Slice(pods.Items, func(left, right int) bool {
		leftReady := podReady(pods.Items[left])
		rightReady := podReady(pods.Items[right])
		if leftReady != rightReady {
			return leftReady
		}
		leftRunning := pods.Items[left].Status.Phase == corev1.PodRunning
		rightRunning := pods.Items[right].Status.Phase == corev1.PodRunning
		if leftRunning != rightRunning {
			return leftRunning
		}
		return pods.Items[left].CreationTimestamp.After(pods.Items[right].CreationTimestamp.Time)
	})
	pod := pods.Items[0]
	selectedContainer, err := selectPodContainer(pod, container)
	if err != nil {
		return corev1.Pod{}, "", err
	}
	return pod, selectedContainer, nil
}

func selectPodContainer(pod corev1.Pod, container string) (string, error) {
	selectedContainer := strings.TrimSpace(container)
	if selectedContainer == "" && len(pod.Spec.Containers) > 0 {
		selectedContainer = pod.Spec.Containers[0].Name
	}
	if selectedContainer == "" {
		return "", fmt.Errorf("runtime container not found")
	}
	for _, item := range pod.Spec.Containers {
		if item.Name == selectedContainer {
			return selectedContainer, nil
		}
	}
	return "", fmt.Errorf("runtime container %q not found", selectedContainer)
}
