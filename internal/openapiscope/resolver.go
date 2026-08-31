package openapiscope

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/LiteyukiStudio/devops/openapi"
	"sigs.k8s.io/yaml"
)

// ErrRequiredScopesNotDeclared marks an operation that cannot safely authorize
// bearer credentials because its OpenAPI operation has no requiredScopes.
var ErrRequiredScopesNotDeclared = errors.New("required scopes are not declared for OpenAPI operation")

type routeKey struct {
	method string
	path   string
}

// Resolver is an immutable view of x-luna-cli.requiredScopes keyed by route.
type Resolver struct {
	scopes map[routeKey][]string
}

type document struct {
	Paths map[string]map[string]json.RawMessage `json:"paths"`
}

type operation struct {
	OperationID string `json:"operationId"`
	CLI         struct {
		RequiredScopes []string `json:"requiredScopes"`
	} `json:"x-luna-cli"`
}

var (
	embeddedOnce     sync.Once
	embeddedResolver *Resolver
	embeddedErr      error
)

// New builds an immutable route-to-scope resolver from an OpenAPI document.
func New(source []byte) (*Resolver, error) {
	jsonSource, err := yaml.YAMLToJSON(source)
	if err != nil {
		return nil, fmt.Errorf("convert OpenAPI scope contract: %w", err)
	}
	var parsed document
	if err := json.Unmarshal(jsonSource, &parsed); err != nil {
		return nil, fmt.Errorf("parse OpenAPI scope contract: %w", err)
	}

	resolver := &Resolver{scopes: make(map[routeKey][]string)}
	for path, pathItem := range parsed.Paths {
		for method, raw := range pathItem {
			if !isHTTPMethod(method) {
				continue
			}
			var operation operation
			if err := json.Unmarshal(raw, &operation); err != nil {
				return nil, fmt.Errorf("parse OpenAPI operation scope contract for %s %s: %w", strings.ToUpper(method), path, err)
			}
			if strings.TrimSpace(operation.OperationID) == "" {
				continue
			}
			key := routeKey{method: strings.ToUpper(method), path: canonicalPath(path)}
			if _, exists := resolver.scopes[key]; exists {
				return nil, fmt.Errorf("duplicate OpenAPI route scope contract for %s %s", key.method, key.path)
			}
			resolver.scopes[key] = normalizedScopes(operation.CLI.RequiredScopes)
		}
	}
	return resolver, nil
}

// RequiredScopes resolves the embedded OpenAPI contract for an OpenAPI or Gin
// route template and returns a defensive copy of its required scopes.
func RequiredScopes(path, method string) ([]string, error) {
	embeddedOnce.Do(func() {
		embeddedResolver, embeddedErr = New(openapi.SpecYAML)
	})
	if embeddedErr != nil {
		return nil, embeddedErr
	}
	return embeddedResolver.RequiredScopes(path, method)
}

// RequiredScopes resolves an OpenAPI or Gin route template and returns a
// defensive copy of its required scopes.
func (resolver *Resolver) RequiredScopes(path, method string) ([]string, error) {
	if resolver == nil {
		return nil, ErrRequiredScopesNotDeclared
	}
	key := routeKey{method: strings.ToUpper(strings.TrimSpace(method)), path: canonicalPath(path)}
	scopes, exists := resolver.scopes[key]
	if !exists || len(scopes) == 0 {
		return nil, fmt.Errorf("%w for %s %s", ErrRequiredScopesNotDeclared, key.method, key.path)
	}
	return append([]string(nil), scopes...), nil
}

func canonicalPath(path string) string {
	path = strings.TrimSpace(strings.SplitN(path, "?", 2)[0])
	parts := strings.Split(path, "/")
	for index, part := range parts {
		switch {
		case strings.HasPrefix(part, ":"), strings.HasPrefix(part, "*"):
			parts[index] = "{}"
		case strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}"):
			parts[index] = "{}"
		}
	}
	return strings.Join(parts, "/")
}

func normalizedScopes(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func isHTTPMethod(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
		return true
	default:
		return false
	}
}
