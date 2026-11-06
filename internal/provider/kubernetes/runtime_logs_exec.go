package kubernetes

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
	clientexec "k8s.io/client-go/util/exec"
)

type runtimeExecOutput struct {
	mu        sync.Mutex
	stdout    bytes.Buffer
	stderr    bytes.Buffer
	remaining int
	truncated bool
}

type runtimeExecOutputWriter struct {
	output *runtimeExecOutput
	stderr bool
}

func newRuntimeExecOutput(limit int) *runtimeExecOutput {
	return &runtimeExecOutput{remaining: limit}
}

func (output *runtimeExecOutput) writer(stderr bool) io.Writer {
	return runtimeExecOutputWriter{output: output, stderr: stderr}
}

func (writer runtimeExecOutputWriter) Write(data []byte) (int, error) {
	if writer.output == nil {
		return len(data), nil
	}
	writer.output.mu.Lock()
	defer writer.output.mu.Unlock()
	written := len(data)
	if len(data) > writer.output.remaining {
		data = data[:max(writer.output.remaining, 0)]
		writer.output.truncated = true
	}
	writer.output.remaining -= len(data)
	if writer.stderr {
		_, _ = writer.output.stderr.Write(data)
	} else {
		_, _ = writer.output.stdout.Write(data)
	}
	return written, nil
}

func (output *runtimeExecOutput) snapshot() (stdout, stderr string, truncated bool) {
	output.mu.Lock()
	defer output.mu.Unlock()
	return output.stdout.String(), output.stderr.String(), output.truncated
}

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
	workloadName := strings.TrimSpace(options.WorkloadName)
	if workloadName == "" {
		return RuntimeMetricsSnapshot{Available: false, Reason: "workload_unavailable", UpdatedAt: time.Now()}, nil
	}
	workload, err := c.GetWorkloadSnapshot(ctx, namespace, workloadName, options.WorkloadType)
	if err != nil {
		return RuntimeMetricsSnapshot{Available: false, Reason: "workload_unavailable", UpdatedAt: time.Now()}, nil
	}
	gvr := schema.GroupVersionResource{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "pods"}
	list, err := c.dynamic.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{LabelSelector: runtimeMetricsPodLabelSelector(deploymentTargetID)})
	if err != nil {
		return RuntimeMetricsSnapshot{
			Available: false, Reason: "metrics_unavailable", DesiredReplicas: workload.DesiredReplicas,
			UpdatedReplicas: workload.UpdatedReplicas, ReadyReplicas: workload.ReadyReplicas,
			AvailableReplicas: workload.AvailableReplicas, Resources: workload.Resources, UpdatedAt: time.Now(),
		}, nil
	}
	snapshot := RuntimeMetricsSnapshot{
		Available: true, DesiredReplicas: workload.DesiredReplicas, UpdatedReplicas: workload.UpdatedReplicas,
		ReadyReplicas: workload.ReadyReplicas, AvailableReplicas: workload.AvailableReplicas,
		PodCount: len(list.Items), Resources: workload.Resources, UpdatedAt: time.Now(),
	}
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
	pod, container, err := c.runtimePod(ctx, options.RuntimePodSelection)
	if err != nil {
		return RuntimePodLogsResult{}, err
	}
	if containerNeverStarted(pod, container) {
		return RuntimePodLogsResult{}, ErrRuntimeContainerNotStarted
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
	pod, container, err := c.runtimePod(ctx, options.RuntimePodSelection)
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
	output := newRuntimeExecOutput(256 * 1024)
	startedAt := time.Now()
	streamErr := executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: output.writer(false),
		Stderr: output.writer(true),
		Tty:    false,
	})
	exitCode := 0
	if streamErr != nil {
		var exited bool
		exitCode, exited = runtimeExecExitCode(streamErr)
		if !exited {
			return RuntimeExecResult{}, fmt.Errorf("stream runtime exec: %w", streamErr)
		}
	}
	stdout, stderr, truncated := output.snapshot()
	return RuntimeExecResult{
		Pod:       pod.Name,
		Container: container,
		Stdout:    stdout,
		Stderr:    stderr,
		ExitCode:  exitCode,
		Truncated: truncated,
		Duration:  time.Since(startedAt).Milliseconds(),
	}, nil
}

func runtimeExecExitCode(err error) (int, bool) {
	var exitError clientexec.ExitError
	if !errors.As(err, &exitError) || !exitError.Exited() {
		return 0, false
	}
	return exitError.ExitStatus(), true
}

