package api

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

var openAPIHTTPMethods = map[string]bool{
	"delete":  true,
	"get":     true,
	"head":    true,
	"options": true,
	"patch":   true,
	"post":    true,
	"put":     true,
	"trace":   true,
}

func TestOpenAPIOperationsHaveRegisteredRouterPaths(t *testing.T) {
	t.Parallel()

	repositoryRoot := apiRepositoryRoot(t)
	registered := registeredRouterOperations(t, filepath.Join(repositoryRoot, "internal", "api", "router.go"))
	document := readOpenAPIDocument(t, filepath.Join(repositoryRoot, "openapi", "openapi.yaml"))

	paths, ok := document["paths"].(map[string]any)
	if !ok {
		t.Fatal("OpenAPI document has no paths object")
	}

	var missing []string
	for path, rawPathItem := range paths {
		pathItem, ok := rawPathItem.(map[string]any)
		if !ok {
			continue
		}
		for method := range pathItem {
			if !openAPIHTTPMethods[strings.ToLower(method)] {
				continue
			}
			operation := strings.ToUpper(method) + " " + normalizeOpenAPIPath(path)
			if !registered[operation] {
				missing = append(missing, operation)
			}
		}
	}

	sort.Strings(missing)
	if len(missing) != 0 {
		t.Fatalf("OpenAPI operations without matching Gin routes:\n%s", strings.Join(missing, "\n"))
	}
}

func TestInitialAdministratorHasNoPublicHTTPInitializationRoutes(t *testing.T) {
	t.Parallel()

	repositoryRoot := apiRepositoryRoot(t)
	registered := registeredRouterOperations(t, filepath.Join(repositoryRoot, "internal", "api", "router.go"))
	document := readOpenAPIDocument(t, filepath.Join(repositoryRoot, "openapi", "openapi.yaml"))
	paths, ok := document["paths"].(map[string]any)
	if !ok {
		t.Fatal("OpenAPI document has no paths object")
	}

	for _, route := range []struct {
		method string
		path   string
	}{
		{method: "GET", path: "/api/v1/auth/bootstrap"},
		{method: "POST", path: "/api/v1/auth/bootstrap/admin"},
	} {
		if registered[route.method+" "+route.path] {
			t.Fatalf("removed initialization route is still registered: %s %s", route.method, route.path)
		}
		if _, exists := paths[route.path]; exists {
			t.Fatalf("removed initialization route is still documented: %s", route.path)
		}
	}
}

func apiRepositoryRoot(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test filename")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func registeredRouterOperations(t *testing.T, routerPath string) map[string]bool {
	t.Helper()

	file, err := parser.ParseFile(token.NewFileSet(), routerPath, nil, 0)
	if err != nil {
		t.Fatalf("parse router: %v", err)
	}

	operations := make(map[string]bool)
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || !isGinHTTPMethod(selector.Sel.Name) {
			return true
		}
		receiver, ok := selector.X.(*ast.Ident)
		if !ok || (receiver.Name != "router" && receiver.Name != "v1") {
			return true
		}
		pathLiteral, ok := call.Args[0].(*ast.BasicLit)
		if !ok || pathLiteral.Kind != token.STRING {
			return true
		}

		path := strings.Trim(pathLiteral.Value, `"`)
		if receiver.Name == "v1" {
			path = "/api/v1" + path
		}
		operations[selector.Sel.Name+" "+path] = true
		return true
	})

	return operations
}

func isGinHTTPMethod(method string) bool {
	switch method {
	case "DELETE", "GET", "HEAD", "OPTIONS", "PATCH", "POST", "PUT":
		return true
	default:
		return false
	}
}

func normalizeOpenAPIPath(path string) string {
	var normalized strings.Builder
	for index := 0; index < len(path); {
		if path[index] != '{' {
			normalized.WriteByte(path[index])
			index++
			continue
		}
		end := strings.IndexByte(path[index:], '}')
		if end < 0 {
			normalized.WriteString(path[index:])
			break
		}
		end += index
		normalized.WriteByte(':')
		normalized.WriteString(path[index+1 : end])
		index = end + 1
	}
	return normalized.String()
}

func readOpenAPIDocument(t *testing.T, path string) map[string]any {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read OpenAPI document: %v", err)
	}

	document := make(map[string]any)
	if err := yaml.Unmarshal(content, &document); err != nil {
		t.Fatalf("parse OpenAPI document: %v", err)
	}
	return document
}
