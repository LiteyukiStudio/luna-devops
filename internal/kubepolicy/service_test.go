package kubepolicy

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestServiceAllowsOnlyClusterIP(t *testing.T) {
	service := &corev1.Service{Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeNodePort, ExternalIPs: []string{"192.0.2.1"}}}
	if errors := ValidateService(PolicyContext{ProjectID: "p1"}, service); len(errors) < 2 {
		t.Fatalf("expected service policy errors, got %#v", errors)
	}
}
