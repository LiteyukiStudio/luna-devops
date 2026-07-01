package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var (
	gatewayGVR      = schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "gateways"}
	gatewayClassGVR = schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "gatewayclasses"}
	httpRouteGVR    = schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "httproutes"}
)

type GatewaySpec struct {
	Name              string
	Namespace         string
	GatewayClassName  string
	ExternalTLSMode   string
	HTTPListenerName  string
	HTTPListenerPort  int32
	HTTPSListenerName string
	HTTPSListenerPort int32
	ProjectID         string
}

type HTTPRouteSpec struct {
	Name                   string
	Namespace              string
	ProjectID              string
	ApplicationID          string
	EnvironmentID          string
	DeploymentTargetID     string
	RouteID                string
	Host                   string
	Path                   string
	PathMatchType          string
	ParentGatewayName      string
	ParentGatewayNamespace string
	SectionName            string
	ServiceName            string
	ServicePort            int32
	BackendWeight          int32
	RequestHeaders         map[string]string
	ResponseHeaders        map[string]string
	URLRewrite             string
	RequestRedirect        string
}

type HTTPRouteStatusSnapshot struct {
	Summary    string
	Conditions []RouteConditionSnapshot
}

type RouteConditionSnapshot struct {
	Type               string
	Status             string
	Reason             string
	Message            string
	ObservedGeneration int64
}

func (c *Client) DetectGatewayAPISupport(ctx context.Context) error {
	if c.dynamic == nil {
		return fmt.Errorf("Gateway API dynamic client is not configured")
	}
	if _, err := c.dynamic.Resource(gatewayClassGVR).List(ctx, metav1.ListOptions{Limit: 1}); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("Gateway API CRDs are not installed: install gateway.networking.k8s.io/v1 GatewayClass, Gateway and HTTPRoute before enabling access routes")
		}
		return err
	}
	return nil
}

func (c *Client) EnsureGateway(ctx context.Context, spec GatewaySpec) error {
	if err := validateGatewayAPISpec(spec); err != nil {
		return err
	}
	if err := c.DetectGatewayAPISupport(ctx); err != nil {
		return err
	}
	gateway := gatewayObject(spec)
	return c.applyGatewayObject(ctx, gateway)
}

func (c *Client) ApplyHTTPRoute(ctx context.Context, spec HTTPRouteSpec) error {
	if err := validateHTTPRouteSpec(spec); err != nil {
		return err
	}
	if err := c.DetectGatewayAPISupport(ctx); err != nil {
		return err
	}
	route, err := httpRouteObject(spec)
	if err != nil {
		return err
	}
	return c.applyHTTPRouteObject(ctx, route)
}

func (c *Client) DeleteHTTPRoute(ctx context.Context, namespace, name string) error {
	if c.dynamic == nil {
		return fmt.Errorf("Gateway API dynamic client is not configured")
	}
	namespace = strings.TrimSpace(namespace)
	name = strings.TrimSpace(name)
	if namespace == "" || name == "" {
		return fmt.Errorf("HTTPRoute namespace and name are required")
	}
	err := c.dynamic.Resource(httpRouteGVR).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func (c *Client) GetHTTPRouteStatus(ctx context.Context, namespace, name string) (HTTPRouteStatusSnapshot, error) {
	if c.dynamic == nil {
		return HTTPRouteStatusSnapshot{}, fmt.Errorf("Gateway API dynamic client is not configured")
	}
	item, err := c.dynamic.Resource(httpRouteGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return HTTPRouteStatusSnapshot{}, err
	}
	conditions := routeConditionsFromUnstructured(item)
	return HTTPRouteStatusSnapshot{Summary: httpRouteSummary(conditions), Conditions: conditions}, nil
}

func gatewayObject(spec GatewaySpec) *gatewayv1.Gateway {
	from := gatewayv1.NamespacesFromAll
	httpListenerName := gatewayv1.SectionName(firstNonEmpty(spec.HTTPListenerName, "web"))
	httpListenerPort := spec.HTTPListenerPort
	if httpListenerPort <= 0 {
		httpListenerPort = 8080
	}
	httpsListenerName := gatewayv1.SectionName(firstNonEmpty(spec.HTTPSListenerName, "websecure"))
	httpsListenerPort := spec.HTTPSListenerPort
	if httpsListenerPort <= 0 {
		httpsListenerPort = 8443
	}
	listeners := []gatewayv1.Listener{{
		Name:     httpListenerName,
		Port:     gatewayv1.PortNumber(httpListenerPort),
		Protocol: gatewayv1.HTTPProtocolType,
		AllowedRoutes: &gatewayv1.AllowedRoutes{
			Namespaces: &gatewayv1.RouteNamespaces{From: &from},
		},
	}}
	if string(httpsListenerName) != string(httpListenerName) || httpsListenerPort != httpListenerPort {
		httpsProtocol := gatewayv1.HTTPProtocolType
		if strings.TrimSpace(spec.ExternalTLSMode) == "gateway" {
			httpsProtocol = gatewayv1.HTTPSProtocolType
		}
		listeners = append(listeners, gatewayv1.Listener{
			Name:     httpsListenerName,
			Port:     gatewayv1.PortNumber(httpsListenerPort),
			Protocol: httpsProtocol,
			AllowedRoutes: &gatewayv1.AllowedRoutes{
				Namespaces: &gatewayv1.RouteNamespaces{From: &from},
			},
		})
	}
	return &gatewayv1.Gateway{
		TypeMeta: metav1.TypeMeta{APIVersion: gatewayv1.GroupVersion.String(), Kind: "Gateway"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      spec.Name,
			Namespace: spec.Namespace,
			Labels:    gatewayLabelsForSpec(spec.ProjectID, "", "", "", "", ""),
		},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: gatewayv1.ObjectName(spec.GatewayClassName),
			Listeners:        listeners,
		},
	}
}

