package gatewayapi

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	gatewayHeaderNamePattern = regexp.MustCompile(`^[!#$%&'*+\-.^_` + "`" + `|~0-9A-Za-z]+$`)
	gatewayHopByHopHeaders   = map[string]struct{}{
		"connection":          {},
		"keep-alive":          {},
		"proxy-authenticate":  {},
		"proxy-authorization": {},
		"te":                  {},
		"trailer":             {},
		"transfer-encoding":   {},
		"upgrade":             {},
	}
	gatewayPrivilegedHeaders = map[string]struct{}{
		"authorization":     {},
		"cookie":            {},
		"host":              {},
		"x-forwarded-for":   {},
		"x-forwarded-host":  {},
		"x-forwarded-port":  {},
		"x-forwarded-proto": {},
		"x-real-ip":         {},
		"set-cookie":        {},
	}
)

func parseGatewayHeaderMap(value string, allowPrivileged bool) (map[string]string, error) {
	items, err := parseGatewayKeyValueMap(value)
	if err != nil {
		return nil, err
	}
	for key := range items {
		normalized := strings.ToLower(strings.TrimSpace(key))
		if !gatewayHeaderNamePattern.MatchString(key) {
			return nil, fmt.Errorf("header %q 不是合法 HTTP header 名称", key)
		}
		if _, exists := gatewayHopByHopHeaders[normalized]; exists {
			return nil, fmt.Errorf("不允许配置逐跳 header %q", key)
		}
		if _, exists := gatewayPrivilegedHeaders[normalized]; exists && !allowPrivileged {
			return nil, fmt.Errorf("header %q 仅平台管理员可配置", key)
		}
	}
	return items, nil
}

func parseGatewayKeyValueMap(value string) (map[string]string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return map[string]string{}, nil
	}
	if strings.HasPrefix(value, "{") {
		var raw map[string]any
		if err := json.Unmarshal([]byte(value), &raw); err != nil {
			return nil, err
		}
		parsed := make(map[string]string, len(raw))
		for key, item := range raw {
			parsed[strings.TrimSpace(key)] = fmt.Sprint(item)
		}
		return compactGatewayKeyValueMap(parsed), nil
	}
	parsed := map[string]string{}
	for lineNumber, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, item, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("第 %s 行缺少 =", strconv.Itoa(lineNumber+1))
		}
		parsed[strings.TrimSpace(key)] = strings.TrimSpace(item)
	}
	return compactGatewayKeyValueMap(parsed), nil
}

func compactGatewayKeyValueMap(values map[string]string) map[string]string {
	compacted := map[string]string{}
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		compacted[key] = strings.TrimSpace(value)
	}
	return compacted
}

func encodeGatewayHeaderMap(values map[string]string) string {
	if len(values) == 0 {
		return ""
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func looksLikeSecretValue(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return strings.Contains(lower, "secret=") ||
		strings.Contains(lower, "token=") ||
		strings.Contains(lower, "password=") ||
		strings.Contains(lower, "authorization:")
}

func normalizeHTTPRoutePathMatchType(value string) string {
	if strings.TrimSpace(value) == "Exact" {
		return "Exact"
	}
	return "PathPrefix"
}

func normalizeBackendWeight(value int) int {
	if value <= 0 {
		return 1
	}
	return value
}

func validateGatewayRouteFilterJSON(label string, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(value), &raw); err != nil {
		return fmt.Errorf("%s 配置必须是 JSON 对象", label)
	}
	for key, item := range raw {
		if looksLikeSecretValue(key) || looksLikeSecretValue(fmt.Sprint(item)) {
			return fmt.Errorf("%s 配置不应直接包含密钥值", label)
		}
	}
	return nil
}
