package authz

import (
	"errors"
	"strings"
)

const (
	KubeScopeRead    = "kube:read"
	KubeScopeWrite   = "kube:write"
	KubeScopeConnect = "kube:connect"
)

var ErrKubeScopeInvalid = errors.New("kubernetes credential scope is invalid")

var kubeScopeOrder = []string{KubeScopeRead, KubeScopeWrite, KubeScopeConnect}

// NormalizeKubeScopes validates the transport scopes used by a kubeconfig
// credential. Write and connect credentials always include read so kubectl can
// complete discovery and resolve the target object before a mutating request.
func NormalizeKubeScopes(values []string) ([]string, error) {
	selected := make(map[string]bool, len(values)+1)
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		switch value {
		case KubeScopeRead, KubeScopeWrite, KubeScopeConnect:
			selected[value] = true
		default:
			return nil, ErrKubeScopeInvalid
		}
	}
	if len(selected) == 0 {
		return nil, ErrKubeScopeInvalid
	}
	if selected[KubeScopeWrite] || selected[KubeScopeConnect] {
		selected[KubeScopeRead] = true
	}
	result := make([]string, 0, len(selected))
	for _, scope := range kubeScopeOrder {
		if selected[scope] {
			result = append(result, scope)
		}
	}
	return result, nil
}

func NormalizeKubeScopeText(scopeText string) (string, error) {
	values := strings.FieldsFunc(scopeText, func(r rune) bool { return r == ',' || r == ' ' })
	normalized, err := NormalizeKubeScopes(values)
	if err != nil {
		return "", err
	}
	return strings.Join(normalized, ","), nil
}

func KubeScopeAllows(scopeText, required string) bool {
	required = strings.ToLower(strings.TrimSpace(required))
	if required == "" {
		return true
	}
	normalized, err := NormalizeKubeScopeText(scopeText)
	if err != nil {
		return false
	}
	for _, scope := range strings.Split(normalized, ",") {
		if scope == required {
			return true
		}
	}
	return false
}

// RequiredKubeScope maps the Kubernetes transport verb to the independent
// kube credential scope. Business authorization is deliberately evaluated by
// ProjectAuthorizer after this check.
func RequiredKubeScope(verb string) string {
	switch strings.ToLower(strings.TrimSpace(verb)) {
	case "create", "update", "patch", "delete", "deletecollection":
		return KubeScopeWrite
	case "connect":
		return KubeScopeConnect
	default:
		return KubeScopeRead
	}
}