func httpRouteObject(spec HTTPRouteSpec) (*gatewayv1.HTTPRoute, error) {
	pathType := gatewayv1.PathMatchPathPrefix
	if spec.PathMatchType == "Exact" {
		pathType = gatewayv1.PathMatchExact
	}
	path := normalizedGatewayPath(spec.Path)
	parentNamespace := gatewayv1.Namespace(spec.ParentGatewayNamespace)
	parent := gatewayv1.ParentReference{
		Name:      gatewayv1.ObjectName(spec.ParentGatewayName),
		Namespace: &parentNamespace,
	}
	if sectionName := strings.TrimSpace(spec.SectionName); sectionName != "" {
		value := gatewayv1.SectionName(sectionName)
		parent.SectionName = &value
	}
	filters, err := httpRouteFilters(spec)
	if err != nil {
		return nil, err
	}
	rule := gatewayv1.HTTPRouteRule{
		Matches: []gatewayv1.HTTPRouteMatch{{
			Path: &gatewayv1.HTTPPathMatch{Type: &pathType, Value: &path},
		}},
		Filters: filters,
	}
	if strings.TrimSpace(spec.RequestRedirect) == "" {
		weight := int32(1)
		if spec.BackendWeight > 0 {
			weight = spec.BackendWeight
		}
		rule.BackendRefs = []gatewayv1.HTTPBackendRef{{
			BackendRef: gatewayv1.BackendRef{
				BackendObjectReference: gatewayv1.BackendObjectReference{
					Name: gatewayv1.ObjectName(spec.ServiceName),
					Port: ptr(gatewayv1.PortNumber(spec.ServicePort)),
				},
				Weight: &weight,
			},
		}}
	}
	return &gatewayv1.HTTPRoute{
		TypeMeta: metav1.TypeMeta{APIVersion: gatewayv1.GroupVersion.String(), Kind: "HTTPRoute"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      spec.Name,
			Namespace: spec.Namespace,
			Labels:    gatewayLabelsForSpec(spec.ProjectID, spec.ApplicationID, spec.EnvironmentID, spec.DeploymentTargetID, spec.RouteID, spec.ServiceName),
		},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{ParentRefs: []gatewayv1.ParentReference{parent}},
			Hostnames:       []gatewayv1.Hostname{gatewayv1.Hostname(spec.Host)},
			Rules:           []gatewayv1.HTTPRouteRule{rule},
		},
	}, nil
}

func normalizedGatewayPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "/"
	}
	if !strings.HasPrefix(value, "/") {
		return "/" + value
	}
	return value
}

