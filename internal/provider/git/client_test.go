package gitprovider

import (
	"context"
	"encoding/base64"
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
		case "/api/v3/repos/snowykami/neo-blog/contents/Dockerfile":
			writeJSON(t, w, map[string]any{
				"path":     "Dockerfile",
				"name":     "Dockerfile",
				"sha":      "a",
				"encoding": "base64",
				"content":  base64.StdEncoding.EncodeToString([]byte("FROM nginx\nEXPOSE 8080/tcp 8443\n")),
			})
		case "/api/v3/repos/snowykami/neo-blog/contents/web/Dockerfile":
			writeJSON(t, w, map[string]any{
				"path":     "web/Dockerfile",
				"name":     "Dockerfile",
				"sha":      "c",
				"encoding": "base64",
				"content":  base64.StdEncoding.EncodeToString([]byte("FROM node\nEXPOSE 3000\n")),
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
	if !reflect.DeepEqual(options.ExposedPorts["Dockerfile"], []int{8080, 8443}) {
		t.Fatalf("root ports = %#v", options.ExposedPorts["Dockerfile"])
	}
	if !reflect.DeepEqual(options.ExposedPorts["web/Dockerfile"], []int{3000}) {
		t.Fatalf("web ports = %#v", options.ExposedPorts["web/Dockerfile"])
	}
}

func TestParseDockerfileExposedPorts(t *testing.T) {
	ports := parseDockerfileExposedPorts(`
FROM alpine
# EXPOSE 9999
EXPOSE 80/tcp 443
expose 3000/udp 80
EXPOSE $PORT
`)
	if !reflect.DeepEqual(ports, []int{80, 443, 3000}) {
		t.Fatalf("ports = %#v", ports)
	}
}

func TestSearchPublicRepositoriesUsesGitHubSearchAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/search/repositories" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("q") != "LiteyukiStudio/devops" {
			t.Fatalf("query = %q", r.URL.Query().Get("q"))
		}
		writeJSON(t, w, map[string]any{
			"items": []map[string]any{{
				"name":           "devops",
				"full_name":      "LiteyukiStudio/devops",
				"clone_url":      "https://github.com/LiteyukiStudio/devops.git",
				"default_branch": "main",
				"private":        false,
				"owner": map[string]any{
					"login": "LiteyukiStudio",
				},
			}},
		})
	}))
	defer server.Close()

	client := NewClientWithPolicy(model.GitProvider{Type: "github", BaseURL: server.URL}, "", security.AdminEgressPolicy())
	repos, err := client.SearchPublicRepositories(context.Background(), "LiteyukiStudio/devops", 1, 10)
	if err != nil {
		t.Fatalf("SearchPublicRepositories() error = %v", err)
	}
	if len(repos) != 1 || repos[0].FullName != "LiteyukiStudio/devops" || repos[0].Source != "public" {
		t.Fatalf("repos = %#v", repos)
	}
}

func TestCreateGitHubWebhookSendsEventsAndSecret(t *testing.T) {
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/repos/snowykami/neo-blog/hooks" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		writeJSON(t, w, map[string]any{
			"id": 42,
			"config": map[string]any{
				"url": "https://devops.example.com/api/v1/git/webhooks/rpb_1",
			},
		})
	}))
	defer server.Close()

	client := NewClientWithPolicy(model.GitProvider{Type: "github", BaseURL: server.URL}, "token", security.AdminEgressPolicy())
	result, err := client.CreateWebhook(context.Background(), "snowykami", "neo-blog", "https://devops.example.com/api/v1/git/webhooks/rpb_1", "secret-value")
	if err != nil {
		t.Fatalf("CreateWebhook() error = %v", err)
	}
	if result.ID != "42" {
		t.Fatalf("webhook id = %q", result.ID)
	}
	if !reflect.DeepEqual(payload["events"], []any{"push", "create"}) {
		t.Fatalf("events = %#v", payload["events"])
	}
	config, ok := payload["config"].(map[string]any)
	if !ok {
		t.Fatalf("config = %#v", payload["config"])
	}
	if config["secret"] != "secret-value" {
		t.Fatalf("secret = %#v", config["secret"])
	}
}

func TestCreateGitHubWebhookValidationErrorIsStructured(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/repos/snowykami/neo-blog/hooks" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusUnprocessableEntity)
		writeJSON(t, w, map[string]any{
			"message": "Validation Failed",
			"errors": []map[string]string{{
				"resource": "Hook",
				"code":     "custom",
				"field":    "url",
				"message":  "url is not supported because it isn't reachable over the public Internet (localhost)",
			}},
			"documentation_url": "https://docs.github.com/rest/repos/webhooks#create-a-repository-webhook",
			"status":            "422",
		})
	}))
	defer server.Close()

	client := NewClientWithPolicy(model.GitProvider{Type: "github", BaseURL: server.URL}, "token", security.AdminEgressPolicy())
	_, err := client.CreateWebhook(context.Background(), "snowykami", "neo-blog", "http://localhost:5173/api/v1/git/webhooks/rpb_1", "secret-value")
	if err == nil {
		t.Fatal("expected CreateWebhook() error")
	}
	upstreamErr, ok := AsUpstreamError(err)
	if !ok {
		t.Fatalf("expected UpstreamError, got %T", err)
	}
	if upstreamErr.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d", upstreamErr.StatusCode)
	}
	if upstreamErr.Message != "Validation Failed" {
		t.Fatalf("message = %q", upstreamErr.Message)
	}
	if len(upstreamErr.Details) != 1 || upstreamErr.Details[0].Resource != "Hook" || upstreamErr.Details[0].Field != "url" {
		t.Fatalf("details = %#v", upstreamErr.Details)
	}
	if strings.Contains(err.Error(), "documentation_url") || strings.Contains(err.Error(), "{") {
		t.Fatalf("error should not contain raw upstream body: %q", err.Error())
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode json: %v", err)
	}
}
