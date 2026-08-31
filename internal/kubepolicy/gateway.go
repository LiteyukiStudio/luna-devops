package kubepolicy

import (
	"context"
	"fmt"
	"strings"

	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

func ValidateGateway(ctx context.Context, policy PolicyContext, object *unstructured.Unstructured) field.ErrorList {
	switch object.GetKind() {
	case "Ingress":
		var ingress networkingv1.Ingress
		if err := fromUnstructured(object, &ingress); err != nil {
			return conversionError(err)
		}
		return validateIngress(ctx, policy, &ingress)
	case "HTTPRoute", "GRPCRoute":
		return validateGatewayRoute(ctx, policy, object)
	default:
		return field.ErrorList{field.NotSupported(field.NewPath("kind"), object.GetKind(), []string{"Ingress", "HTTPRoute", "GRPCRoute"})}
	}
}

func validateIngress(ctx context.Context, policy PolicyContext, ingress *networkingv1.Ingress) field.ErrorList {
	errors := field.ErrorList{}
	if ingress.Spec.IngressClassName == nil || !setContains(policy.AllowedIngressClasses, *ingress.Spec.IngressClassName) {
		errors = append(errors, field.Forbidden(field.NewPath("spec", "ingressClassName"), "ingress class is not allowed for this project"))
	}
	if ingress.Spec.DefaultBackend != nil {
		errors = append(errors, validateIngressBackend(ctx, policy, ingress.Spec.DefaultBackend, field.NewPath("spec", "defaultBackend"))...)
	}
	for index, rule := range ingress.Spec.Rules {
		path := field.NewPath("spec", "rules").Index(index)
		if !domainAllowed(rule.Host, policy.AllowedDomains) {
			errors = append(errors, field.Forbidden(path.Child("host"), "domain is not allowed for this project"))
		}
		if rule.HTTP == nil {
			continue
		}
		for pathIndex := range rule.HTTP.Paths {
			errors = append(errors, validateIngressBackend(ctx, policy, &rule.HTTP.Paths[pathIndex].Backend, path.Child("http", "paths").Index(pathIndex).Child("backend"))...)
		}
	}
	for index, tls := range ingress.Spec.TLS {
		for hostIndex, host := range tls.Hosts {
			if !domainAllowed(host, policy.AllowedDomains) {
				errors = append(errors, field.Forbidden(field.NewPath("spec", "tls").Index(index).Child("hosts").Index(hostIndex), "domain is not allowed for this project"))
			}
		}
		if tls.SecretName != "" {
			errors = append(errors, ValidateReference(ctx, policy, ObjectReference{GVR: schema.GroupVersionResource{Version: "v1", Resource: "secrets"}, Name: tls.SecretName}, field.NewPath("spec", "tls").Index(index).Child("secretName"))...)
		}
	}
	return errors
}

func validateIngressBackend(ctx context.Context, policy PolicyContext, backend *networkingv1.IngressBackend, path *field.Path) field.ErrorList {
	if backend == nil || backend.Service == nil {
		return field.ErrorList{field.Forbidden(path, "only Service backends are allowed")}
	}
	return ValidateReference(ctx, policy, ObjectReference{GVR: schema.GroupVersionResource{Version: "v1", Resource: "services"}, Name: backend.Service.Name}, path.Child("service", "name"))
}

func validateGatewayRoute(ctx context.Context, policy PolicyContext, object *unstructured.Unstructured) field.ErrorList {
	errors := field.ErrorList{}
	hostnames, _, _ := unstructured.NestedStringSlice(object.Object, "spec", "hostnames")
	for index, hostname := range hostnames {
		if !domainAllowed(hostname, policy.AllowedDomains) {
			errors = append(errors, field.Forbidden(field.NewPath("spec", "hostnames").Index(index), "domain is not allowed for this project"))
		}
	}
	parents, _, _ := unstructured.NestedSlice(object.Object, "spec", "parentRefs")
	if len(parents) == 0 {
		errors = append(errors, field.Required(field.NewPath("spec", "parentRefs"), "an allowed parent Gateway is required"))
	}
	for index, raw := range parents {
		parent, ok := raw.(map[string]any)
		if !ok {
			errors = append(errors, field.Invalid(field.NewPath("spec", "parentRefs").Index(index), raw, "parentRef must be an object"))
			continue
		}
		name, _, _ := unstructured.NestedString(parent, "name")
		namespace, _, _ := unstructured.NestedString(parent, "namespace")
		group, _, _ := unstructured.NestedString(parent, "group")
		kind, _, _ := unstructured.NestedString(parent, "kind")
		if namespace == "" {
			namespace = policy.Namespace
		}
		if group != "" && group != "gateway.networking.k8s.io" || kind != "" && kind != "Gateway" {
			errors = append(errors, field.Forbidden(field.NewPath("spec", "parentRefs").Index(index), "only configured Gateway parents are allowed"))
		}
		if !setContains(policy.AllowedGatewayParents, GatewayParentKey(namespace, name)) {
			errors = append(errors, field.Forbidden(field.NewPath("spec", "parentRefs").Index(index).Child("name"), "parent Gateway is not allowed for this project"))
		}
	}
	rules, _, _ := unstructured.NestedSlice(object.Object, "spec", "rules")
	for ruleIndex, rawRule := range rules {
		rule, ok := rawRule.(map[string]any)
		if !ok {
			continue
		}
		backends, _, _ := unstructured.NestedSlice(rule, "backendRefs")
		for backendIndex, rawBackend := range backends {
			path := field.NewPath("spec", "rules").Index(ruleIndex).Child("backendRefs").Index(backendIndex)
			errors = append(errors, validateRouteBackend(ctx, policy, rawBackend, path)...)
			if backend, ok := rawBackend.(map[string]any); ok {
				filters, _, _ := unstructured.NestedSlice(backend, "filters")
				errors = append(errors, validateRouteFilters(ctx, policy, filters, path.Child("filters"))...)
			}
		}
		filters, _, _ := unstructured.NestedSlice(rule, "filters")
		errors = append(errors, validateRouteFilters(ctx, policy, filters, field.NewPath("spec", "rules").Index(ruleIndex).Child("filters"))...)
	}
	return errors
}

// GatewayParentKey is the canonical identity used by the trusted gateway
// configuration and route validation. Names alone are not sufficient because
// a same-name Gateway can exist in multiple namespaces.
func GatewayParentKey(namespace, name string) string {
	return strings.TrimSpace(namespace) + "/" + strings.TrimSpace(name)
}

func validateRouteFilters(ctx context.Context, policy PolicyContext, filters []any, path *field.Path) field.ErrorList {
	errors := field.ErrorList{}
	for index, raw := range filters {
		filterPath := path.Index(index)
		filter, ok := raw.(map[string]any)
		if !ok {
			errors = append(errors, field.Invalid(filterPath, raw, "filter must be an object"))
			continue
		}
		filterType, _, _ := unstructured.NestedString(filter, "type")
		if filterType == "ExtensionRef" {
			errors = append(errors, field.Forbidden(filterPath.Child("extensionRef"), "extension filters are not allowed"))
		}
		if _, found, _ := unstructured.NestedMap(filter, "extensionRef"); found {
			errors = append(errors, field.Forbidden(filterPath.Child("extensionRef"), "extension filters are not allowed"))
		}
		mirror, found, _ := unstructured.NestedMap(filter, "requestMirror")
		if !found {
			continue
		}
		backend, backendFound, _ := unstructured.NestedMap(mirror, "backendRef")
		if !backendFound {
			errors = append(errors, field.Required(filterPath.Child("requestMirror", "backendRef"), "request mirror backend is required"))
			continue
		}
		errors = append(errors, validateRouteBackend(ctx, policy, backend, filterPath.Child("requestMirror", "backendRef"))...)
	}
	return errors
}

func validateRouteBackend(ctx context.Context, policy PolicyContext, raw any, path *field.Path) field.ErrorList {
	backend, ok := raw.(map[string]any)
	if !ok {
		return field.ErrorList{field.Invalid(path, raw, "backendRef must be an object")}
	}
	name, _, _ := unstructured.NestedString(backend, "name")
	namespace, _, _ := unstructured.NestedString(backend, "namespace")
	group, _, _ := unstructured.NestedString(backend, "group")
	kind, _, _ := unstructured.NestedString(backend, "kind")
	if group != "" || (kind != "" && kind != "Service") {
		return field.ErrorList{field.Forbidden(path, "only core Service backends are allowed")}
	}
	return ValidateReference(ctx, policy, ObjectReference{GVR: schema.GroupVersionResource{Version: "v1", Resource: "services"}, Namespace: namespace, Name: name}, path.Child("name"))
}

func domainAllowed(host string, allowed []string) bool {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if host == "" {
		return false
	}
	for _, raw := range allowed {
		candidate := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(raw), "."))
		if candidate == host {
			return true
		}
		if strings.HasPrefix(candidate, "*.") {
			suffix := strings.TrimPrefix(candidate, "*")
			if strings.HasSuffix(host, suffix) && host != strings.TrimPrefix(suffix, ".") {
				return true
			}
		}
	}
	return false
}

func setContains(values map[string]struct{}, value string) bool {
	if len(values) == 0 {
		return false
	}
	_, ok := values[strings.TrimSpace(value)]
	return ok
}

func gatewayError(path *field.Path, message string) field.ErrorList {
	return field.ErrorList{field.InternalError(path, fmt.Errorf("%s", message))}
}