func (c *Client) RuntimeTerminal(ctx context.Context, options RuntimeTerminalOptions) (RuntimeTerminalResult, error) {
	if c.restConfig == nil {
		return RuntimeTerminalResult{}, fmt.Errorf("runtime terminal requires a REST config")
	}
	if options.Stdin == nil || options.Stdout == nil {
		return RuntimeTerminalResult{}, fmt.Errorf("runtime terminal streams are required")
	}
	pod, container, err := c.runtimePod(ctx, options.RuntimePodSelection)
	if err != nil {
		return RuntimeTerminalResult{}, err
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
		return RuntimeTerminalResult{}, err
	}
	streamErr := executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:             options.Stdin,
		Stdout:            options.Stdout,
		Stderr:            options.Stdout,
		Tty:               true,
		TerminalSizeQueue: options.SizeQueue,
	})
	return runtimeTerminalResult(streamErr)
}

func (c *Client) PodTerminal(ctx context.Context, options PodTerminalOptions) (RuntimeTerminalResult, error) {
	if c.restConfig == nil {
		return RuntimeTerminalResult{}, fmt.Errorf("pod terminal requires a REST config")
	}
	if options.Stdin == nil || options.Stdout == nil {
		return RuntimeTerminalResult{}, fmt.Errorf("pod terminal streams are required")
	}
	pod, container, err := c.namedPod(ctx, options.Namespace, options.PodName, options.Container)
	if err != nil {
		return RuntimeTerminalResult{}, err
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
		return RuntimeTerminalResult{}, err
	}
	streamErr := executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:             options.Stdin,
		Stdout:            options.Stdout,
		Stderr:            options.Stdout,
		Tty:               true,
		TerminalSizeQueue: options.SizeQueue,
	})
	return runtimeTerminalResult(streamErr)
}

func runtimeTerminalResult(err error) (RuntimeTerminalResult, error) {
	if err == nil {
		return RuntimeTerminalResult{ExitCode: 0}, nil
	}
	exitCode, exited := runtimeExecExitCode(err)
	if exited {
		return RuntimeTerminalResult{ExitCode: exitCode}, nil
	}
	return RuntimeTerminalResult{}, err
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

// ErrRuntimePodNotFound 表示部署配置下没有运行 Pod，或指定的 Pod 不属于该部署配置。
var ErrRuntimePodNotFound = errors.New("runtime pod not found")

// ErrRuntimeContainerNotStarted 表示所选副本的容器从未启动过（拉取镜像失败、创建中等），没有可读取的日志。
var ErrRuntimeContainerNotStarted = errors.New("runtime container has not started")

// RuntimePods 读取部署配置当前的运行副本：当前发布优先，其次就绪、运行中，最后按创建时间从新到旧。
func (c *Client) RuntimePods(ctx context.Context, namespace string, deploymentTargetID string, preferredReleaseID string) ([]RuntimePodSnapshot, error) {
	pods, err := c.listRuntimePods(ctx, namespace, deploymentTargetID)
	if err != nil {
		return nil, err
	}
	sortRuntimePods(pods, preferredReleaseID)
	items := make([]RuntimePodSnapshot, 0, len(pods))
	for _, pod := range pods {
		items = append(items, runtimePodSnapshot(pod))
	}
	return items, nil
}

func (c *Client) runtimePod(ctx context.Context, selection RuntimePodSelection) (corev1.Pod, string, error) {
	namespace := strings.TrimSpace(selection.Namespace)
	deploymentTargetID := strings.TrimSpace(selection.DeploymentTargetID)
	if namespace == "" {
		return corev1.Pod{}, "", fmt.Errorf("resource namespace is required")
	}
	if deploymentTargetID == "" {
		return corev1.Pod{}, "", fmt.Errorf("deployment target is required")
	}
	if podName := strings.TrimSpace(selection.Pod); podName != "" {
		pod, err := c.client.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return corev1.Pod{}, "", ErrRuntimePodNotFound
		}
		if err != nil {
			return corev1.Pod{}, "", err
		}
		if !isRuntimePodOf(*pod, deploymentTargetID) {
			return corev1.Pod{}, "", ErrRuntimePodNotFound
		}
		selectedContainer, err := selectPodContainer(*pod, selection.Container)
		if err != nil {
			return corev1.Pod{}, "", err
		}
		return *pod, selectedContainer, nil
	}
	pods, err := c.listRuntimePods(ctx, namespace, deploymentTargetID)
	if err != nil {
		return corev1.Pod{}, "", err
	}
	if len(pods) == 0 {
		return corev1.Pod{}, "", ErrRuntimePodNotFound
	}
	sortRuntimePods(pods, selection.PreferredReleaseID)
	selectedContainer, err := selectPodContainer(pods[0], selection.Container)
	if err != nil {
		return corev1.Pod{}, "", err
	}
	return pods[0], selectedContainer, nil
}

