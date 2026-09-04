package gatewayapi

import (
	"regexp"
	"strings"
)

var gatewayHostSegmentPattern = regexp.MustCompile(`[^a-z0-9-]+`)

func gatewayHostSegment(value string) string {
	segment := strings.Trim(strings.ToLower(strings.TrimSpace(value)), "-")
	segment = gatewayHostSegmentPattern.ReplaceAllString(segment, "-")
	segment = strings.Join(strings.FieldsFunc(segment, func(char rune) bool { return char == '-' }), "-")
	return strings.Trim(segment, "-")
}

func dnsLabelName(value string) string {
	value = strings.Trim(strings.ToLower(strings.TrimSpace(value)), "-")
	if value == "" {
		return ""
	}
	value = gatewayHostSegmentPattern.ReplaceAllString(value, "-")
	value = strings.Join(strings.FieldsFunc(value, func(char rune) bool { return char == '-' }), "-")
	return strings.Trim(value, "-")
}
