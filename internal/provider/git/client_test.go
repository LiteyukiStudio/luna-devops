package gitprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/security"
)

func TestUserResponseExternalIDFormatsNumericIDs(t *testing.T) {
	if got := (UserResponse{ID: float64(79104275)}).ExternalID(); got != "79104275" {
		t.Fatalf("float64 id = %q", got)
	}
	if got := (UserResponse{ID: json.Number("79104275")}).ExternalID(); got != "79104275" {
		t.Fatalf("json number id = %q", got)
	}

	var user UserResponse
	if err := json.NewDecoder(strings.NewReader(`{"id":79104275,"login":"snowykami"}`)).Decode(&user); err != nil {
		t.Fatal(err)
	}
	if got := user.ExternalID(); got != "79104275" {
		t.Fatalf("decoded id = %q", got)
	}
	if got := user.Username(); got != "snowykami" {
		t.Fatalf("username = %q", got)
	}
}

func TestDiscoverBuildOptionsUsesRecursiveTree(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v3/repos/snowykami/neo-blog/branches/main":
			writeJSON(t, w, map[string]any{
				"name": "main",
				"commit": map[string]any{
					"sha": "commit-sha",
				},
			})
		case "/api/v3/repos/snowykami/neo-blog/git/trees/commit-sha":
			if r.URL.Query().Get("recursive") != "1" {
				t.Fatalf("recursive query = %q, want 1", r.URL.Query().Get("recursive"))
			}
			writeJSON(t, w, map[string]any{
				"sha":       "tree-sha",
				"truncated": false,
				"tree": []map[string]any{
					{"path": "Dockerfile", "type": "blob", "sha": "a"},
					{"path": "web", "type": "tree", "sha": "b"},
					{"path": "web/Dockerfile", "type": "blob", "sha": "c"},
					{"path": "web/src", "type": "tree", "sha": "d"},
					{"path": "README.md", "type": "blob", "sha": "e"},
				},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClientWithPolicy(model.GitProvider{Type: "github", BaseURL: server.URL}, "", security.AdminEgressPolicy())
	options, err := client.DiscoverBuildOptions(context.Background(), "snowykami", "neo-blog", "main")
	if err != nil {
		t.Fatalf("DiscoverBuildOptions() error = %v", err)
	}
	if options.Strategy != "recursive-tree" {
		t.Fatalf("strategy = %q, want recursive-tree", options.Strategy)
	}
	if !reflect.DeepEqual(options.Dockerfiles, []string{"Dockerfile", "web/Dockerfile"}) {
		t.Fatalf("dockerfiles = %#v", options.Dockerfiles)
	}
	if !reflect.DeepEqual(options.Directories, []string{".", "web", "web/src"}) {
		t.Fatalf("directories = %#v", options.Directories)
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode json: %v", err)
	}
}
