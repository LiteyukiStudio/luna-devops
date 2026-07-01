package kubernetes

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ServiceBackendSnapshot struct {
	ServiceExists  bool
	PortExists     bool
	ReadyEndpoints int
	ServicePorts   []int32
}

func (c *Client) GetServiceBackendSnapshot(ctx context.Context, namespace, name string, servicePort int32) (ServiceBackendSnapshot, error) {
	namespace = strings.TrimSpace(namespace)
	name = strings.TrimSpace(name)
	if namespace == "" || name == "" || servicePort <= 0 {
		return ServiceBackendSnapshot{}, nil
	}
	service, err := c.client.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return ServiceBackendSnapshot{ServiceExists: false}, nil
	}
	if err != nil {
		return ServiceBackendSnapshot{}, err
	}
	snapshot := ServiceBackendSnapshot{ServiceExists: true}
	matchingPortNames := map[string]bool{}
	matchingTargetPorts := map[int32]bool{}
	for _, port := range service.Spec.Ports {
		snapshot.ServicePorts = append(snapshot.ServicePorts, port.Port)
		if port.Port != servicePort {
			continue
		}
		snapshot.PortExists = true
		if strings.TrimSpace(port.Name) != "" {
			matchingPortNames[port.Name] = true
		}
		if port.TargetPort.IntVal > 0 {
			matchingTargetPorts[port.TargetPort.IntVal] = true
		}
	}
	if !snapshot.PortExists {
		return snapshot, nil
	}
	snapshot.ReadyEndpoints = c.readyEndpointCount(ctx, namespace, service, servicePort, matchingPortNames, matchingTargetPorts)
	return snapshot, nil
}

func (c *Client) readyEndpointCount(ctx context.Context, namespace string, service *corev1.Service, servicePort int32, matchingPortNames map[string]bool, matchingTargetPorts map[int32]bool) int {
	slices, err := c.client.DiscoveryV1().EndpointSlices(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "kubernetes.io/service-name=" + service.Name,
	})
	if err != nil {
		return 0
	}
	count := 0
	for _, slice := range slices.Items {
		if !endpointSliceMatchesPort(slice.Ports, servicePort, matchingPortNames, matchingTargetPorts) {
			continue
		}
		for _, endpoint := range slice.Endpoints {
			if endpoint.Conditions.Ready != nil && !*endpoint.Conditions.Ready {
				continue
			}
			count += len(endpoint.Addresses)
		}
	}
	return count
}

func endpointSliceMatchesPort(ports []discoveryv1.EndpointPort, servicePort int32, matchingPortNames map[string]bool, matchingTargetPorts map[int32]bool) bool {
	if len(ports) == 0 {
		return true
	}
	for _, port := range ports {
		if port.Name != nil && matchingPortNames[*port.Name] {
			return true
		}
		if port.Port != nil && (*port.Port == servicePort || matchingTargetPorts[*port.Port]) {
			return true
		}
	}
	return false
}
