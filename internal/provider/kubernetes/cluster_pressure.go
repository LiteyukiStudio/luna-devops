package kubernetes

import (
	"context"
	"fmt"
	"time"

	"github.com/LiteyukiStudio/devops/internal/telemetry"

	"go.opentelemetry.io/otel/trace"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var nodeMetricsResource = schema.GroupVersionResource{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "nodes"}

// ClusterPressureSnapshot is one point-in-time view of scheduler allocation and
// optional node usage. It is never persisted or cached as current state.
type ClusterPressureSnapshot struct {
	NodeCount              int
	PodCount               int
	CPURequestsMilli       int64
	CPUAllocatableMilli    int64
	CPUUsageMilli          int64
	MemoryRequestsBytes    int64
	MemoryAllocatableBytes int64
	MemoryUsageBytes       int64
	MetricsAvailable       bool
	ObservedAt             time.Time
}

func (c *Client) ClusterPressure(ctx context.Context) (snapshot ClusterPressureSnapshot, err error) {
	ctx, end := telemetry.StartOperationWithKind(ctx, "kubernetes", "observe_cluster_pressure", trace.SpanKindClient)
	defer func() { end(err) }()

	nodes, err := c.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return snapshot, fmt.Errorf("list nodes for cluster pressure: %w", err)
	}
	if len(nodes.Items) == 0 {
		return snapshot, fmt.Errorf("cluster has no nodes")
	}

	nodeNames := make(map[string]struct{}, len(nodes.Items))
	for i := range nodes.Items {
		node := &nodes.Items[i]
		nodeNames[node.Name] = struct{}{}
		snapshot.CPUAllocatableMilli += quantityMilli(node.Status.Allocatable, corev1.ResourceCPU)
		snapshot.MemoryAllocatableBytes += quantityValue(node.Status.Allocatable, corev1.ResourceMemory)
	}
	if snapshot.CPUAllocatableMilli <= 0 || snapshot.MemoryAllocatableBytes <= 0 {
		return snapshot, fmt.Errorf("cluster allocatable CPU or memory is unavailable")
	}

	pods, err := c.client.CoreV1().Pods(corev1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return snapshot, fmt.Errorf("list pods for cluster pressure: %w", err)
	}
	for i := range pods.Items {
		pod := &pods.Items[i]
		if _, exists := nodeNames[pod.Spec.NodeName]; !exists || podTerminal(pod) {
			continue
		}
		snapshot.PodCount++
		snapshot.CPURequestsMilli += effectivePodRequest(pod, corev1.ResourceCPU, true)
		snapshot.MemoryRequestsBytes += effectivePodRequest(pod, corev1.ResourceMemory, false)
	}

	snapshot.NodeCount = len(nodes.Items)
	snapshot.ObservedAt = time.Now().UTC()
	c.observeNodeUsage(ctx, nodeNames, &snapshot)
	return snapshot, nil
}

func (c *Client) observeNodeUsage(ctx context.Context, nodeNames map[string]struct{}, snapshot *ClusterPressureSnapshot) {
	if c.dynamic == nil {
		return
	}
	metrics, err := c.dynamic.Resource(nodeMetricsResource).List(ctx, metav1.ListOptions{})
	if err != nil {
		return
	}
	observed := 0
	for i := range metrics.Items {
		item := &metrics.Items[i]
		if _, exists := nodeNames[item.GetName()]; !exists {
			continue
		}
		cpuRaw, cpuFound, cpuReadErr := unstructured.NestedString(item.Object, "usage", string(corev1.ResourceCPU))
		memoryRaw, memoryFound, memoryReadErr := unstructured.NestedString(item.Object, "usage", string(corev1.ResourceMemory))
		if cpuReadErr != nil || memoryReadErr != nil || !cpuFound || !memoryFound {
			return
		}
		cpu, cpuErr := resource.ParseQuantity(cpuRaw)
		memory, memoryErr := resource.ParseQuantity(memoryRaw)
		if cpuErr != nil || memoryErr != nil {
			return
		}
		snapshot.CPUUsageMilli += cpu.MilliValue()
		snapshot.MemoryUsageBytes += memory.Value()
		observed++
	}
	if observed != len(nodeNames) {
		snapshot.CPUUsageMilli = 0
		snapshot.MemoryUsageBytes = 0
		return
	}
	snapshot.MetricsAvailable = true
}

func effectivePodRequest(pod *corev1.Pod, resourceName corev1.ResourceName, milli bool) int64 {
	regular := int64(0)
	for i := range pod.Spec.Containers {
		regular += containerRequest(pod.Spec.Containers[i], resourceName, milli)
	}

	restartableInit := int64(0)
	maxInit := int64(0)
	for i := range pod.Spec.InitContainers {
		container := pod.Spec.InitContainers[i]
		request := containerRequest(container, resourceName, milli)
		candidate := request + restartableInit
		if container.RestartPolicy != nil && *container.RestartPolicy == corev1.ContainerRestartPolicyAlways {
			regular += request
			restartableInit += request
			candidate = restartableInit
		}
		maxInit = max(maxInit, candidate)
	}

	effective := max(regular, maxInit)
	if pod.Spec.Resources != nil {
		if quantity, exists := pod.Spec.Resources.Requests[resourceName]; exists {
			effective = quantityValueByMode(quantity, milli)
		}
	}
	if overhead, exists := pod.Spec.Overhead[resourceName]; exists {
		effective += quantityValueByMode(overhead, milli)
	}
	return effective
}

func containerRequest(container corev1.Container, resourceName corev1.ResourceName, milli bool) int64 {
	quantity, exists := container.Resources.Requests[resourceName]
	if !exists {
		return 0
	}
	return quantityValueByMode(quantity, milli)
}

func quantityMilli(resources corev1.ResourceList, name corev1.ResourceName) int64 {
	quantity, exists := resources[name]
	if !exists {
		return 0
	}
	return quantity.MilliValue()
}

func quantityValue(resources corev1.ResourceList, name corev1.ResourceName) int64 {
	quantity, exists := resources[name]
	if !exists {
		return 0
	}
	return quantity.Value()
}

func quantityValueByMode(quantity resource.Quantity, milli bool) int64 {
	if milli {
		return quantity.MilliValue()
	}
	return quantity.Value()
}

func podTerminal(pod *corev1.Pod) bool {
	return pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed
}
