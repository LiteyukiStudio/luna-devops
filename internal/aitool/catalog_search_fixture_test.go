package aitool

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const updateCatalogFixtureEnvironment = "UPDATE_PLATFORM_CATALOG_FIXTURE"

// TestPlatformCatalogSearchFixtureMatches keeps the Agent retrieval evaluation
// on the real OpenAPI-derived catalog instead of a separately maintained mock.
func TestPlatformCatalogSearchFixtureMatches(t *testing.T) {
	operations, err := PlatformCatalog()
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.MarshalIndent(operations, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	encoded = append(encoded, '\n')
	path := filepath.Join("..", "..", "luna-agent", "tests", "fixtures", "platform-catalog.json")
	if os.Getenv(updateCatalogFixtureEnvironment) == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, encoded, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	fixture, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v; regenerate with %s=1 go test ./internal/aitool -run TestPlatformCatalogSearchFixtureMatches", path, err, updateCatalogFixtureEnvironment)
	}
	if !bytes.Equal(fixture, encoded) {
		t.Fatalf("%s is stale; regenerate with %s=1 go test ./internal/aitool -run TestPlatformCatalogSearchFixtureMatches", path, updateCatalogFixtureEnvironment)
	}
}
