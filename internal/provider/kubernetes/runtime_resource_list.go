package kubernetes

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/pager"
)

func (c *Client) ListManagedResources(ctx context.Context, options ResourceListOptions) ([]ResourceSnapshot, error) {
	switch normalizeResourceKind(options.Kind) {
	case "namespaces":
		return c.listManagedNamespaces(ctx, options)
	case "workloads":
		return c.listManagedWorkloads(ctx, options)
	case "services":
		return c.listManagedServicesAndRoutes(ctx, options)
	case "configs":
		return c.listManagedConfigs(ctx, options)
	case "storage":
		return c.listManagedStorage(ctx, options)
	default:
		return nil, fmt.Errorf("unsupported resource kind: %s", options.Kind)
	}
}

func listResourceSnapshots(
	ctx context.Context,
	options metav1.ListOptions,
	listPage pager.ListPageFunc,
	snapshot func(k8sruntime.Object) ResourceSnapshot,
) ([]ResourceSnapshot, error) {
	// A resource category combines several Kubernetes kinds. Drain each kind's
	// continuation pages before the API applies one global sort and pagination.
	items := make([]ResourceSnapshot, 0)
	err := pager.New(listPage).EachListItem(ctx, options, func(item k8sruntime.Object) error {
		items = append(items, snapshot(item))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return items, nil
}

func (c *Client) listManagedNamespaces(ctx context.Context, options ResourceListOptions) ([]ResourceSnapshot, error) {
	return listResourceSnapshots(
		ctx,
		metav1.ListOptions{LabelSelector: managedResourceSelector(options)},
		func(ctx context.Context, listOptions metav1.ListOptions) (k8sruntime.Object, error) {
			return c.client.CoreV1().Namespaces().List(ctx, listOptions)
		},
		func(object k8sruntime.Object) ResourceSnapshot {
			item := object.(*corev1.Namespace)
			return snapshotFromMeta("Namespace", item.ObjectMeta, "", item.Status.Phase, "")
		},
	)
}

func managedSnapshot(snapshot ResourceSnapshot) (ResourceSnapshot, error) {
	if !isManagedResource(snapshot.Labels) {
		return ResourceSnapshot{}, fmt.Errorf("resource is not managed by Luna DevOps")
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
	listOptions := metav1.ListOptions{LabelSelector: selector}
	deployments, err := listResourceSnapshots(ctx, listOptions, func(ctx context.Context, listOptions metav1.ListOptions) (k8sruntime.Object, error) {
		return c.client.AppsV1().Deployments(options.Namespace).List(ctx, listOptions)
	}, func(object k8sruntime.Object) ResourceSnapshot {
		return deploymentSnapshot(*object.(*appsv1.Deployment))
	})
	if err != nil {
		return nil, err
	}
	statefulSets, err := listResourceSnapshots(ctx, listOptions, func(ctx context.Context, listOptions metav1.ListOptions) (k8sruntime.Object, error) {
		return c.client.AppsV1().StatefulSets(options.Namespace).List(ctx, listOptions)
	}, func(object k8sruntime.Object) ResourceSnapshot {
		return statefulSetSnapshot(*object.(*appsv1.StatefulSet))
	})
	if err != nil {
		return nil, err
	}
	pods, err := listResourceSnapshots(ctx, listOptions, func(ctx context.Context, listOptions metav1.ListOptions) (k8sruntime.Object, error) {
		return c.client.CoreV1().Pods(options.Namespace).List(ctx, listOptions)
	}, func(object k8sruntime.Object) ResourceSnapshot {
		return podSnapshot(*object.(*corev1.Pod))
	})
	if err != nil {
		return nil, err
	}
	hpas, err := listResourceSnapshots(ctx, listOptions, func(ctx context.Context, listOptions metav1.ListOptions) (k8sruntime.Object, error) {
		return c.client.AutoscalingV2().HorizontalPodAutoscalers(options.Namespace).List(ctx, listOptions)
	}, func(object k8sruntime.Object) ResourceSnapshot {
		return hpaSnapshot(*object.(*autoscalingv2.HorizontalPodAutoscaler))
	})
	if err != nil {
		return nil, err
	}
	items := make([]ResourceSnapshot, 0, len(deployments)+len(statefulSets)+len(pods)+len(hpas))
	items = append(items, deployments...)
	items = append(items, statefulSets...)
	items = append(items, hpas...)
	items = append(items, pods...)
	return items, nil
}

func (c *Client) listManagedServicesAndRoutes(ctx context.Context, options ResourceListOptions) ([]ResourceSnapshot, error) {
	selector := managedResourceSelector(options)
	services, err := listResourceSnapshots(ctx, metav1.ListOptions{LabelSelector: selector}, func(ctx context.Context, listOptions metav1.ListOptions) (k8sruntime.Object, error) {
		return c.client.CoreV1().Services(options.Namespace).List(ctx, listOptions)
	}, func(object k8sruntime.Object) ResourceSnapshot {
		return serviceSnapshot(*object.(*corev1.Service))
	})
	if err != nil {
		return nil, err
	}
	if c.dynamic == nil {
		return services, nil
	}
	httpRoutes, err := c.listGatewayAPIResources(ctx, httpRouteGVR, options.Namespace, selector, httpRouteSnapshot)
	if err != nil {
		return nil, err
	}
	gateways, err := c.listGatewayAPIResources(ctx, gatewayGVR, options.Namespace, selector, gatewaySnapshot)
	if err != nil {
		return nil, err
	}
	items := make([]ResourceSnapshot, 0, len(services)+len(httpRoutes)+len(gateways))
	items = append(items, services...)
	items = append(items, httpRoutes...)
	items = append(items, gateways...)
	return items, nil
}

func (c *Client) listGatewayAPIResources(
	ctx context.Context,
	gvr schema.GroupVersionResource,
	namespace string,
	selector string,
	snapshot func(*unstructured.Unstructured) ResourceSnapshot,
) ([]ResourceSnapshot, error) {
	if c.dynamic == nil {
		return []ResourceSnapshot{}, nil
	}
	items, err := listResourceSnapshots(ctx, metav1.ListOptions{LabelSelector: selector}, func(ctx context.Context, listOptions metav1.ListOptions) (k8sruntime.Object, error) {
		return c.dynamic.Resource(gvr).Namespace(namespace).List(ctx, listOptions)
	}, func(object k8sruntime.Object) ResourceSnapshot {
		return snapshot(object.(*unstructured.Unstructured))
	})
	if apierrors.IsNotFound(err) {
		return []ResourceSnapshot{}, nil
	}
	return items, err
}

func (c *Client) getGatewayAPIResource(ctx context.Context, gvr schema.GroupVersionResource, namespace string, name string) (*unstructured.Unstructured, error) {
	if c.dynamic == nil {
		return nil, fmt.Errorf("Gateway API resources require a dynamic Kubernetes client")
	}
	item, err := c.dynamic.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("Gateway API CRDs are not installed or %s %q does not exist", gvr.Resource, name)
	}
	return item, err
}

func (c *Client) listManagedConfigs(ctx context.Context, options ResourceListOptions) ([]ResourceSnapshot, error) {
	selector := managedResourceSelector(options)
	listOptions := metav1.ListOptions{LabelSelector: selector}
	configMaps, err := listResourceSnapshots(ctx, listOptions, func(ctx context.Context, listOptions metav1.ListOptions) (k8sruntime.Object, error) {
		return c.client.CoreV1().ConfigMaps(options.Namespace).List(ctx, listOptions)
	}, func(object k8sruntime.Object) ResourceSnapshot {
		item := object.(*corev1.ConfigMap)
		return snapshotFromMeta("ConfigMap", item.ObjectMeta, "", fmt.Sprintf("%d keys", len(item.Data)), "")
	})
	if err != nil {
		return nil, err
	}
	secrets, err := listResourceSnapshots(ctx, listOptions, func(ctx context.Context, listOptions metav1.ListOptions) (k8sruntime.Object, error) {
		return c.client.CoreV1().Secrets(options.Namespace).List(ctx, listOptions)
	}, func(object k8sruntime.Object) ResourceSnapshot {
		item := object.(*corev1.Secret)
		return snapshotFromMeta("Secret", item.ObjectMeta, "", string(item.Type), "data hidden")
	})
	if err != nil {
		return nil, err
	}
	items := make([]ResourceSnapshot, 0, len(configMaps)+len(secrets))
	items = append(items, configMaps...)
	items = append(items, secrets...)
	return items, nil
}

func (c *Client) listManagedStorage(ctx context.Context, options ResourceListOptions) ([]ResourceSnapshot, error) {
	return listResourceSnapshots(
		ctx,
		metav1.ListOptions{LabelSelector: managedResourceSelector(options)},
		func(ctx context.Context, listOptions metav1.ListOptions) (k8sruntime.Object, error) {
			return c.client.CoreV1().PersistentVolumeClaims(options.Namespace).List(ctx, listOptions)
		},
		func(object k8sruntime.Object) ResourceSnapshot {
			item := object.(*corev1.PersistentVolumeClaim)
			return snapshotFromMeta("PersistentVolumeClaim", item.ObjectMeta, "", item.Status.Phase, pvcSummary(*item))
		},
	)
}

func deploymentSnapshot(item appsv1.Deployment) ResourceSnapshot {
	desired := int32(0)
	if item.Spec.Replicas != nil {
		desired = *item.Spec.Replicas
	}
	status := "progressing"
	if desired == 0 && item.Status.ObservedGeneration >= item.Generation {
		status = "scaled-to-zero"
	} else if desired > 0 && item.Status.ObservedGeneration >= item.Generation && item.Status.ReadyReplicas >= desired && item.Status.AvailableReplicas >= desired {
		status = "ready"
	}
	return snapshotFromMeta("Deployment", item.ObjectMeta, "", status, fmt.Sprintf("ready %d/%d", item.Status.ReadyReplicas, desired))
}

func statefulSetSnapshot(item appsv1.StatefulSet) ResourceSnapshot {
	desired := int32(0)
	if item.Spec.Replicas != nil {
		desired = *item.Spec.Replicas
	}
	status := "progressing"
	if desired == 0 && item.Status.ObservedGeneration >= item.Generation {
		status = "scaled-to-zero"
	} else if desired > 0 && item.Status.ObservedGeneration >= item.Generation && item.Status.ReadyReplicas >= desired {
		status = "ready"
	}
	return snapshotFromMeta("StatefulSet", item.ObjectMeta, "", status, fmt.Sprintf("ready %d/%d", item.Status.ReadyReplicas, desired))
}

func hpaSnapshot(item autoscalingv2.HorizontalPodAutoscaler) ResourceSnapshot {
	summary := fmt.Sprintf("%d-%d replicas", firstNonNilInt32(item.Spec.MinReplicas, 1), item.Spec.MaxReplicas)
	return snapshotFromMeta("HorizontalPodAutoscaler", item.ObjectMeta, "", fmt.Sprintf("%d current", item.Status.CurrentReplicas), summary)
}

func firstNonNilInt32(value *int32, fallbackValue int32) int32 {
	if value == nil {
		return fallbackValue
	}
	return *value
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

func httpRouteSnapshot(item *unstructured.Unstructured) ResourceSnapshot {
	hostnames := make([]string, 0)
	if values, ok, _ := unstructured.NestedStringSlice(item.Object, "spec", "hostnames"); ok {
		hostnames = append(hostnames, values...)
	}
	summary := strings.Join(hostnames, ", ")
	if summary == "" {
		parentRefs, _, _ := unstructured.NestedSlice(item.Object, "spec", "parentRefs")
		summary = fmt.Sprintf("%d parent refs", len(parentRefs))
	}
	status := httpRouteSummary(routeConditionsFromUnstructured(item))
	return snapshotFromMeta("HTTPRoute", metav1.ObjectMeta{
		Name:              item.GetName(),
		Namespace:         item.GetNamespace(),
		Labels:            item.GetLabels(),
		CreationTimestamp: item.GetCreationTimestamp(),
		ManagedFields:     item.GetManagedFields(),
	}, "", status, summary)
}

func gatewaySnapshot(item *unstructured.Unstructured) ResourceSnapshot {
	listeners, _, _ := unstructured.NestedSlice(item.Object, "spec", "listeners")
	summary := make([]string, 0, len(listeners))
	for _, raw := range listeners {
		listener, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name, _ := listener["name"].(string)
		protocol, _ := listener["protocol"].(string)
		port := fmt.Sprint(listener["port"])
		summary = append(summary, strings.TrimSpace(name+" "+protocol+":"+port))
	}
	status := gatewayProgrammedStatus(item)
	return snapshotFromMeta("Gateway", metav1.ObjectMeta{
		Name:              item.GetName(),
		Namespace:         item.GetNamespace(),
		Labels:            item.GetLabels(),
		CreationTimestamp: item.GetCreationTimestamp(),
		ManagedFields:     item.GetManagedFields(),
	}, "", status, strings.Join(compactStrings(summary), ", "))
}

func gatewayProgrammedStatus(item *unstructured.Unstructured) string {
	listeners, _, _ := unstructured.NestedSlice(item.Object, "status", "listeners")
	for _, raw := range listeners {
		listener, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		conditions, ok := listener["conditions"].([]any)
		if !ok {
			continue
		}
		for _, rawCondition := range conditions {
			condition, ok := rawCondition.(map[string]any)
			if !ok {
				continue
			}
			if condition["type"] == "Programmed" && condition["status"] == "True" {
				return "programmed"
			}
			if condition["type"] == "Programmed" && condition["status"] == "False" {
				return "pending"
			}
		}
	}
	conditions, _, _ := unstructured.NestedSlice(item.Object, "status", "conditions")
	for _, rawCondition := range conditions {
		condition, ok := rawCondition.(map[string]any)
		if !ok {
			continue
		}
		if condition["type"] == "Programmed" && condition["status"] == "True" {
			return "programmed"
		}
		if condition["type"] == "Accepted" && condition["status"] == "False" {
			return "failed"
		}
	}
	return "pending"
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

func normalizeResourceKind(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "namespace", "namespaces":
		return "namespaces"
	case "workload", "workloads":
		return "workloads"
	case "service", "services", "httproute", "httproutes", "gateway", "gateways":
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
	case "statefulset", "statefulsets", "sts":
		return "statefulset"
	case "pod", "pods":
		return "pod"
	case "horizontalpodautoscaler", "horizontalpodautoscalers", "hpa", "hpas":
		return "horizontalpodautoscaler"
	case "service", "services":
		return "service"
	case "httproute", "httproutes":
		return "httproute"
	case "gateway", "gateways":
		return "gateway"
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