func (c *Client) listRuntimePods(ctx context.Context, namespace string, deploymentTargetID string) ([]corev1.Pod, error) {
	namespace = strings.TrimSpace(namespace)
	deploymentTargetID = strings.TrimSpace(deploymentTargetID)
	if namespace == "" {
		return nil, fmt.Errorf("resource namespace is required")
	}
	if deploymentTargetID == "" {
		return nil, fmt.Errorf("deployment target is required")
	}
	pods, err := c.client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: runtimePodLabelSelector(deploymentTargetID)})
	if err != nil {
		return nil, err
	}
	return pods.Items, nil
}

func runtimePodLabelSelector(deploymentTargetID string) string {
	return strings.Join([]string{
		ManagedByLabel + "=" + ManagedByValue,
		DeploymentTargetIDLabel + "=" + deploymentTargetID,
		ScopeLabel + "!=build",
	}, ",")
}

// runtimeMetricsPodLabelSelector selects only workload Pods. Hook Job Pods
// carry the deployment target label too, but they are not part of the Pod
// template whose requests and limits are compared with this usage.
func runtimeMetricsPodLabelSelector(deploymentTargetID string) string {
	return runtimePodLabelSelector(deploymentTargetID) + ",!" + HookRunIDLabel
}

func isRuntimePodOf(pod corev1.Pod, deploymentTargetID string) bool {
	return pod.Labels[ManagedByLabel] == ManagedByValue &&
		pod.Labels[DeploymentTargetIDLabel] == deploymentTargetID &&
		pod.Labels[ScopeLabel] != "build"
}

func sortRuntimePods(pods []corev1.Pod, preferredReleaseID string) {
	preferredReleaseID = strings.TrimSpace(preferredReleaseID)
	sort.SliceStable(pods, func(left, right int) bool {
		if preferredReleaseID != "" {
			leftPreferred := pods[left].Labels[ReleaseIDLabel] == preferredReleaseID
			rightPreferred := pods[right].Labels[ReleaseIDLabel] == preferredReleaseID
			if leftPreferred != rightPreferred {
				return leftPreferred
			}
		}
		leftReady := podReady(pods[left])
		rightReady := podReady(pods[right])
		if leftReady != rightReady {
			return leftReady
		}
		leftRunning := pods[left].Status.Phase == corev1.PodRunning
		rightRunning := pods[right].Status.Phase == corev1.PodRunning
		if leftRunning != rightRunning {
			return leftRunning
		}
		return pods[left].CreationTimestamp.After(pods[right].CreationTimestamp.Time)
	})
}

func runtimePodSnapshot(pod corev1.Pod) RuntimePodSnapshot {
	var restartCount int32
	for _, status := range pod.Status.ContainerStatuses {
		restartCount += status.RestartCount
	}
	containers := make([]string, 0, len(pod.Spec.Containers))
	for _, container := range pod.Spec.Containers {
		containers = append(containers, container.Name)
	}
	return RuntimePodSnapshot{
		Name:         pod.Name,
		Phase:        string(pod.Status.Phase),
		Ready:        podReady(pod),
		Terminating:  pod.DeletionTimestamp != nil,
		RestartCount: restartCount,
		ReleaseID:    pod.Labels[ReleaseIDLabel],
		Containers:   containers,
		CreatedAt:    pod.CreationTimestamp.Time,
	}
}

// containerNeverStarted 判断容器是否仍在首次等待（未运行过也未退出过）；崩溃重启过的容器仍有上次输出可读。
func containerNeverStarted(pod corev1.Pod, container string) bool {
	for _, status := range pod.Status.ContainerStatuses {
		if status.Name != container {
			continue
		}
		return status.State.Waiting != nil && status.RestartCount == 0 && status.LastTerminationState.Terminated == nil
	}
	return false
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
