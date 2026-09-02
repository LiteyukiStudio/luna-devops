package api

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
)

func TestRequestDatabaseCallsRequireContext(t *testing.T) {
	for _, path := range apiProductionGoFiles(t) {
		if filepath.Base(path) == "handlers.go" && filepath.Dir(path) == apiSourceRoot(t) {
			continue
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, filepath.Clean(path), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.SelectorExpr:
				if value.Sel.Name == "db" {
					if receiver, ok := value.X.(*ast.Ident); ok && receiver.Name == "h" {
						t.Errorf("%s: request database access must use dbFor/dbWithContext", fset.Position(value.Pos()))
					}
				}
			case *ast.CallExpr:
				selector, ok := value.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if selector.Sel.Name == "audit" {
					if receiver, ok := selector.X.(*ast.Ident); ok && receiver.Name == "h" {
						t.Errorf("%s: request audit writes must use auditWithContext", fset.Position(value.Pos()))
					}
				}
				if selector.Sel.Name == "Store" || selector.Sel.Name == "Resolve" {
					if secrets, ok := selector.X.(*ast.SelectorExpr); ok && secrets.Sel.Name == "secrets" {
						if receiver, ok := secrets.X.(*ast.Ident); ok && receiver.Name == "h" {
							t.Errorf("%s: request secret database access must use StoreContext/ResolveContext", fset.Position(value.Pos()))
						}
					}
				}
			}
			return true
		})
	}
}
