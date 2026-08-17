package runtimeconfig

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ParseLegacyKeyValue reads the historical text representation used by runtime
// configuration columns. It accepts both KEY=value lines and a JSON object
// encoded as text so existing rows can be read during the contract migration.
func ParseLegacyKeyValue(value string) (map[string]string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return map[string]string{}, nil
	}
	if strings.HasPrefix(trimmed, "{") {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
			return nil, fmt.Errorf("runtime configuration JSON object is invalid: %w", err)
		}
		output := make(map[string]string, len(raw))
		for key, encoded := range raw {
			var item any
			if err := json.Unmarshal(encoded, &item); err != nil {
				return nil, fmt.Errorf("runtime configuration value for %q is invalid: %w", key, err)
			}
			var normalizedItem string
			switch typed := item.(type) {
			case string:
				normalizedItem = typed
			case float64, bool:
				normalizedItem = fmt.Sprint(typed)
			case nil:
				normalizedItem = ""
			default:
				return nil, fmt.Errorf("runtime configuration value for %q must be a scalar", key)
			}
			if err := addValue(output, key, normalizedItem); err != nil {
				return nil, err
			}
		}
		return output, nil
	}

	output := map[string]string{}
	for _, rawLine := range strings.Split(value, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, item, ok := strings.Cut(rawLine, "=")
		if !ok {
			return nil, fmt.Errorf("runtime configuration line %q must contain '='", line)
		}
		if err := addValue(output, key, strings.TrimRight(item, " \t\r")); err != nil {
			return nil, err
		}
	}
	return output, nil
}

// NormalizeKeyValue validates and normalizes an API object before it is
// persisted or passed to a runtime provider.
func NormalizeKeyValue(values map[string]string) (map[string]string, error) {
	output := make(map[string]string, len(values))
	for key, value := range values {
		if err := addValue(output, key, value); err != nil {
			return nil, err
		}
	}
	return output, nil
}

// EncodeKeyValue stores a stable JSON representation for text database columns.
// encoding/json sorts map keys, making the result deterministic for audits and
// tests.
func EncodeKeyValue(values map[string]string) (string, error) {
	normalized, err := NormalizeKeyValue(values)
	if err != nil {
		return "", err
	}
	if len(normalized) == 0 {
		return "", nil
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("encode runtime configuration: %w", err)
	}
	return string(encoded), nil
}

func SortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func addValue(output map[string]string, key, value string) error {
	normalizedKey := strings.TrimSpace(key)
	if normalizedKey == "" {
		return fmt.Errorf("runtime configuration key cannot be empty")
	}
	if _, exists := output[normalizedKey]; exists {
		return fmt.Errorf("runtime configuration key %q is duplicated", normalizedKey)
	}
	output[normalizedKey] = value
	return nil
}
