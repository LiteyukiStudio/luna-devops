package resourcename

import (
	"regexp"
	"strings"
)

var dnsLabelInvalidPattern = regexp.MustCompile(`[^a-z0-9-]+`)

func ProjectNamespace(projectID string) string {
	return FromID("ns", projectID)
}

func DeploymentTarget(targetID string) string {
	return FromID("dplt", targetID)
}

func GatewayRoute(routeID string) string {
	value := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(routeID)), "_", "-")
	return DNSLabel("luna-gateway-" + value)
}

func FromID(prefix, value string) string {
	suffix := ShortID(value)
	if suffix == "" {
		return DNSLabel(prefix)
	}
	return DNSLabel(prefix + "-" + suffix)
}

func ShortID(value string) string {
	value = strings.TrimSpace(value)
	if index := strings.Index(value, "_"); index >= 0 {
		value = value[index+1:]
	}
	value = dnsLabelSegment(value)
	if len(value) > 10 {
		return value[:10]
	}
	return value
}

func DNSLabel(value string) string {
	value = dnsLabelSegment(value)
	if value == "" {
		return "app"
	}
	if len(value) > 63 {
		value = strings.Trim(value[:63], "-")
	}
	if value == "" {
		return "app"
	}
	return value
}

func dnsLabelSegment(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = dnsLabelInvalidPattern.ReplaceAllString(value, "-")
	return strings.Trim(value, "-")
}
