package runtimeconfig

import (
	"regexp"
	"sort"
	"strings"
)

var (
	secretURLCredentials = regexp.MustCompile(`(?i)[a-z][a-z0-9+.-]*://[^\s/@:]+:[^\s/@]+@`)
	secretAssignment     = regexp.MustCompile(`(?i)(?:^|[?&;\s])(?:password|passwd|pass|token|api[_-]?key|secret|auth|credential)\s*=`)
	secretKeySeparator   = regexp.MustCompile(`[^A-Z0-9]+`)
)

// PotentialSecret reports whether a legacy public runtime setting has common
// secret semantics for the opt-in diagnostic command. It must not be used to
// reject writes: callers choose the authoritative public or secret valueMode.
func PotentialSecret(key, value string) bool {
	return PotentialSecretKey(key) || PotentialSecretValue(value)
}

func PotentialSecretKey(key string) bool {
	upper := strings.ToUpper(strings.TrimSpace(key))
	compact := secretKeySeparator.ReplaceAllString(upper, "")
	for _, marker := range []string{
		"SECRET", "PASSWORD", "PASSWD", "TOKEN", "APIKEY", "PRIVATEKEY", "CLIENTSECRET",
		"ACCESSKEY", "REFRESHTOKEN", "KUBECONFIG", "CREDENTIAL", "AUTH", "DSN",
	} {
		if strings.Contains(compact, marker) {
			return true
		}
	}
	for _, part := range secretKeySeparator.Split(upper, -1) {
		if part == "PASS" {
			return true
		}
	}
	return false
}

func PotentialSecretValue(value string) bool {
	trimmed := strings.TrimSpace(value)
	return secretURLCredentials.MatchString(trimmed) || secretAssignment.MatchString(trimmed)
}

// SuspectedSecretKeys inspects a legacy public column and returns key names
// only. Values are intentionally excluded from the diagnostic result.
func SuspectedSecretKeys(raw string) ([]string, error) {
	values, err := ParseLegacyKeyValue(raw)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0)
	for key, value := range values {
		if PotentialSecret(key, value) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys, nil
}
