package kubernetes

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func applicationServicePorts(spec ApplicationResourcesSpec) []ApplicationServicePort {
	ports := make([]ApplicationServicePort, 0, len(spec.ServicePorts))
	seen := map[int32]bool{}
	seenNames := map[string]int{}
	for _, item := range spec.ServicePorts {
		if item.Port <= 0 || item.Port > 65535 || seen[item.Port] {
			continue
		}
		seen[item.Port] = true
		name := dnsLabel(firstNonEmpty(strings.TrimSpace(item.Name), fmt.Sprintf("port-%d", item.Port)))
		if name == "" {
			name = fmt.Sprintf("port-%d", item.Port)
		}
		if count := seenNames[name]; count > 0 {
			suffix := fmt.Sprintf("-%d", count+1)
			if len(name)+len(suffix) > 63 {
				name = strings.Trim(name[:63-len(suffix)], "-")
			}
			name = fmt.Sprintf("%s%s", name, suffix)
		}
		seenNames[name]++
		ports = append(ports, ApplicationServicePort{Name: name, Port: item.Port, AppProtocol: strings.TrimSpace(item.AppProtocol)})
	}
	if len(ports) == 0 {
		port := spec.ServicePort
		if port <= 0 {
			port = 8080
		}
		ports = append(ports, ApplicationServicePort{Name: "http", Port: port})
	}
	return ports
}

func containerPorts(spec ApplicationResourcesSpec) []corev1.ContainerPort {
	ports := applicationServicePorts(spec)
	result := make([]corev1.ContainerPort, 0, len(ports))
	for _, item := range ports {
		result = append(result, corev1.ContainerPort{Name: item.Name, ContainerPort: item.Port})
	}
	return result
}

func servicePorts(spec ApplicationResourcesSpec) []corev1.ServicePort {
	ports := applicationServicePorts(spec)
	result := make([]corev1.ServicePort, 0, len(ports))
	for _, item := range ports {
		result = append(result, corev1.ServicePort{
			Name:        item.Name,
			Port:        item.Port,
			TargetPort:  intstrFromInt32(item.Port),
			AppProtocol: stringPtrOrNil(item.AppProtocol),
		})
	}
	return result
}

func (c *Client) applyService(ctx context.Context, spec ApplicationResourcesSpec, labels map[string]string, selectorLabels map[string]string) error {
	annotations := mustApplicationServiceAnnotations(spec)
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: spec.Name, Namespace: spec.Namespace, Labels: labels, Annotations: annotations},
		Spec: corev1.ServiceSpec{
			Type:                  applicationServiceType(spec),
			Selector:              selectorLabels,
			Ports:                 servicePorts(spec),
			ExternalTrafficPolicy: applicationServiceExternalTrafficPolicy(spec),
			SessionAffinity:       applicationServiceSessionAffinity(spec),
		},
	}
	existing, err := c.client.CoreV1().Services(spec.Namespace).Get(ctx, spec.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = c.client.CoreV1().Services(spec.Namespace).Create(ctx, service, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if err := ensureResourceOwnership("Service", existing, labels); err != nil {
		return err
	}
	existing.Labels = labels
	existing.Annotations = annotations
	existing.Spec.Type = service.Spec.Type
	existing.Spec.Selector = selectorLabels
	existing.Spec.Ports = service.Spec.Ports
	existing.Spec.ExternalTrafficPolicy = service.Spec.ExternalTrafficPolicy
	existing.Spec.SessionAffinity = service.Spec.SessionAffinity
	_, err = c.client.CoreV1().Services(spec.Namespace).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

func applicationServiceType(spec ApplicationResourcesSpec) corev1.ServiceType {
	switch strings.TrimSpace(spec.ServiceType) {
	case string(corev1.ServiceTypeNodePort):
		return corev1.ServiceTypeNodePort
	case string(corev1.ServiceTypeLoadBalancer):
		return corev1.ServiceTypeLoadBalancer
	default:
		return corev1.ServiceTypeClusterIP
	}
}

func applicationServiceExternalTrafficPolicy(spec ApplicationResourcesSpec) corev1.ServiceExternalTrafficPolicy {
	serviceType := applicationServiceType(spec)
	if serviceType != corev1.ServiceTypeNodePort && serviceType != corev1.ServiceTypeLoadBalancer {
		return ""
	}
	switch strings.TrimSpace(spec.ServiceExternalTrafficPolicy) {
	case string(corev1.ServiceExternalTrafficPolicyLocal):
		return corev1.ServiceExternalTrafficPolicyLocal
	case string(corev1.ServiceExternalTrafficPolicyCluster):
		return corev1.ServiceExternalTrafficPolicyCluster
	default:
		return ""
	}
}

func applicationServiceSessionAffinity(spec ApplicationResourcesSpec) corev1.ServiceAffinity {
	switch strings.TrimSpace(spec.ServiceSessionAffinity) {
	case string(corev1.ServiceAffinityClientIP):
		return corev1.ServiceAffinityClientIP
	default:
		return corev1.ServiceAffinityNone
	}
}

func applicationServiceAnnotations(spec ApplicationResourcesSpec) (map[string]string, error) {
	return stringMapFromJSONOrLines(spec.ServiceAnnotations, "service annotations")
}

func mustApplicationServiceAnnotations(spec ApplicationResourcesSpec) map[string]string {
	values, err := applicationServiceAnnotations(spec)
	if err != nil {
		panic(err)
	}
	return values
}
