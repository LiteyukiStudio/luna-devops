package runtimecluster

import "strings"

const DefaultGatewayDomainSuffix = "apps.local"

func NormalizeGatewayDomainSuffix(value string) string {
	return strings.Trim(strings.ToLower(strings.TrimSpace(value)), ".")
}

func NormalizeGatewayDomainSuffixes(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		suffix := NormalizeGatewayDomainSuffix(value)
		if suffix == "" {
			continue
		}
		if _, exists := seen[suffix]; exists {
			continue
		}
		seen[suffix] = struct{}{}
		result = append(result, suffix)
	}
	if len(result) == 0 {
		return []string{DefaultGatewayDomainSuffix}
	}
	return result
}

func EncodeGatewayDomainSuffixes(values []string) string {
	return strings.Join(NormalizeGatewayDomainSuffixes(values), "\n")
}

func DecodeGatewayDomainSuffixes(raw string) []string {
	return NormalizeGatewayDomainSuffixes(strings.FieldsFunc(raw, func(char rune) bool {
		return char == '\n' || char == ',' || char == ';'
	}))
}
