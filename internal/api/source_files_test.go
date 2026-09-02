package api

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func apiSourceRoot(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve API test source path")
	}
	return filepath.Dir(currentFile)
}

func apiProductionGoFiles(t *testing.T) []string {
	t.Helper()
	root := apiSourceRoot(t)
	paths := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk API source files: %v", err)
	}
	sort.Strings(paths)
	return paths
}

func findAPIProductionGoFile(t *testing.T, fileName string) string {
	t.Helper()
	root := apiSourceRoot(t)
	wantPath := filepath.Clean(filepath.FromSlash(fileName))
	qualified := filepath.Dir(wantPath) != "."
	var match string
	for _, path := range apiProductionGoFiles(t) {
		relative, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatalf("resolve API source path %s: %v", path, err)
		}
		if qualified && filepath.Clean(relative) != wantPath {
			continue
		}
		if !qualified && filepath.Base(path) != wantPath {
			continue
		}
		if match != "" {
			t.Fatalf("API source file %s is ambiguous: %s and %s", fileName, match, path)
		}
		match = path
	}
	if match == "" {
		t.Fatalf("API source file %s was not found", fileName)
	}
	return match
}

func TestAPISubpackagesDoNotImportRootAPI(t *testing.T) {
	t.Parallel()
	root := apiSourceRoot(t)
	for _, path := range apiProductionGoFiles(t) {
		relative, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatalf("resolve API source path %s: %v", path, err)
		}
		if filepath.Dir(relative) == "." {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse API source file %s: %v", relative, err)
		}
		for _, imported := range file.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				t.Fatalf("decode import in %s: %v", relative, err)
			}
			if importPath == "github.com/LiteyukiStudio/devops/internal/api" {
				t.Errorf("API subpackage %s imports the root composition package", relative)
			}
		}
	}
}