func httpRouteFilters(spec HTTPRouteSpec) ([]gatewayv1.HTTPRouteFilter, error) {
	filters := []gatewayv1.HTTPRouteFilter{}
	if len(spec.RequestHeaders) > 0 {
		filters = append(filters, gatewayv1.HTTPRouteFilter{
			Type:                  gatewayv1.HTTPRouteFilterRequestHeaderModifier,
			RequestHeaderModifier: &gatewayv1.HTTPHeaderFilter{Set: httpHeaders(spec.RequestHeaders)},
		})
	}
	if len(spec.ResponseHeaders) > 0 {
		filters = append(filters, gatewayv1.HTTPRouteFilter{
			Type:                   gatewayv1.HTTPRouteFilterResponseHeaderModifier,
			ResponseHeaderModifier: &gatewayv1.HTTPHeaderFilter{Set: httpHeaders(spec.ResponseHeaders)},
		})
	}
	if strings.TrimSpace(spec.URLRewrite) != "" && strings.TrimSpace(spec.RequestRedirect) != "" {
		return nil, fmt.Errorf("URLRewrite and RequestRedirect cannot be used together")
	}
	if strings.TrimSpace(spec.URLRewrite) != "" {
		rewrite, err := parseURLRewriteFilter(spec.URLRewrite)
		if err != nil {
			return nil, err
		}
		filters = append(filters, gatewayv1.HTTPRouteFilter{Type: gatewayv1.HTTPRouteFilterURLRewrite, URLRewrite: rewrite})
	}
	if strings.TrimSpace(spec.RequestRedirect) != "" {
		redirect, err := parseRequestRedirectFilter(spec.RequestRedirect)
		if err != nil {
			return nil, err
		}
		filters = append(filters, gatewayv1.HTTPRouteFilter{Type: gatewayv1.HTTPRouteFilterRequestRedirect, RequestRedirect: redirect})
	}
	return filters, nil
}

func parseURLRewriteFilter(value string) (*gatewayv1.HTTPURLRewriteFilter, error) {
	raw := map[string]any{}
	if err := json.Unmarshal([]byte(value), &raw); err != nil {
		return nil, err
	}
	filter := &gatewayv1.HTTPURLRewriteFilter{}
	if hostname := stringValue(raw, "hostname", "host"); hostname != "" {
		value := gatewayv1.PreciseHostname(hostname)
		filter.Hostname = &value
	}
	if replacement := stringValue(raw, "replacePrefixMatch"); replacement != "" {
		filter.Path = &gatewayv1.HTTPPathModifier{Type: gatewayv1.PrefixMatchHTTPPathModifier, ReplacePrefixMatch: &replacement}
	}
	if replacement := stringValue(raw, "replaceFullPath"); replacement != "" {
		filter.Path = &gatewayv1.HTTPPathModifier{Type: gatewayv1.FullPathHTTPPathModifier, ReplaceFullPath: &replacement}
	}
	return filter, nil
}

func parseRequestRedirectFilter(value string) (*gatewayv1.HTTPRequestRedirectFilter, error) {
	raw := map[string]any{}
	if err := json.Unmarshal([]byte(value), &raw); err != nil {
		return nil, err
	}
	filter := &gatewayv1.HTTPRequestRedirectFilter{}
	if scheme := stringValue(raw, "scheme"); scheme != "" {
		filter.Scheme = &scheme
	}
	if hostname := stringValue(raw, "hostname", "host"); hostname != "" {
		value := gatewayv1.PreciseHostname(hostname)
		filter.Hostname = &value
	}
	if statusCode := intValue(raw, "statusCode"); statusCode > 0 {
		filter.StatusCode = &statusCode
	}
	if replacement := stringValue(raw, "replacePrefixMatch"); replacement != "" {
		filter.Path = &gatewayv1.HTTPPathModifier{Type: gatewayv1.PrefixMatchHTTPPathModifier, ReplacePrefixMatch: &replacement}
	}
	if replacement := stringValue(raw, "replaceFullPath"); replacement != "" {
		filter.Path = &gatewayv1.HTTPPathModifier{Type: gatewayv1.FullPathHTTPPathModifier, ReplaceFullPath: &replacement}
	}
	return filter, nil
}

