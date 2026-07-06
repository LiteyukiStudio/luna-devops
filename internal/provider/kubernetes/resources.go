package kubernetes

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
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

type PodTerminalOptions struct {
	Namespace string
	PodName   string
	Container string
	Stdin     io.Reader
	Stdout    io.Writer
	SizeQueue remotecommand.TerminalSizeQueue
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
	case "statefulset":
		if namespace == "" {
			return ResourceSnapshot{}, fmt.Errorf("resource namespace is required")
		}
		item, err := c.client.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return ResourceSnapshot{}, err
		}
		return managedSnapshot(statefulSetSnapshot(*item))
	case "pod":
		if namespace == "" {
			return ResourceSnapshot{}, fmt.Errorf("resource namespace is required")
		}
		item, err := c.client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return ResourceSnapshot{}, err
		}
		return managedSnapshot(podSnapshot(*item))
	case "horizontalpodautoscaler":
		if namespace == "" {
			return ResourceSnapshot{}, fmt.Errorf("resource namespace is required")
		}
		item, err := c.client.AutoscalingV2().HorizontalPodAutoscalers(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return ResourceSnapshot{}, err
		}
		return managedSnapshot(hpaSnapshot(*item))
	case "service":
		if namespace == "" {
			return ResourceSnapshot{}, fmt.Errorf("resource namespace is required")
		}
		item, err := c.client.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return ResourceSnapshot{}, err
		}
		return managedSnapshot(serviceSnapshot(*item))
	case "httproute":
		if namespace == "" {
			return ResourceSnapshot{}, fmt.Errorf("resource namespace is required")
		}
		item, err := c.getGatewayAPIResource(ctx, httpRouteGVR, namespace, name)
		if err != nil {
			return ResourceSnapshot{}, err
		}
		return managedSnapshot(httpRouteSnapshot(item))
	case "gateway":
		if namespace == "" {
			return ResourceSnapshot{}, fmt.Errorf("resource namespace is required")
		}
		item, err := c.getGatewayAPIResource(ctx, gatewayGVR, namespace, name)
		if err != nil {
			return ResourceSnapshot{}, err
		}
		return managedSnapshot(gatewaySnapshot(item))
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
	case "statefulset":
		if namespace == "" {
			return "", ResourceSnapshot{}, fmt.Errorf("resource namespace is required")
		}
		item, err := c.client.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return "", ResourceSnapshot{}, err
		}
		snapshot, err := managedSnapshot(statefulSetSnapshot(*item))
		if err != nil {
			return "", ResourceSnapshot{}, err
		}
		item.TypeMeta = metav1.TypeMeta{APIVersion: "apps/v1", Kind: "StatefulSet"}
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
	case "horizontalpodautoscaler":
		if namespace == "" {
			return "", ResourceSnapshot{}, fmt.Errorf("resource namespace is required")
		}
		item, err := c.client.AutoscalingV2().HorizontalPodAutoscalers(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return "", ResourceSnapshot{}, err
		}
		snapshot, err := managedSnapshot(hpaSnapshot(*item))
		if err != nil {
			return "", ResourceSnapshot{}, err
		}
		item.TypeMeta = metav1.TypeMeta{APIVersion: "autoscaling/v2", Kind: "HorizontalPodAutoscaler"}
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
	case "httproute":
		if namespace == "" {
			return "", ResourceSnapshot{}, fmt.Errorf("resource namespace is required")
		}
		item, err := c.getGatewayAPIResource(ctx, httpRouteGVR, namespace, name)
		if err != nil {
			return "", ResourceSnapshot{}, err
		}
		snapshot, err := managedSnapshot(httpRouteSnapshot(item))
		if err != nil {
			return "", ResourceSnapshot{}, err
		}
		item.SetManagedFields(nil)
		content, err := yaml.Marshal(item)
		return string(content), snapshot, err
	case "gateway":
		if namespace == "" {
			return "", ResourceSnapshot{}, fmt.Errorf("resource namespace is required")
		}
		item, err := c.getGatewayAPIResource(ctx, gatewayGVR, namespace, name)
		if err != nil {
			return "", ResourceSnapshot{}, err
		}
		snapshot, err := managedSnapshot(gatewaySnapshot(item))
		if err != nil {
			return "", ResourceSnapshot{}, err
		}
		item.SetManagedFields(nil)
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
	case "statefulset":
		if namespace == "" {
			return fmt.Errorf("resource namespace is required")
		}
		item, err := c.client.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if !isManagedResource(item.Labels) {
			return fmt.Errorf("resource is not managed by Liteyuki DevOps")
		}
		return c.client.AppsV1().StatefulSets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
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
	case "horizontalpodautoscaler":
		if namespace == "" {
			return fmt.Errorf("resource namespace is required")
		}
		item, err := c.client.AutoscalingV2().HorizontalPodAutoscalers(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if !isManagedResource(item.Labels) {
			return fmt.Errorf("resource is not managed by Liteyuki DevOps")
		}
		return c.client.AutoscalingV2().HorizontalPodAutoscalers(namespace).Delete(ctx, name, metav1.DeleteOptions{})
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
	case "httproute":
		if namespace == "" {
			return fmt.Errorf("resource namespace is required")
		}
		item, err := c.getGatewayAPIResource(ctx, httpRouteGVR, namespace, name)
		if err != nil {
			return err
		}
		if !isManagedResource(item.GetLabels()) {
			return fmt.Errorf("resource is not managed by Liteyuki DevOps")
		}
		return c.dynamic.Resource(httpRouteGVR).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	case "gateway":
		if namespace == "" {
			return fmt.Errorf("resource namespace is required")
		}
		item, err := c.getGatewayAPIResource(ctx, gatewayGVR, namespace, name)
		if err != nil {
			return err
		}
		if !isManagedResource(item.GetLabels()) {
			return fmt.Errorf("resource is not managed by Liteyuki DevOps")
		}
		return c.dynamic.Resource(gatewayGVR).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
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
