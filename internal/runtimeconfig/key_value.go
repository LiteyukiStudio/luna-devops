package runtimeconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// DecodeKeyValue reads the canonical JSON object stored in runtime
// configuration columns.
func DecodeKeyValue(value string) (map[string]string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return map[string]string{}, nil
	}
	var raw map[string]string
	if err := json.Unmarshal([]byte(trimmed), &raw); err != nil || raw == nil {
		if err == nil {
			err = errors.New("expected JSON object")
		}
		return nil, fmt.Errorf("runtime configuration JSON object is invalid: %w", err)
	}
	return NormalizeKeyValue(raw)
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
