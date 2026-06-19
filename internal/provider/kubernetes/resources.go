package kubernetes

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
	"sigs.k8s.io/yaml"
)

type ResourceListOptions struct {
	Kind               string
	Namespace          string
	ProjectID          string
	ApplicationID      string
	EnvironmentID      string
	DeploymentTargetID string
	RouteID            string
}

type ResourceSnapshot struct {
	ID                 string            `json:"id"`
	Kind               string            `json:"kind"`
	Name               string            `json:"name"`
	Namespace          string            `json:"namespace"`
	Status             string            `json:"status"`
	Summary            string            `json:"summary"`
	ProjectID          string            `json:"projectId"`
	ApplicationID      string            `json:"applicationId"`
	EnvironmentID      string            `json:"environmentId"`
	DeploymentTargetID string            `json:"deploymentTargetId"`
	ReleaseID          string            `json:"releaseId"`
	RouteID            string            `json:"routeId"`
	Labels             map[string]string `json:"labels"`
	CreatedAt          time.Time         `json:"createdAt"`
	UpdatedAt          time.Time         `json:"updatedAt"`
}

type ResourceEventSnapshot struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Reason    string    `json:"reason"`
	Message   string    `json:"message"`
	Source    string    `json:"source"`
	Count     int32     `json:"count"`
	FirstSeen time.Time `json:"firstSeen"`
	LastSeen  time.Time `json:"lastSeen"`
}

type RuntimePodLogsOptions struct {
	Namespace          string
	DeploymentTargetID string
	Container          string
	TailLines          int64
}

type RuntimePodLogsResult struct {
	Pod       string `json:"pod"`
	Container string `json:"container"`
	Content   string `json:"content"`
}

type RuntimeExecOptions struct {
	Namespace          string
	DeploymentTargetID string
	Container          string
	Command            string
}

type RuntimeExecResult struct {
	Pod       string `json:"pod"`
	Container string `json:"container"`
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
	ExitCode  int    `json:"exitCode"`
}

type RuntimeTerminalOptions struct {
	Namespace          string
	DeploymentTargetID string
	Container          string
	Stdin              io.Reader
	Stdout             io.Writer
	SizeQueue          remotecommand.TerminalSizeQueue
}

type RuntimeMetricsOptions struct {
	Namespace          string
	DeploymentTargetID string
}