func (c *Client) applyGatewayObject(ctx context.Context, item *gatewayv1.Gateway) error {
	unstructuredItem, err := toUnstructured(item)
	if err != nil {
		return err
	}
	resource := c.dynamic.Resource(gatewayGVR).Namespace(item.Namespace)
	existing, err := resource.Get(ctx, item.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = resource.Create(ctx, unstructuredItem, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	unstructuredItem.SetResourceVersion(existing.GetResourceVersion())
	_, err = resource.Update(ctx, unstructuredItem, metav1.UpdateOptions{})
	return err
}

func (c *Client) applyHTTPRouteObject(ctx context.Context, item *gatewayv1.HTTPRoute) error {
	unstructuredItem, err := toUnstructured(item)
	if err != nil {
		return err
	}
	resource := c.dynamic.Resource(httpRouteGVR).Namespace(item.Namespace)
	existing, err := resource.Get(ctx, item.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = resource.Create(ctx, unstructuredItem, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	unstructuredItem.SetResourceVersion(existing.GetResourceVersion())
	_, err = resource.Update(ctx, unstructuredItem, metav1.UpdateOptions{})
	return err
}

func toUnstructured(item any) (*unstructured.Unstructured, error) {
	raw, err := runtime.DefaultUnstructuredConverter.ToUnstructured(item)
	if err != nil {
		return nil, err
	}
	return &unstructured.Unstructured{Object: raw}, nil
}

func gatewayLabelsForSpec(projectID, applicationID, environmentID, deploymentTargetID, routeID, serviceName string) map[string]string {
	labels := baseManagedLabels(serviceName)
	setLabel(labels, ProjectIDLabel, projectID)
	setLabel(labels, ApplicationIDLabel, applicationID)
	setLabel(labels, EnvironmentIDLabel, environmentID)
	setLabel(labels, DeploymentTargetIDLabel, deploymentTargetID)
	setLabel(labels, GatewayRouteIDLabel, routeID)
	return labels
}

func httpHeaders(values map[string]string) []gatewayv1.HTTPHeader {
	headers := make([]gatewayv1.HTTPHeader, 0, len(values))
	for key, value := range values {
		headers = append(headers, gatewayv1.HTTPHeader{Name: gatewayv1.HTTPHeaderName(key), Value: value})
	}
	return headers
}

func routeConditionsFromUnstructured(item *unstructured.Unstructured) []RouteConditionSnapshot {
	parents, _, _ := unstructured.NestedSlice(item.Object, "status", "parents")
	conditions := []RouteConditionSnapshot{}
	for _, rawParent := range parents {
		parent, ok := rawParent.(map[string]any)
		if !ok {
			continue
		}
		rawConditions, _, _ := unstructured.NestedSlice(parent, "conditions")
		for _, rawCondition := range rawConditions {
			condition, ok := rawCondition.(map[string]any)
			if !ok {
				continue
			}
			conditions = append(conditions, RouteConditionSnapshot{
				Type:               fmt.Sprint(condition["type"]),
				Status:             fmt.Sprint(condition["status"]),
				Reason:             fmt.Sprint(condition["reason"]),
				Message:            fmt.Sprint(condition["message"]),
				ObservedGeneration: int64Value(condition["observedGeneration"]),
			})
		}
	}
	return conditions
}

func httpRouteSummary(conditions []RouteConditionSnapshot) string {
	summary := "pending"
	for _, condition := range conditions {
		if condition.Type == "Accepted" && condition.Status == "True" {
			summary = "accepted"
		}
		if (condition.Type == "ResolvedRefs" || condition.Type == "Programmed" || condition.Type == "Accepted") && condition.Status == "False" {
			summary = "failed"
		}
	}
	return summary
}

func validateGatewayAPISpec(spec GatewaySpec) error {
	if strings.TrimSpace(spec.Name) == "" || strings.TrimSpace(spec.Namespace) == "" {
		return fmt.Errorf("Gateway name and namespace are required")
	}
	if strings.TrimSpace(spec.GatewayClassName) == "" {
		return fmt.Errorf("GatewayClass name is required")
	}
	if spec.HTTPListenerPort < 0 || spec.HTTPListenerPort > 65535 || spec.HTTPSListenerPort < 0 || spec.HTTPSListenerPort > 65535 {
		return fmt.Errorf("Gateway listener ports must be between 1 and 65535")
	}
	return nil
}

func validateHTTPRouteSpec(spec HTTPRouteSpec) error {
	if strings.TrimSpace(spec.Name) == "" || strings.TrimSpace(spec.Namespace) == "" {
		return fmt.Errorf("HTTPRoute name and namespace are required")
	}
	if strings.TrimSpace(spec.ParentGatewayName) == "" || strings.TrimSpace(spec.ParentGatewayNamespace) == "" {
		return fmt.Errorf("HTTPRoute parent Gateway is required")
	}
	if strings.TrimSpace(spec.Host) == "" {
		return fmt.Errorf("HTTPRoute host is required")
	}
	if strings.TrimSpace(spec.ServiceName) == "" && strings.TrimSpace(spec.RequestRedirect) == "" {
		return fmt.Errorf("HTTPRoute backend service is required")
	}
	if strings.TrimSpace(spec.RequestRedirect) == "" && (spec.ServicePort <= 0 || spec.ServicePort > 65535) {
		return fmt.Errorf("service port must be between 1 and 65535")
	}
	return nil
}

func stringValue(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			return strings.TrimSpace(fmt.Sprint(value))
		}
	}
	return ""
}

func intValue(values map[string]any, key string) int {
	switch value := values[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

func int64Value(value any) int64 {
	switch item := value.(type) {
	case int64:
		return item
	case int:
		return int64(item)
	case float64:
		return int64(item)
	default:
		return 0
	}
}

func ptr[T any](value T) *T {
	return &value
}
