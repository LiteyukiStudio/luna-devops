package kubepolicy

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

func ValidateService(policy PolicyContext, service *corev1.Service) field.ErrorList {
	if service == nil {
		return field.ErrorList{field.Required(field.NewPath("object"), "service is required")}
	}
	path := field.NewPath("spec")
	errors := field.ErrorList{}
	serviceType := service.Spec.Type
	if serviceType == "" {
		serviceType = corev1.ServiceTypeClusterIP
	}
	if serviceType != corev1.ServiceTypeClusterIP {
		errors = append(errors, field.NotSupported(path.Child("type"), serviceType, []string{string(corev1.ServiceTypeClusterIP)}))
	}
	if len(service.Spec.ExternalIPs) > 0 {
		errors = append(errors, field.Forbidden(path.Child("externalIPs"), "external IPs are not allowed"))
	}
	for index, port := range service.Spec.Ports {
		if port.NodePort != 0 {
			errors = append(errors, field.Forbidden(path.Child("ports").Index(index).Child("nodePort"), "node ports are not allowed"))
		}
	}
	if len(service.Spec.Selector) > 0 {
		for key, expected := range RequiredSelectionLabels(policy) {
			if service.Spec.Selector[key] != expected {
				errors = append(errors, field.Invalid(path.Child("selector").Key(key), service.Spec.Selector[key], "selector must include the binding ownership label"))
			}
		}
	}
	return errors
}
