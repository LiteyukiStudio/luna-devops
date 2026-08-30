package config

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestProductionEnvironmentAccessIsOwnedByStartupAdapters(t *testing.T) {
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	allowed := map[string]bool{
		"internal/config/environment.go":  true,
		"internal/gatewayprobe/config.go": true,
		"internal/telemetry/logger.go":    true,
		"internal/testdb/postgres.go":     true,
		"internal/transferjob/config.go":  true,
	}
	var violations []string
	err := filepath.WalkDir(repositoryRoot, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == "node_modules" || entry.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(filePath) != ".go" || strings.HasSuffix(filePath, "_test.go") {
			return nil
		}
		relativePath, err := filepath.Rel(repositoryRoot, filePath)
		if err != nil {
			return err
		}
		relativePath = filepath.ToSlash(relativePath)
		if allowed[relativePath] {
			return nil
		}
		fileSet := token.NewFileSet()
		parsed, err := parser.ParseFile(fileSet, filePath, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		osAliases := map[string]bool{}
		for _, imported := range parsed.Imports {
			importPath, _ := strconv.Unquote(imported.Path.Value)
			if importPath != "os" {
				continue
			}
			alias := "os"
			if imported.Name != nil {
				alias = imported.Name.Name
			}
			osAliases[alias] = true
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			identifier, ok := selector.X.(*ast.Ident)
			if !ok || !osAliases[identifier.Name] {
				return true
			}
			switch selector.Sel.Name {
			case "Getenv", "LookupEnv", "Environ", "ExpandEnv":
				position := fileSet.Position(selector.Pos())
				violations = append(violations, relativePath+":"+strconv.Itoa(position.Line))
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 0 {
		t.Fatalf("production code reads process environment outside startup adapters: %s", strings.Join(violations, ", "))
	}

	loggerSource, err := os.ReadFile(filepath.Join(repositoryRoot, "internal", "telemetry", "logger.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"LOG_FORMAT", "LOG_COLOR", "LOG_LEVEL", "NO_COLOR", "OTEL_EXPORTER"} {
		if strings.Contains(string(loggerSource), forbidden) {
			t.Fatalf("telemetry logger owns deployment setting %s instead of receiving the startup snapshot", forbidden)
		}
	}
}