type RuntimeMetricsSnapshot struct {
	Available        bool      `json:"available"`
	Reason           string    `json:"reason,omitempty"`
	PodCount         int       `json:"podCount"`
	ContainerCount   int       `json:"containerCount"`
	CPUUsageMilli    int64     `json:"cpuUsageMilli"`
	MemoryUsageBytes int64     `json:"memoryUsageBytes"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

func (c *Client) ListManagedResources(ctx context.Context, options ResourceListOptions) ([]ResourceSnapshot, error) {
	switch normalizeResourceKind(options.Kind) {
	case "namespaces":
		return c.listManagedNamespaces(ctx, options)
	case "workloads":
		return c.listManagedWorkloads(ctx, options)
	case "services":
		return c.listManagedServicesAndIngresses(ctx, options)
	case "configs":
		return c.listManagedConfigs(ctx, options)
	case "storage":
		return c.listManagedStorage(ctx, options)
	default:
		return nil, fmt.Errorf("unsupported resource kind: %s", options.Kind)
	}
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
	selectedContainer := strings.TrimSpace(container)
	if selectedContainer == "" && len(pod.Spec.Containers) > 0 {
		selectedContainer = pod.Spec.Containers[0].Name
	}
	if selectedContainer == "" {
		return corev1.Pod{}, "", fmt.Errorf("runtime container not found")
	}
	for _, item := range pod.Spec.Containers {
		if item.Name == selectedContainer {
			return pod, selectedContainer, nil
		}
	}
	return corev1.Pod{}, "", fmt.Errorf("runtime container %q not found", selectedContainer)
}

func (c *Client) GetManagedResource(ctx context.Context, kind string, namespace string, name string) (ResourceSnapshot, error) {
	kind = normalizeResourceObjectKind(kind)
	name = strings.TrimSpace(name)
	namespace = strings.TrimSpace(namespace)
	if name == "" {
		return ResourceSnapshot{}, fmt.Errorf("resource name is required")
	}
	switch kind {
	case "namespace":
		item, err := c.client.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return ResourceSnapshot{}, err
		}
		return managedSnapshotFromMeta("Namespace", item.ObjectMeta, "", item.Status.Phase, "")
	case "deployment":
		if namespace == "" {
			return ResourceSnapshot{}, fmt.Errorf("resource namespace is required")
		}
		item, err := c.client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return ResourceSnapshot{}, err
		}
		return managedSnapshot(deploymentSnapshot(*item))
	case "pod":
		if namespace == "" {
			return ResourceSnapshot{}, fmt.Errorf("resource namespace is required")
		}
		item, err := c.client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return ResourceSnapshot{}, err
		}
		return managedSnapshot(podSnapshot(*item))
	case "service":
		if namespace == "" {
			return ResourceSnapshot{}, fmt.Errorf("resource namespace is required")
		}
		item, err := c.client.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return ResourceSnapshot{}, err
		}
		return managedSnapshot(serviceSnapshot(*item))
	case "ingress":
		if namespace == "" {
			return ResourceSnapshot{}, fmt.Errorf("resource namespace is required")
		}
		item, err := c.client.NetworkingV1().Ingresses(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return ResourceSnapshot{}, err
		}
		return managedSnapshot(ingressSnapshot(*item))
	case "configmap":
		if namespace == "" {
			return ResourceSnapshot{}, fmt.Errorf("resource namespace is required")
		}
		item, err := c.client.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return ResourceSnapshot{}, err
		}
		return managedSnapshotFromMeta("ConfigMap", item.ObjectMeta, "", fmt.Sprintf("%d keys", len(item.Data)), "")
	case "secret":
		if namespace == "" {
			return ResourceSnapshot{}, fmt.Errorf("resource namespace is required")
		}
		item, err := c.client.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return ResourceSnapshot{}, err
		}
		return managedSnapshotFromMeta("Secret", item.ObjectMeta, "", string(item.Type), "data hidden")
	case "persistentvolumeclaim":
		if namespace == "" {
			return ResourceSnapshot{}, fmt.Errorf("resource namespace is required")
		}
		item, err := c.client.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return ResourceSnapshot{}, err
		}
		return managedSnapshotFromMeta("PersistentVolumeClaim", item.ObjectMeta, "", item.Status.Phase, pvcSummary(*item))
	default:
		return ResourceSnapshot{}, fmt.Errorf("unsupported resource kind: %s", kind)
	}
}

func (c *Client) GetManagedResourceYAML(ctx context.Context, kind string, namespace string, name string) (string, ResourceSnapshot, error) {
	kind = normalizeResourceObjectKind(kind)
	name = strings.TrimSpace(name)
	namespace = strings.TrimSpace(namespace)
	if name == "" {
		return "", ResourceSnapshot{}, fmt.Errorf("resource name is required")
	}
	switch kind {
	case "namespace":
		item, err := c.client.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return "", ResourceSnapshot{}, err
		}
		snapshot, err := managedSnapshotFromMeta("Namespace", item.ObjectMeta, "", item.Status.Phase, "")
		if err != nil {
			return "", ResourceSnapshot{}, err
		}
		item.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "Namespace"}
		item.ManagedFields = nil
		content, err := yaml.Marshal(item)
		return string(content), snapshot, err
	case "deployment":
		if namespace == "" {
			return "", ResourceSnapshot{}, fmt.Errorf("resource namespace is required")
		}
		item, err := c.client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return "", ResourceSnapshot{}, err
		}
		snapshot, err := managedSnapshot(deploymentSnapshot(*item))
		if err != nil {
			return "", ResourceSnapshot{}, err
		}
		item.TypeMeta = metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"}
		item.ManagedFields = nil
		content, err := yaml.Marshal(item)
		return string(content), snapshot, err
	case "pod":
		if namespace == "" {
			return "", ResourceSnapshot{}, fmt.Errorf("resource namespace is required")
		}
		item, err := c.client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return "", ResourceSnapshot{}, err
		}
		snapshot, err := managedSnapshot(podSnapshot(*item))
		if err != nil {
			return "", ResourceSnapshot{}, err
		}
		item.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"}
		item.ManagedFields = nil
		content, err := yaml.Marshal(item)
		return string(content), snapshot, err
	case "service":
		if namespace == "" {
			return "", ResourceSnapshot{}, fmt.Errorf("resource namespace is required")
		}
		item, err := c.client.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return "", ResourceSnapshot{}, err
		}
		snapshot, err := managedSnapshot(serviceSnapshot(*item))
		if err != nil {
			return "", ResourceSnapshot{}, err
		}
		item.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "Service"}
		item.ManagedFields = nil
		content, err := yaml.Marshal(item)
		return string(content), snapshot, err
	case "ingress":
		if namespace == "" {
			return "", ResourceSnapshot{}, fmt.Errorf("resource namespace is required")
		}
		item, err := c.client.NetworkingV1().Ingresses(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return "", ResourceSnapshot{}, err
		}
		snapshot, err := managedSnapshot(ingressSnapshot(*item))
		if err != nil {
			return "", ResourceSnapshot{}, err
		}
		item.TypeMeta = metav1.TypeMeta{APIVersion: "networking.k8s.io/v1", Kind: "Ingress"}
		item.ManagedFields = nil
		content, err := yaml.Marshal(item)
		return string(content), snapshot, err
	case "configmap":
		if namespace == "" {
			return "", ResourceSnapshot{}, fmt.Errorf("resource namespace is required")
		}
		item, err := c.client.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return "", ResourceSnapshot{}, err
		}
		snapshot, err := managedSnapshotFromMeta("ConfigMap", item.ObjectMeta, "", fmt.Sprintf("%d keys", len(item.Data)), "")
		if err != nil {
			return "", ResourceSnapshot{}, err
		}
		item.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"}
		item.ManagedFields = nil
		content, err := yaml.Marshal(item)
		return string(content), snapshot, err
	case "secret":
		if namespace == "" {
			return "", ResourceSnapshot{}, fmt.Errorf("resource namespace is required")
		}
		item, err := c.client.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return "", ResourceSnapshot{}, err
		}
		snapshot, err := managedSnapshotFromMeta("Secret", item.ObjectMeta, "", string(item.Type), "data hidden")
		if err != nil {
			return "", ResourceSnapshot{}, err
		}
		item.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"}
		item.ManagedFields = nil
		item.StringData = redactedSecretStringData(item.Data)
		item.Data = nil
		content, err := yaml.Marshal(item)
		return string(content), snapshot, err
	case "persistentvolumeclaim":
		if namespace == "" {
			return "", ResourceSnapshot{}, fmt.Errorf("resource namespace is required")
		}
		item, err := c.client.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return "", ResourceSnapshot{}, err
		}
		snapshot, err := managedSnapshotFromMeta("PersistentVolumeClaim", item.ObjectMeta, "", item.Status.Phase, pvcSummary(*item))
		if err != nil {
			return "", ResourceSnapshot{}, err
		}
		item.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "PersistentVolumeClaim"}
		item.ManagedFields = nil
		content, err := yaml.Marshal(item)
		return string(content), snapshot, err
	default:
		return "", ResourceSnapshot{}, fmt.Errorf("unsupported resource kind: %s", kind)
	}
}

func redactedSecretStringData(data map[string][]byte) map[string]string {
	if len(data) == 0 {
		return nil
	}
	redacted := make(map[string]string, len(data))
	for key := range data {
		redacted[key] = "<redacted>"
	}
	return redacted
}

func (c *Client) DeleteManagedResource(ctx context.Context, kind string, namespace string, name string) error {
	kind = normalizeResourceObjectKind(kind)
	name = strings.TrimSpace(name)
	namespace = strings.TrimSpace(namespace)
	if name == "" {
		return fmt.Errorf("resource name is required")
	}
	switch kind {
	case "namespace":
		item, err := c.client.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if !isManagedResource(item.Labels) {
			return fmt.Errorf("resource is not managed by Liteyuki DevOps")
		}
		return c.client.CoreV1().Namespaces().Delete(ctx, name, metav1.DeleteOptions{})
	case "deployment":
		if namespace == "" {
			return fmt.Errorf("resource namespace is required")
		}
		item, err := c.client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if !isManagedResource(item.Labels) {
			return fmt.Errorf("resource is not managed by Liteyuki DevOps")
		}
		return c.client.AppsV1().Deployments(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	case "pod":
		if namespace == "" {
			return fmt.Errorf("resource namespace is required")
		}
		item, err := c.client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if !isManagedResource(item.Labels) {
			return fmt.Errorf("resource is not managed by Liteyuki DevOps")
		}
		return c.client.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	case "service":
		if namespace == "" {
			return fmt.Errorf("resource namespace is required")
		}
		item, err := c.client.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if !isManagedResource(item.Labels) {
			return fmt.Errorf("resource is not managed by Liteyuki DevOps")
		}
		return c.client.CoreV1().Services(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	case "ingress":
		if namespace == "" {
			return fmt.Errorf("resource namespace is required")
		}
		item, err := c.client.NetworkingV1().Ingresses(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if !isManagedResource(item.Labels) {
			return fmt.Errorf("resource is not managed by Liteyuki DevOps")
		}
		return c.client.NetworkingV1().Ingresses(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	case "configmap":
		if namespace == "" {
			return fmt.Errorf("resource namespace is required")
		}
		item, err := c.client.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if !isManagedResource(item.Labels) {
			return fmt.Errorf("resource is not managed by Liteyuki DevOps")
		}
		return c.client.CoreV1().ConfigMaps(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	case "secret":
		if namespace == "" {
			return fmt.Errorf("resource namespace is required")
		}
		item, err := c.client.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if !isManagedResource(item.Labels) {
			return fmt.Errorf("resource is not managed by Liteyuki DevOps")
		}
		return c.client.CoreV1().Secrets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	case "persistentvolumeclaim":
		if namespace == "" {
			return fmt.Errorf("resource namespace is required")
		}
		item, err := c.client.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if !isManagedResource(item.Labels) {
			return fmt.Errorf("resource is not managed by Liteyuki DevOps")
		}
		return c.client.CoreV1().PersistentVolumeClaims(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	default:
		return fmt.Errorf("unsupported resource kind: %s", kind)
	}
}

func (c *Client) ListManagedResourceEvents(ctx context.Context, kind string, namespace string, name string) ([]ResourceEventSnapshot, ResourceSnapshot, error) {
	snapshot, err := c.GetManagedResource(ctx, kind, namespace, name)
	if err != nil {
		return nil, ResourceSnapshot{}, err
	}
	selector := fields.Set{
		"involvedObject.kind": snapshot.Kind,
		"involvedObject.name": snapshot.Name,
	}.AsSelector().String()
	events, err := c.client.CoreV1().Events(snapshot.Namespace).List(ctx, metav1.ListOptions{FieldSelector: selector})
	if err != nil {
		return nil, ResourceSnapshot{}, err
	}
	items := make([]ResourceEventSnapshot, 0, len(events.Items))
	for _, item := range events.Items {
		items = append(items, eventSnapshot(item))
	}
	sort.Slice(items, func(left, right int) bool {
		return items[left].LastSeen.After(items[right].LastSeen)
	})
	return items, snapshot, nil
}

func (c *Client) listManagedNamespaces(ctx context.Context, options ResourceListOptions) ([]ResourceSnapshot, error) {
	list, err := c.client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{LabelSelector: managedResourceSelector(options)})
	if err != nil {
		return nil, err
	}
	items := make([]ResourceSnapshot, 0, len(list.Items))
	for _, item := range list.Items {
		if !matchesResourceOptions(item.Labels, options) {
			continue
		}
		items = append(items, snapshotFromMeta("Namespace", item.ObjectMeta, "", item.Status.Phase, ""))
	}
	return items, nil
}

func managedSnapshot(snapshot ResourceSnapshot) (ResourceSnapshot, error) {
	if !isManagedResource(snapshot.Labels) {
		return ResourceSnapshot{}, fmt.Errorf("resource is not managed by Liteyuki DevOps")
	}
	return snapshot, nil
}

func managedSnapshotFromMeta(kind string, meta metav1.ObjectMeta, namespace string, status any, summary string) (ResourceSnapshot, error) {
	return managedSnapshot(snapshotFromMeta(kind, meta, namespace, status, summary))
}

func eventSnapshot(item corev1.Event) ResourceEventSnapshot {
	firstSeen := item.FirstTimestamp.Time
	lastSeen := item.LastTimestamp.Time
	if firstSeen.IsZero() {
		firstSeen = item.EventTime.Time
	}
	if lastSeen.IsZero() {
		lastSeen = item.EventTime.Time
	}
	if lastSeen.IsZero() {
		lastSeen = item.CreationTimestamp.Time
	}
	if firstSeen.IsZero() {
		firstSeen = lastSeen
	}
	return ResourceEventSnapshot{
		ID:        resourceID("Event", item.Namespace, item.Name),
		Type:      item.Type,
		Reason:    item.Reason,
		Message:   item.Message,
		Source:    firstNonEmpty(item.ReportingController, item.Source.Component),
		Count:     item.Count,
		FirstSeen: firstSeen,
		LastSeen:  lastSeen,
	}
}

func (c *Client) listManagedWorkloads(ctx context.Context, options ResourceListOptions) ([]ResourceSnapshot, error) {
	selector := managedRuntimeResourceSelector(options)
	deployments, err := c.client.AppsV1().Deployments(options.Namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, err
	}
	pods, err := c.client.CoreV1().Pods(options.Namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, err
	}
	items := make([]ResourceSnapshot, 0, len(deployments.Items)+len(pods.Items))
	for _, item := range deployments.Items {
		items = append(items, deploymentSnapshot(item))
	}
	for _, item := range pods.Items {
		items = append(items, podSnapshot(item))
	}
	return items, nil
}

func (c *Client) listManagedServicesAndIngresses(ctx context.Context, options ResourceListOptions) ([]ResourceSnapshot, error) {
	selector := managedResourceSelector(options)
	services, err := c.client.CoreV1().Services(options.Namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, err
	}
	ingresses, err := c.client.NetworkingV1().Ingresses(options.Namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, err
	}
	items := make([]ResourceSnapshot, 0, len(services.Items)+len(ingresses.Items))
	for _, item := range services.Items {
		items = append(items, serviceSnapshot(item))
	}
	for _, item := range ingresses.Items {
		items = append(items, ingressSnapshot(item))
	}
	return items, nil
}

func (c *Client) listManagedConfigs(ctx context.Context, options ResourceListOptions) ([]ResourceSnapshot, error) {
	selector := managedResourceSelector(options)
	configMaps, err := c.client.CoreV1().ConfigMaps(options.Namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, err
	}
	secrets, err := c.client.CoreV1().Secrets(options.Namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, err
	}
	items := make([]ResourceSnapshot, 0, len(configMaps.Items)+len(secrets.Items))
	for _, item := range configMaps.Items {
		items = append(items, snapshotFromMeta("ConfigMap", item.ObjectMeta, "", fmt.Sprintf("%d keys", len(item.Data)), ""))
	}
	for _, item := range secrets.Items {
		items = append(items, snapshotFromMeta("Secret", item.ObjectMeta, "", string(item.Type), "data hidden"))
	}
	return items, nil
}

func (c *Client) listManagedStorage(ctx context.Context, options ResourceListOptions) ([]ResourceSnapshot, error) {
	claims, err := c.client.CoreV1().PersistentVolumeClaims(options.Namespace).List(ctx, metav1.ListOptions{LabelSelector: managedResourceSelector(options)})
	if err != nil {
		return nil, err
	}
	items := make([]ResourceSnapshot, 0, len(claims.Items))
	for _, item := range claims.Items {
		items = append(items, snapshotFromMeta("PersistentVolumeClaim", item.ObjectMeta, "", item.Status.Phase, pvcSummary(item)))
	}
	return items, nil
}

func deploymentSnapshot(item appsv1.Deployment) ResourceSnapshot {
	desired := int32(0)
	if item.Spec.Replicas != nil {
		desired = *item.Spec.Replicas
	}
	status := "progressing"
	if item.Status.ReadyReplicas >= desired && item.Status.AvailableReplicas >= desired {
		status = "ready"
	}
	return snapshotFromMeta("Deployment", item.ObjectMeta, "", status, fmt.Sprintf("ready %d/%d", item.Status.ReadyReplicas, desired))
}

func podSnapshot(item corev1.Pod) ResourceSnapshot {
	ready := 0
	for _, condition := range item.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			ready = 1
			break
		}
	}
	return snapshotFromMeta("Pod", item.ObjectMeta, "", item.Status.Phase, podSummary(item, ready))
}

func serviceSnapshot(item corev1.Service) ResourceSnapshot {
	ports := make([]string, 0, len(item.Spec.Ports))
	for _, port := range item.Spec.Ports {
		ports = append(ports, strconv.Itoa(int(port.Port)))
	}
	return snapshotFromMeta("Service", item.ObjectMeta, "", string(item.Spec.Type), strings.Join(ports, ", "))
}

func ingressSnapshot(item networkingv1.Ingress) ResourceSnapshot {
	hosts := make([]string, 0, len(item.Spec.Rules))
	for _, rule := range item.Spec.Rules {
		if rule.Host != "" {
			hosts = append(hosts, rule.Host)
		}
	}
	return snapshotFromMeta("Ingress", item.ObjectMeta, "", "active", strings.Join(hosts, ", "))
}

func pvcSummary(item corev1.PersistentVolumeClaim) string {
	storage := item.Status.Capacity.Storage()
	if storage == nil || storage.IsZero() {
		if item.Spec.StorageClassName != nil {
			return *item.Spec.StorageClassName
		}
		return ""
	}
	return storage.String()
}

func snapshotFromMeta(kind string, meta metav1.ObjectMeta, namespace string, status any, summary string) ResourceSnapshot {
	labels := cloneLabels(meta.Labels)
	ns := namespace
	if ns == "" {
		ns = meta.Namespace
	}
	return ResourceSnapshot{
		ID:                 resourceID(kind, ns, meta.Name),
		Kind:               kind,
		Name:               meta.Name,
		Namespace:          ns,
		Status:             fmt.Sprint(status),
		Summary:            summary,
		ProjectID:          labels[ProjectIDLabel],
		ApplicationID:      labels[ApplicationIDLabel],
		EnvironmentID:      labels[EnvironmentIDLabel],
		DeploymentTargetID: labels[DeploymentTargetIDLabel],
		ReleaseID:          labels[ReleaseIDLabel],
		RouteID:            labels[GatewayRouteIDLabel],
		Labels:             labels,
		CreatedAt:          meta.CreationTimestamp.Time,
		UpdatedAt:          resourceUpdatedAt(meta),
	}
}

func resourceUpdatedAt(meta metav1.ObjectMeta) time.Time {
	updatedAt := meta.CreationTimestamp.Time
	for _, field := range meta.ManagedFields {
		if field.Time == nil {
			continue
		}
		if field.Time.After(updatedAt) {
			updatedAt = field.Time.Time
		}
	}
	return updatedAt
}

func managedResourceSelector(options ResourceListOptions) string {
	parts := []string{ManagedByLabel + "=" + ManagedByValue}
	if options.ProjectID != "" {
		parts = append(parts, ProjectIDLabel+"="+options.ProjectID)
	}
	if options.ApplicationID != "" {
		parts = append(parts, ApplicationIDLabel+"="+options.ApplicationID)
	}
	if options.EnvironmentID != "" {
		parts = append(parts, EnvironmentIDLabel+"="+options.EnvironmentID)
	}
	if options.DeploymentTargetID != "" {
		parts = append(parts, DeploymentTargetIDLabel+"="+options.DeploymentTargetID)
	}
	if options.RouteID != "" {
		parts = append(parts, GatewayRouteIDLabel+"="+options.RouteID)
	}
	return strings.Join(parts, ",")
}

func managedRuntimeResourceSelector(options ResourceListOptions) string {
	selector := managedResourceSelector(options)
	if selector == "" {
		return ScopeLabel + "!=build"
	}
	return selector + "," + ScopeLabel + "!=build"
}

func matchesResourceOptions(labels map[string]string, options ResourceListOptions) bool {
	if options.ProjectID != "" && labels[ProjectIDLabel] != options.ProjectID {
		return false
	}
	if options.ApplicationID != "" && labels[ApplicationIDLabel] != options.ApplicationID {
		return false
	}
	if options.EnvironmentID != "" && labels[EnvironmentIDLabel] != options.EnvironmentID {
		return false
	}
	if options.DeploymentTargetID != "" && labels[DeploymentTargetIDLabel] != options.DeploymentTargetID {
		return false
	}
	if options.RouteID != "" && labels[GatewayRouteIDLabel] != options.RouteID {
		return false
	}
	return true
}

func normalizeResourceKind(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "namespace", "namespaces":
		return "namespaces"
	case "workload", "workloads":
		return "workloads"
	case "service", "services", "ingress", "ingresses":
		return "services"
	case "config", "configs", "secret", "secrets":
		return "configs"
	case "storage", "pvc", "pvcs":
		return "storage"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func normalizeResourceObjectKind(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "namespace", "namespaces":
		return "namespace"
	case "deployment", "deployments":
		return "deployment"
	case "pod", "pods":
		return "pod"
	case "service", "services":
		return "service"
	case "ingress", "ingresses":
		return "ingress"
	case "configmap", "configmaps":
		return "configmap"
	case "secret", "secrets":
		return "secret"
	case "persistentvolumeclaim", "persistentvolumeclaims", "pvc", "pvcs":
		return "persistentvolumeclaim"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func isManagedResource(labels map[string]string) bool {
	return labels[ManagedByLabel] == ManagedByValue
}

func podSummary(item corev1.Pod, ready int) string {
	parts := []string{fmt.Sprintf("ready %d/1", ready)}
	for _, status := range item.Status.ContainerStatuses {
		switch {
		case status.State.Waiting != nil:
			parts = append(parts, strings.TrimSpace(status.Name+" waiting: "+firstNonEmpty(status.State.Waiting.Reason, status.State.Waiting.Message)))
		case status.State.Terminated != nil:
			parts = append(parts, strings.TrimSpace(status.Name+" terminated: "+firstNonEmpty(status.State.Terminated.Reason, status.State.Terminated.Message)))
		case !status.Ready:
			parts = append(parts, status.Name+" not ready")
		}
	}
	for _, condition := range item.Status.Conditions {
		if condition.Status == corev1.ConditionTrue || condition.Reason == "" && condition.Message == "" {
			continue
		}
		parts = append(parts, strings.TrimSpace(string(condition.Type)+": "+firstNonEmpty(condition.Reason, condition.Message)))
	}
	return strings.Join(compactStrings(parts), "; ")
}

func podReady(item corev1.Pod) bool {
	for _, condition := range item.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func compactStrings(values []string) []string {
	compacted := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			compacted = append(compacted, strings.TrimSpace(value))
		}
	}
	return compacted
}

func resourceID(kind string, namespace string, name string) string {
	if namespace == "" {
		return kind + "/" + name
	}
	return kind + "/" + namespace + "/" + name
}

func cloneLabels(labels map[string]string) map[string]string {
	result := make(map[string]string, len(labels))
	for key, value := range labels {
		result[key] = value
	}
	return result
}
