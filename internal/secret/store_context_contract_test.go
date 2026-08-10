package secret

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestStoreDatabaseMethodsRequireExplicitContext(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "store.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, declaration := range file.Decls {
		if typeDeclaration, ok := declaration.(*ast.GenDecl); ok {
			for _, spec := range typeDeclaration.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if ok && typeSpec.Name.Name == "AuditFunc" {
					t.Error("secret Store must not expose a context-free AuditFunc")
				}
			}
		}
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Recv == nil {
			continue
		}
		if function.Name.Name == "Store" || function.Name.Name == "Resolve" || function.Name.Name == "WithContextAudit" {
			t.Errorf("secret Store must not expose context-free %s", function.Name.Name)
		}
	}
}
