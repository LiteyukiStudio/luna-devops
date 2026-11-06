package kubernetes

import (
	"encoding/json"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/resourcepolicy"
	corev1 "k8s.io/api/core/v1"
)

// ResourcePolicyAnnotation records, on the rendered Pod template, the runtime
// cluster resource policy that produced the container requests and limits.
const ResourcePolicyAnnotation = "luna.devops/resource-policy"

// PodTemplateResources is the per-replica resource envelope that Kubernetes
// actually runs: the sum over every regular container in the workload Pod
// template (application container plus sidecars). metrics.k8s.io usage is
// summed over the same container set, so usage, request floor and limit are
// directly comparable.
type PodTemplateResources struct {
	CPURequestMilli    int64
	MemoryRequestBytes int64
	// CPULimitMilli and MemoryLimitBytes are 0 when any container omits the
	// limit, because the Pod as a whole is then unbounded for that resource.
	CPULimitMilli    int64
	MemoryLimitBytes int64
	// Policy is the render-time cluster policy; nil when the workload was
	// rendered before the policy annotation existed.
	Policy *resourcepolicy.Policy
}

// EncodeResourcePolicyAnnotation serializes a resource policy for the Pod
// template annotation.
func EncodeResourcePolicyAnnotation(policy resourcepolicy.Policy) string {
	payload, err := json.Marshal(policy)
	if err != nil {
		return ""
	}
	return string(payload)
}

func podTemplateResources(template corev1.PodTemplateSpec) PodTemplateResources {
	result := PodTemplateResources{Policy: parseResourcePolicyAnnotation(template.Annotations[ResourcePolicyAnnotation])}
	cpuLimitBounded, memoryLimitBounded := len(template.Spec.Containers) > 0, len(template.Spec.Containers) > 0
	for _, container := range template.Spec.Containers {
		if quantity, ok := container.Resources.Requests[corev1.ResourceCPU]; ok {
			result.CPURequestMilli += quantity.MilliValue()
		}
		if quantity, ok := container.Resources.Requests[corev1.ResourceMemory]; ok {
			result.MemoryRequestBytes += quantity.Value()
		}
		if quantity, ok := container.Resources.Limits[corev1.ResourceCPU]; ok && quantity.Sign() > 0 {
			result.CPULimitMilli += quantity.MilliValue()
		} else {
			cpuLimitBounded = false
		}
		if quantity, ok := container.Resources.Limits[corev1.ResourceMemory]; ok && quantity.Sign() > 0 {
			result.MemoryLimitBytes += quantity.Value()
		} else {
			memoryLimitBounded = false
		}
	}
	if !cpuLimitBounded {
		result.CPULimitMilli = 0
	}
	if !memoryLimitBounded {
		result.MemoryLimitBytes = 0
	}
	return result
}

func parseResourcePolicyAnnotation(value string) *resourcepolicy.Policy {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	var policy resourcepolicy.Policy
	if err := json.Unmarshal([]byte(value), &policy); err != nil || policy.Validate() != nil {
		return nil
	}
	return &policy
}
