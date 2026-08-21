package volume

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func readVolumeMigration(t *testing.T, name string) string {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve volume migration test source")
	}
	content, err := os.ReadFile(filepath.Join(filepath.Dir(source), "..", "..", "migrations", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(content)
}
