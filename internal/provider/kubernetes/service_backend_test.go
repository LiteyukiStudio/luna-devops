package kubernetes

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func TestGetServiceBackendSnapshotReportsReadyBackend(t *testing.T) {
	serviceName := "dplt-demo"
	portName := "admin"
	servicePort := int32(3002)
	ready := true
	client := NewClientForInterfaces(kubefake.NewSimpleClientset(
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: serviceName, Namespace: "ns-demo"},
			Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{
				Name:       portName,
				Port:       servicePort,
				TargetPort: intstr.FromInt32(servicePort),
			}}},
		},
		&discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceName + "-abc",
				Namespace: "ns-demo",
				Labels:    map[string]string{"kubernetes.io/service-name": serviceName},
			},
			Ports: []discoveryv1.EndpointPort{{Name: &portName, Port: &servicePort}},
			Endpoints: []discoveryv1.Endpoint{{
				Addresses:  []string{"10.42.0.10"},
				Conditions: discoveryv1.EndpointConditions{Ready: &ready},
			}},
		},
	), nil)

	snapshot, err := client.GetServiceBackendSnapshot(context.Background(), "ns-demo", serviceName, servicePort)
	if err != nil {
		t.Fatalf("GetServiceBackendSnapshot returned error: %v", err)
	}
	if !snapshot.ServiceExists || !snapshot.PortExists || snapshot.ReadyEndpoints != 1 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestGetServiceBackendSnapshotReportsMissingService(t *testing.T) {
	client := NewClientForInterfaces(kubefake.NewSimpleClientset(), nil)

	snapshot, err := client.GetServiceBackendSnapshot(context.Background(), "ns-demo", "dplt-missing", 8080)
	if err != nil {
		t.Fatalf("GetServiceBackendSnapshot returned error: %v", err)
	}
	if snapshot.ServiceExists || snapshot.PortExists {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestGetServiceBackendSnapshotReportsMissingPort(t *testing.T) {
	client := NewClientForInterfaces(kubefake.NewSimpleClientset(&corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "dplt-demo", Namespace: "ns-demo"},
		Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Name: "web", Port: 3001}}},
	}), nil)

	snapshot, err := client.GetServiceBackendSnapshot(context.Background(), "ns-demo", "dplt-demo", 3002)
	if err != nil {
		t.Fatalf("GetServiceBackendSnapshot returned error: %v", err)
	}
	if !snapshot.ServiceExists || snapshot.PortExists {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}
