package kubeproxy

import (
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

type RequestInfoResolver struct{}

func NewRequestInfoResolver() RequestInfoResolver { return RequestInfoResolver{} }

func (RequestInfoResolver) Resolve(request *http.Request, escapedKubePath string) (RequestInfo, error) {
	if request == nil {
		return RequestInfo{}, BadRequest(CodeBadRequest, fmt.Errorf("request is required"))
	}
	method := strings.ToUpper(strings.TrimSpace(request.Method))
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return RequestInfo{}, MethodNotAllowed()
	}
	path, err := validateEscapedPath(escapedKubePath)
	if err != nil {
		return RequestInfo{}, err
	}
	upgradeProtocol, isUpgrade, err := parseUpgrade(request.Header)
	if err != nil {
		return RequestInfo{}, err
	}
	query := request.URL.Query()
	watch, err := strictBoolQuery(query, "watch")
	if err != nil {
		return RequestInfo{}, err
	}
	follow, err := strictBoolQuery(query, "follow")
	if err != nil {
		return RequestInfo{}, err
	}
	if method == http.MethodHead && (watch || follow || isUpgrade) {
		return RequestInfo{}, BadRequest(CodeBadRequest, fmt.Errorf("HEAD does not support streaming"))
	}

	info := RequestInfo{
		Method: method, NonResourcePath: path, IsUpgrade: isUpgrade, UpgradeProtocol: upgradeProtocol,
		IsApplyPatch: isApplyPatchRequest(request, method), Transport: TransportNormal,
	}
	if nonResourceDiscovery(path) {
		if method != http.MethodGet && method != http.MethodHead {
			return RequestInfo{}, MethodNotAllowed()
		}
		info.Verb = "get"
		info.IsDiscovery = true
		return info, nil
	}
	segments := splitPath(path)
	resourceSegments := []string(nil)
	switch {
	case len(segments) >= 2 && segments[0] == "api":
		info.APIVersion = segments[1]
		if len(segments) == 2 {
			if method != http.MethodGet && method != http.MethodHead {
				return RequestInfo{}, MethodNotAllowed()
			}
			info.Verb, info.IsDiscovery = "get", true
			return info, nil
		}
		resourceSegments = segments[2:]
	case len(segments) >= 3 && segments[0] == "apis":
		info.APIGroup, info.APIVersion = segments[1], segments[2]
		if len(segments) == 3 {
			if method != http.MethodGet && method != http.MethodHead {
				return RequestInfo{}, MethodNotAllowed()
			}
			info.Verb, info.IsDiscovery = "get", true
			return info, nil
		}
		resourceSegments = segments[3:]
	default:
		return RequestInfo{}, NotFound(info.GVR(), "")
	}
	if info.APIVersion == "" || len(resourceSegments) == 0 {
		return RequestInfo{}, NotFound(info.GVR(), "")
	}
	if resourceSegments[0] == "namespaces" && len(resourceSegments) == 3 && (resourceSegments[2] == "status" || resourceSegments[2] == "finalize") {
		info.Resource, info.Name, info.Subresource = "namespaces", resourceSegments[1], strings.ToLower(resourceSegments[2])
		resourceSegments = nil
	} else if resourceSegments[0] == "namespaces" && len(resourceSegments) >= 3 {
		info.Namespace = resourceSegments[1]
		resourceSegments = resourceSegments[2:]
	}
	if resourceSegments != nil && (len(resourceSegments) < 1 || len(resourceSegments) > 3) {
		return RequestInfo{}, NotFound(info.GVR(), "")
	}
	if resourceSegments != nil {
		info.Resource = strings.ToLower(resourceSegments[0])
		if len(resourceSegments) >= 2 {
			info.Name = resourceSegments[1]
		}
		if len(resourceSegments) == 3 {
			info.Subresource = strings.ToLower(resourceSegments[2])
		}
	}
	info.IsResourceRequest = true
	info.IsCollection = info.Name == ""
	info.IsWatch = watch
	verb, err := requestVerb(method, info)
	if err != nil {
		return RequestInfo{}, err
	}
	info.Verb = verb
	if isUpgrade {
		info.Transport = TransportUpgrade
	} else if watch {
		info.Transport = TransportWatch
	} else if info.Subresource == "log" && follow {
		info.Transport = TransportLogs
	}
	return info, nil
}

func isApplyPatchRequest(request *http.Request, method string) bool {
	if request == nil || method != http.MethodPatch {
		return false
	}
	values := request.Header.Values("Content-Type")
	if len(values) != 1 {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(values[0])
	if err != nil {
		return false
	}
	switch strings.ToLower(mediaType) {
	case "application/apply-patch+yaml", "application/apply-patch+json":
		return true
	default:
		return false
	}
}

func validateEscapedPath(raw string) (string, error) {
	if raw == "" {
		raw = "/"
	}
	lower := strings.ToLower(raw)
	if !strings.HasPrefix(raw, "/") || strings.Contains(raw, "\\") || strings.Contains(raw, "//") ||
		strings.Contains(lower, "%2f") || strings.Contains(lower, "%5c") || strings.Contains(lower, "%00") ||
		strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return "", BadRequest(CodeBadRequest, fmt.Errorf("unsafe Kubernetes path"))
	}
	decoded, err := url.PathUnescape(raw)
	if err != nil || strings.ContainsRune(decoded, '\x00') || strings.Contains(decoded, "\\") {
		return "", BadRequest(CodeBadRequest, fmt.Errorf("malformed escaped Kubernetes path"))
	}
	decodedLower := strings.ToLower(decoded)
	if strings.Contains(decodedLower, "%2f") || strings.Contains(decodedLower, "%5c") || strings.Contains(decodedLower, "%00") || strings.Contains(decodedLower, "%2e") {
		return "", BadRequest(CodeBadRequest, fmt.Errorf("recursively escaped Kubernetes path is not allowed"))
	}
	for _, segment := range strings.Split(decoded, "/") {
		if segment == "." || segment == ".." {
			return "", BadRequest(CodeBadRequest, fmt.Errorf("path traversal is not allowed"))
		}
	}
	return decoded, nil
}

