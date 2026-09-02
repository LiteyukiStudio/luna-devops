package buildapi

import (
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
)

func TestParseBuildArgsInputLines(t *testing.T) {
	values, err := parseBuildArgsInput(`
EMBED_WEB=true
VERSION=${{ github.sha }}
# ignored
`)
	if err != nil {
		t.Fatalf("parse build args: %v", err)
	}
	if values["EMBED_WEB"] != "true" {
		t.Fatalf("expected EMBED_WEB=true, got %q", values["EMBED_WEB"])
	}
	if values["VERSION"] != "${{ github.sha }}" {
		t.Fatalf("expected VERSION template to be preserved, got %q", values["VERSION"])
	}
}

func TestParseBuildArgsInputJSON(t *testing.T) {
	values, err := parseBuildArgsInput(`{"VERSION":"{short_sha}","EMBED_WEB":"true"}`)
	if err != nil {
		t.Fatalf("parse build args json: %v", err)
	}
	if encoded := model.EncodeBuildArgs(values); encoded != `{"EMBED_WEB":"true","VERSION":"{short_sha}"}` {
		t.Fatalf("unexpected encoded build args: %s", encoded)
	}
}

func TestParseBuildArgsInputRejectsInvalidName(t *testing.T) {
	if _, err := parseBuildArgsInput("1INVALID=true"); err == nil {
		t.Fatal("expected invalid build arg name to fail")
	}
}
