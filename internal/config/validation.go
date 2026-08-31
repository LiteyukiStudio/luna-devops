package config

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"path"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	minimumVolumeTransferBytes = int64(1 * 1024 * 1024 * 1024)
	maximumVolumeTransferBytes = int64(5 * 1024 * 1024 * 1024 * 1024)
)

func validatePublicBaseURL(value string) error {
	if value == "" {
		return nil
	}
	return validateHTTPURL("PUBLIC_BASE_URL", value)
}

func validateProductionPublicBaseURL(mode, value string) error {
	if mode != "production" || strings.TrimSpace(value) == "" {
		return nil
	}
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" {
		return nil
	}
	if parsed.Scheme == "https" {
		return nil
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "localhost" {
		return nil
	}
	if address, parseErr := netip.ParseAddr(host); parseErr == nil && address.IsLoopback() {
		return nil
	}
	return errors.New("PUBLIC_BASE_URL must use https in production except for localhost or loopback addresses")
}

func validateDatabaseURL(value string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		return errors.New("DATABASE_URL must be an absolute postgres or postgresql URL")
	}
	return nil
}

func validateListenAddress(key, value string, required bool) error {
	value = strings.TrimSpace(value)
	if value == "" {
		if required {
			return fmt.Errorf("%s is required", key)
		}
		return nil
	}
	_, portValue, err := net.SplitHostPort(value)
	if err != nil {
		return fmt.Errorf("%s must use IP:port or :port format", key)
	}
	portNumber, err := strconv.Atoi(portValue)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return fmt.Errorf("%s port must be between 1 and 65535", key)
	}
	return nil
}

func validateHTTPURL(key string, value string) error {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("%s must be an absolute http or https URL", key)
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("%s must not contain credentials, query parameters, or fragments", key)
	}
	return nil
}

func validateEnum(key string, value string, allowed ...string) error {
	for _, candidate := range allowed {
		if value == candidate {
			return nil
		}
	}
	return fmt.Errorf("%s must be one of %s", key, strings.Join(allowed, ", "))
}

func runtimeModeFromValue(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "production", "prod", "":
		return "production", nil
	case "development", "dev", "local":
		return "development", nil
	default:
		return "production", errors.New("APP_ENV must be production or development")
	}
}

func splitList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			values = append(values, value)
		}
	}
	return values
}

func parsePortList(key, raw string, fallback []int) ([]int, error) {
	if strings.TrimSpace(raw) == "" {
		return append([]int(nil), fallback...), nil
	}
	parts := strings.Split(raw, ",")
	values := make([]int, 0, len(parts))
	seen := map[int]bool{}
	for _, part := range parts {
		value, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || value < 1 || value > 65535 {
			return nil, fmt.Errorf("%s must contain ports between 1 and 65535", key)
		}
		if !seen[value] {
			seen[value] = true
			values = append(values, value)
		}
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("%s must contain at least one port", key)
	}
	return values, nil
}

func parseByteQuantity(key, raw string, fallback int64) (int64, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return fallback, nil
	}
	quantity, err := resource.ParseQuantity(value)
	if err != nil || quantity.Sign() <= 0 {
		return 0, fmt.Errorf("%s must be a positive byte quantity", key)
	}
	return quantity.Value(), nil
}

func parseCIDRList(key string, values []string) ([]string, error) {
	result := make([]string, 0, len(values))
	seen := make(map[netip.Prefix]struct{}, len(values))
	for _, value := range values {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("%s must contain valid CIDRs", key)
		}
		prefix = prefix.Masked()
		if _, exists := seen[prefix]; exists {
			continue
		}
		seen[prefix] = struct{}{}
		result = append(result, prefix.String())
	}
	return result, nil
}

func parseTrustedProxyCIDRList(values []string) ([]string, error) {
	cidrs, err := parseCIDRList("TRUSTED_PROXY_CIDRS", values)
	if err != nil {
		return nil, err
	}
	for _, value := range cidrs {
		prefix, _ := netip.ParsePrefix(value)
		if prefix.Bits() == 0 {
			return nil, errors.New("TRUSTED_PROXY_CIDRS must not contain universal CIDRs")
		}
	}
	return cidrs, nil
}

func parseOrigins(key string, values []string) ([]string, error) {
	result := make([]string, 0, len(values))
	for _, value := range values {
		parsed, err := url.Parse(strings.TrimSpace(value))
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
			return nil, fmt.Errorf("%s must contain absolute http or https origins without paths", key)
		}
		result = append(result, strings.TrimRight(parsed.Scheme+"://"+parsed.Host, "/"))
	}
	return normalizeList(result), nil
}

func parseKeyValueList(key, raw string) (map[string]string, error) {
	values := make(map[string]string)
	if strings.TrimSpace(raw) == "" {
		return values, nil
	}
	for _, entry := range strings.Split(raw, ",") {
		name, encodedValue, ok := strings.Cut(entry, "=")
		name = strings.TrimSpace(name)
		encodedValue = strings.TrimSpace(encodedValue)
		if !ok || name == "" || strings.ContainsAny(name+encodedValue, "\r\n") {
			return nil, fmt.Errorf("%s must contain comma-separated key=value pairs", key)
		}
		value, err := url.QueryUnescape(encodedValue)
		if err != nil || strings.ContainsAny(value, "\r\n") {
			return nil, fmt.Errorf("%s contains an invalid encoded value", key)
		}
		values[name] = value
	}
	return values, nil
}

func signalEndpoint(raw, signalPath string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", nil
	}
	if err := validateHTTPURL("OTLP endpoint", value); err != nil {
		return "", err
	}
	parsed, _ := url.Parse(value)
	parsed.Path = path.Join(parsed.Path, signalPath)
	return parsed.String(), nil
}

func allowedOrigins(mode, publicBaseURL string, configured []string) []string {
	origins := append([]string(nil), configured...)
	if publicBase := originFromURL(publicBaseURL); publicBase != "" {
		origins = append(origins, publicBase)
	}
	if mode == "development" {
		origins = append(origins,
			"http://localhost:5173", "http://127.0.0.1:5173",
			"http://localhost:4173", "http://127.0.0.1:4173",
			"http://localhost:4174", "http://127.0.0.1:4174",
			"http://localhost:4184", "http://127.0.0.1:4184",
		)
	}
	return normalizeList(origins)
}

func originFromURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return ""
	}
	return strings.TrimRight(parsed.Scheme+"://"+parsed.Host, "/")
}

func normalizeList(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimRight(strings.TrimSpace(value), "/")
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func cloneMap(input map[string]string) map[string]string {
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func appendError(errs []error, err error) []error {
	if err != nil {
		return append(errs, err)
	}
	return errs
}