func splitPath(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

func nonResourceDiscovery(path string) bool {
	if path == "/version" || path == "/api" || path == "/apis" || path == "/openapi/v2" || path == "/openapi/v3" {
		return true
	}
	return strings.HasPrefix(path, "/openapi/v3/")
}

func strictBoolQuery(query url.Values, name string) (bool, error) {
	if len(query[name]) > 1 {
		return false, BadRequest(CodeBadRequest, fmt.Errorf("query parameter %s must not be repeated", name))
	}
	value := query.Get(name)
	if value == "" {
		return false, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, BadRequest(CodeBadRequest, fmt.Errorf("query parameter %s must be a boolean", name))
	}
	return parsed, nil
}

func parseUpgrade(header http.Header) (string, bool, error) {
	connectionUpgrade := headerTokenContains(header.Values("Connection"), "upgrade")
	protocol := strings.ToLower(strings.TrimSpace(header.Get("Upgrade")))
	if !connectionUpgrade && protocol == "" {
		return "", false, nil
	}
	if !connectionUpgrade || protocol == "" {
		return "", false, BadRequest(CodeBadRequest, fmt.Errorf("malformed Upgrade headers"))
	}
	switch protocol {
	case "websocket", "spdy/3.1":
		return protocol, true, nil
	default:
		return "", false, BadRequest(CodeBadRequest, fmt.Errorf("unsupported Upgrade protocol"))
	}
}

func headerTokenContains(values []string, target string) bool {
	for _, value := range values {
		for _, token := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(token), target) {
				return true
			}
		}
	}
	return false
}

func requestVerb(method string, info RequestInfo) (string, error) {
	if info.Subresource == "exec" || info.Subresource == "attach" || info.Subresource == "portforward" {
		if !info.IsUpgrade {
			return "", BadRequest(CodeBadRequest, fmt.Errorf("connect subresource requires Upgrade"))
		}
		switch info.UpgradeProtocol {
		case "websocket":
			if method != http.MethodGet {
				return "", BadRequest(CodeBadRequest, fmt.Errorf("WebSocket connect requires GET"))
			}
		case "spdy/3.1":
			if method != http.MethodPost {
				return "", BadRequest(CodeBadRequest, fmt.Errorf("SPDY connect requires POST"))
			}
		default:
			return "", BadRequest(CodeBadRequest, fmt.Errorf("unsupported connect protocol"))
		}
		return "connect", nil
	}
	if info.IsUpgrade {
		return "", BadRequest(CodeBadRequest, fmt.Errorf("Upgrade is only supported for connect subresources"))
	}
	switch method {
	case http.MethodGet, http.MethodHead:
		if info.IsWatch {
			if !info.IsCollection {
				return "", BadRequest(CodeBadRequest, fmt.Errorf("watch requires a collection path"))
			}
			return "watch", nil
		}
		if info.IsCollection {
			return "list", nil
		}
		return "get", nil
	case http.MethodPost:
		if !info.IsCollection && info.Subresource == "" {
			return "", MethodNotAllowed()
		}
		return "create", nil
	case http.MethodPut:
		if info.IsCollection {
			return "", MethodNotAllowed()
		}
		return "update", nil
	case http.MethodPatch:
		if info.IsCollection {
			return "", MethodNotAllowed()
		}
		return "patch", nil
	case http.MethodDelete:
		if info.IsCollection {
			return "deletecollection", nil
		}
		return "delete", nil
	default:
		return "", MethodNotAllowed()
	}
}

// RequestedPortForwardPorts parses the Kubernetes port-forward query without
// retaining the request URL. AccessPreflight implementations can compare this
// bounded result with container ports and Service target ports.
func RequestedPortForwardPorts(request *http.Request) ([]int32, error) {
	if request == nil {
		return nil, BadRequest(CodeBadRequest, fmt.Errorf("port-forward request is required"))
	}
	values := request.URL.Query()["ports"]
	if len(values) == 0 {
		return nil, BadRequest(CodeBadRequest, fmt.Errorf("port-forward ports are required"))
	}
	unique := map[int32]struct{}{}
	for _, value := range values {
		for _, raw := range strings.Split(value, ",") {
			port, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 32)
			if err != nil || port < 1 || port > 65535 {
				return nil, BadRequest(CodeBadRequest, fmt.Errorf("port-forward port is invalid"))
			}
			unique[int32(port)] = struct{}{}
			if len(unique) > 64 {
				return nil, BadRequest(CodeBadRequest, fmt.Errorf("too many port-forward ports"))
			}
		}
	}
	ports := make([]int32, 0, len(unique))
	for port := range unique {
		ports = append(ports, port)
	}
	sort.Slice(ports, func(i, j int) bool { return ports[i] < ports[j] })
	return ports, nil
}
