package kubernetes

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestSelectPodContainer(t *testing.T) {
	pod := corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app"},
				{Name: "sidecar"},
			},
		},
	}

	selected, err := selectPodContainer(pod, "")
	if err != nil {
		t.Fatalf("selectPodContainer returned error: %v", err)
	}
	if selected != "app" {
		t.Fatalf("selectPodContainer default = %q, want app", selected)
	}

	selected, err = selectPodContainer(pod, "sidecar")
	if err != nil {
		t.Fatalf("selectPodContainer sidecar returned error: %v", err)
	}
	if selected != "sidecar" {
		t.Fatalf("selectPodContainer sidecar = %q, want sidecar", selected)
	}

	if _, err := selectPodContainer(pod, "missing"); err == nil {
		t.Fatal("expected missing container to fail")
	}
}
