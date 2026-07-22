package buildenv

import (
	"encoding/json"
	"strings"
)

type Snapshot struct {
	Variables  map[string]string
	SecretRefs map[string]string
}

func NewSnapshot() Snapshot {
	return Snapshot{Variables: map[string]string{}, SecretRefs: map[string]string{}}
}

func IsKey(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for index, char := range value {
		if index == 0 {
			if char == '_' || char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' {
				continue
			}
			return false
		}
		if char == '_' || char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' || char >= '0' && char <= '9' {
			continue
		}
		return false
	}
	return true
}

func Decode(raw string) map[string]string {
	values := map[string]string{}
	if err := json.Unmarshal([]byte(fallback(raw, "{}")), &values); err != nil {
		return map[string]string{}
	}
	return values
}

func Encode(values map[string]string) string {
	content, err := json.Marshal(values)
	if err != nil {
		return "{}"
	}
	return string(content)
}

// Apply overlays one scope. A key is either public or secret at the winning scope.
func Apply(snapshot *Snapshot, variablesRaw, secretRefsRaw string) {
	if snapshot.Variables == nil {
		snapshot.Variables = map[string]string{}
	}
	if snapshot.SecretRefs == nil {
		snapshot.SecretRefs = map[string]string{}
	}
	for key, value := range Decode(variablesRaw) {
		if !IsKey(key) {
			continue
		}
		snapshot.Variables[key] = value
		delete(snapshot.SecretRefs, key)
	}
	for key, ref := range Decode(secretRefsRaw) {
		if !IsKey(key) || strings.TrimSpace(ref) == "" {
			continue
		}
		snapshot.SecretRefs[key] = ref
		delete(snapshot.Variables, key)
	}
}

func Resolve(snapshot Snapshot, resolveSecret func(string) string) (map[string]string, []string) {
	output := make(map[string]string, len(snapshot.Variables)+len(snapshot.SecretRefs))
	for key, value := range snapshot.Variables {
		if IsKey(key) {
			output[key] = value
		}
	}
	sensitive := make([]string, 0, len(snapshot.SecretRefs))
	for key, ref := range snapshot.SecretRefs {
		if !IsKey(key) {
			continue
		}
		if value := resolveSecret(ref); value != "" {
			output[key] = value
			sensitive = append(sensitive, value)
		}
	}
	return output, sensitive
}

func fallback(value, fallbackValue string) string {
	if strings.TrimSpace(value) == "" {
		return fallbackValue
	}
	return value
}
