package kubernetes

import (
	"reflect"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/resourcepolicy"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func TestPodTemplateResourcesSumsRegularContainers(t *testing.T) {
	container := func(cpuRequest, memoryRequest, cpuLimit, memoryLimit string) corev1.Container {
		requirements := corev1.ResourceRequirements{Requests: corev1.ResourceList{}, Limits: corev1.ResourceList{}}
		for name, value := range map[corev1.ResourceName]string{corev1.ResourceCPU: cpuRequest, corev1.ResourceMemory: memoryRequest} {
			if value != "" {
				requirements.Requests[name] = resource.MustParse(value)
			}
		}
		for name, value := range map[corev1.ResourceName]string{corev1.ResourceCPU: cpuLimit, corev1.ResourceMemory: memoryLimit} {
			if value != "" {
				requirements.Limits[name] = resource.MustParse(value)
			}
		}
		return corev1.Container{Resources: requirements}
	}
	policy := resourcepolicy.Policy{CPURequestPercent: 10, MemoryRequestPercent: 25, CPULimitPercent: 100, MemoryLimitPercent: 100}
	for _, test := range []struct {
		name        string
		annotations map[string]string
		containers  []corev1.Container
		initOnly    []corev1.Container
		want        PodTemplateResources
	}{
		{
			name:        "application container with render policy",
			annotations: map[string]string{ResourcePolicyAnnotation: EncodeResourcePolicyAnnotation(policy)},
			containers:  []corev1.Container{container("100m", "256Mi", "1", "1Gi")},
			want:        PodTemplateResources{CPURequestMilli: 100, MemoryRequestBytes: 256 << 20, CPULimitMilli: 1000, MemoryLimitBytes: 1 << 30, Policy: &policy},
		},
		{
			name:       "sidecar requests and limits are included",
			containers: []corev1.Container{container("100m", "256Mi", "1", "1Gi"), container("50m", "64Mi", "200m", "128Mi")},
			want:       PodTemplateResources{CPURequestMilli: 150, MemoryRequestBytes: 320 << 20, CPULimitMilli: 1200, MemoryLimitBytes: 1152 << 20},
		},
		{
			name:       "a container without limits makes the pod unbounded",
			containers: []corev1.Container{container("100m", "256Mi", "1", "1Gi"), container("50m", "", "", "128Mi")},
			want:       PodTemplateResources{CPURequestMilli: 150, MemoryRequestBytes: 256 << 20, CPULimitMilli: 0, MemoryLimitBytes: 1152 << 20},
		},
		{
			name:       "init containers are not running capacity",
			containers: []corev1.Container{container("", "", "", "")},
			initOnly:   []corev1.Container{container("2", "2Gi", "2", "2Gi")},
			want:       PodTemplateResources{},
		},
		{
			name:        "invalid policy annotation is not recorded",
			annotations: map[string]string{ResourcePolicyAnnotation: `{"cpuRequestPercent":200}`},
			containers:  []corev1.Container{container("100m", "", "", "")},
			want:        PodTemplateResources{CPURequestMilli: 100},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			template := corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Annotations: test.annotations},
				Spec:       corev1.PodSpec{Containers: test.containers, InitContainers: test.initOnly},
			}
			if got := podTemplateResources(template); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("podTemplateResources() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestApplicationPodTemplateAnnotatesRenderPolicy(t *testing.T) {
	spec := ApplicationResourcesSpec{Name: "app", Image: "example.invalid/app:1", ResourcePolicy: `{"cpuRequestPercent":10}`}
	template := applicationPodTemplate(spec, map[string]string{}, map[string]string{})
	if got := template.Annotations[ResourcePolicyAnnotation]; got != spec.ResourcePolicy {
		t.Fatalf("resource policy annotation = %q, want %q", got, spec.ResourcePolicy)
	}
}

func TestRuntimeMetricsPodSelectorExcludesHookJobPods(t *testing.T) {
	selector, err := labels.Parse(runtimeMetricsPodLabelSelector("dplt_metrics"))
	if err != nil {
		t.Fatalf("parse selector: %v", err)
	}
	workload := labels.Set{ManagedByLabel: ManagedByValue, DeploymentTargetIDLabel: "dplt_metrics", ScopeLabel: "project"}
	hook := labels.Set{ManagedByLabel: ManagedByValue, DeploymentTargetIDLabel: "dplt_metrics", HookRunIDLabel: "hrun_1"}
	if !selector.Matches(workload) || selector.Matches(hook) {
		t.Fatalf("selector %q: workload=%v hook=%v", selector, selector.Matches(workload), selector.Matches(hook))
	}
}
