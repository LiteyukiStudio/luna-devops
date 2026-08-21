package api

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/aitool"
)

// TestAgentEligibleHandlersDeclareConsumedQueryParameters keeps the HTTP
// handler and the model-visible OpenAPI schema on the same contract. Without
// this guard, valid filters emitted by the model are rejected before the
// request reaches the handler.
func TestAgentEligibleHandlersDeclareConsumedQueryParameters(t *testing.T) {
	operations, err := aitool.PlatformCatalog()
	if err != nil {
		t.Fatal(err)
	}
	operationByHandler := make(map[string]aitool.OpenAPIOperation, len(operations))
	for _, operation := range operations {
		operationByHandler[upperFirst(operation.OperationID)] = operation
	}

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	files := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		parsed, parseErr := parser.ParseFile(files, entry.Name(), nil, 0)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", entry.Name(), parseErr)
		}
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv == nil || function.Body == nil {
				continue
			}
			operation, ok := operationByHandler[function.Name.Name]
			if !ok {
				continue
			}
			consumed := consumedHandlerQueryParameters(function.Body)
			properties, _ := operation.InputSchema["properties"].(map[string]any)
			missing := make([]string, 0)
			for parameter := range consumed {
				if _, declared := properties[parameter]; !declared {
					missing = append(missing, parameter)
				}
			}
			sort.Strings(missing)
			if len(missing) > 0 {
				t.Errorf("%s consumes undeclared query parameters: %v", operation.OperationID, missing)
			}
		}
	}
}

func consumedHandlerQueryParameters(body *ast.BlockStmt) map[string]struct{} {
	parameters := map[string]struct{}{}
	ast.Inspect(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if selector, ok := call.Fun.(*ast.SelectorExpr); ok {
			context, contextOK := selector.X.(*ast.Ident)
			if contextOK && context.Name == "ctx" &&
				(selector.Sel.Name == "Query" || selector.Sel.Name == "DefaultQuery" || selector.Sel.Name == "GetQuery" || selector.Sel.Name == "QueryArray") {
				addStringArgument(parameters, call.Args, 0)
			}
			return true
		}
		function, ok := call.Fun.(*ast.Ident)
		if !ok {
			return true
		}
		switch function.Name {
		case "paginationFromQuery", "paginationFromQueryWithSort":
			for _, parameter := range []string{"page", "pageSize", "sortBy", "sortOrder"} {
				parameters[parameter] = struct{}{}
			}
		case "applySearch":
			parameters["search"] = struct{}{}
		case "boolQuery":
			addStringArgument(parameters, call.Args, 1)
		}
		return true
	})
	return parameters
}

func addStringArgument(target map[string]struct{}, arguments []ast.Expr, index int) {
	if index >= len(arguments) {
		return
	}
	literal, ok := arguments[index].(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return
	}
	value, err := strconv.Unquote(literal.Value)
	if err == nil && value != "" {
		target[value] = struct{}{}
	}
}

func upperFirst(value string) string {
	if value == "" {
		return value
	}
	return strings.ToUpper(value[:1]) + value[1:]
}
