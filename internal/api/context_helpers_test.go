package api

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequestContextPreservesCallerContext(t *testing.T) {
	type contextKey string
	const key contextKey = "trace"
	want := context.WithValue(context.Background(), key, "trace-value")
	request := httptest.NewRequest("GET", "/", nil).WithContext(want)
	ginContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	ginContext.Request = request

	if got := requestContext(ginContext); got.Value(key) != "trace-value" {
		t.Fatal("requestContext did not preserve caller context values")
	}
}

func TestRequestContextRejectsMissingContext(t *testing.T) {
	assertPanics := func(name string, call func()) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("expected missing business context to panic")
				}
			}()
			call()
		})
	}
	assertPanics("request", func() { _ = requestContext(nil) })
}

func TestBusinessMethodsDoNotExposeOptionalContext(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, filepath.Clean(name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if function.Name.Name == "firstContext" {
				t.Errorf("%s: optional business context helper must not exist", fset.Position(function.Pos()))
			}
			if function.Type.Params == nil {
				continue
			}
			for _, field := range function.Type.Params.List {
				ellipsis, ok := field.Type.(*ast.Ellipsis)
				if !ok {
					continue
				}
				selector, ok := ellipsis.Elt.(*ast.SelectorExpr)
				if !ok {
					continue
				}
				packageName, packageOK := selector.X.(*ast.Ident)
				if packageOK && packageName.Name == "context" && selector.Sel.Name == "Context" {
					t.Errorf("%s: business methods must not expose optional context", fset.Position(field.Pos()))
				}
			}
		}
	}
}
