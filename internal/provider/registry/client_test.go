package registryprovider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/builder"
	"github.com/LiteyukiStudio/devops/internal/security"
)

func TestPingURLNormalizesV2Path(t *testing.T) {
	cases := map[string]string{
		"https://registry.example.com":           "https://registry.example.com/v2/",
		"https://registry.example.com/":          "https://registry.example.com/v2/",
		"https://registry.example.com/v2":        "https://registry.example.com/v2/",
		"https://registry.example.com/v2/":       "https://registry.example.com/v2/",
		"https://registry.example.com/harbor":    "https://registry.example.com/harbor/v2/",
		"https://registry.example.com/harbor/v2": "https://registry.example.com/harbor/v2/",
	}

	for input, expected := range cases {
		t.Run(input, func(t *testing.T) {
			actual, err := PingURL(input)
			if err != nil {
				t.Fatalf("PingURL() error = %v", err)
			}
			if actual.String() != expected {
				t.Fatalf("PingURL() = %q, want %q", actual.String(), expected)
			}
		})
	}
}

func TestPingSendsTokenOnlyBasicAuth(t *testing.T) {
	expectedAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(":registry-token"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/" {
			t.Fatalf("request path = %q, want /v2/", r.URL.Path)
		}
		if r.Header.Get("Authorization") != expectedAuth {
			t.Fatalf("Authorization = %q, want %q", r.Header.Get("Authorization"), expectedAuth)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	result := Ping(context.Background(), server.URL, security.AdminEgressPolicy(), Credential{Secret: "registry-token"})
	if !result.Success {
		t.Fatalf("Ping() success = false, message = %q", result.Message)
	}
}

func TestSearchCatalogRepositoriesFiltersAndLimits(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/_catalog" {
			t.Fatalf("request path = %q, want /v2/_catalog", r.URL.Path)
		}
		if r.URL.Query().Get("n") != "2" {
			t.Fatalf("n = %q, want 2", r.URL.Query().Get("n"))
		}
		_ = json.NewEncoder(w).Encode(map[string][]string{
			"repositories": {
				"team/api",
				"team/web",
				"team/worker",
				"other/api",
			},
		})
	}))
	defer server.Close()

	result, err := SearchRepositories(context.Background(), "generic-oci", server.URL, "team", "w", 1, 2, security.AdminEgressPolicy(), Credential{})
	if err != nil {
		t.Fatalf("SearchRepositories() error = %v", err)
	}
	if result.Total != 2 {
		t.Fatalf("Total = %d, want 2", result.Total)
	}
	if result.Limited {
		t.Fatalf("Limited = true, want false")
	}
	if len(result.Items) != 2 || result.Items[0].Name != "team/web" || result.Items[1].Name != "team/worker" {
		t.Fatalf("Items = %#v, want team/web and team/worker", result.Items)
	}
}

func TestListRegistryTagsLimitsRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/team/web/tags/list" {
			t.Fatalf("request path = %q, want /v2/team/web/tags/list", r.URL.Path)
		}
		if r.URL.Query().Get("n") != "50" {
			t.Fatalf("n = %q, want 50", r.URL.Query().Get("n"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "team/web",
			"tags": []string{"latest", builder.RegistryCacheTag, "v1"},
		})
	}))
	defer server.Close()

	result, err := ListTags(context.Background(), "generic-oci", server.URL, "team/web", 200, security.AdminEgressPolicy(), Credential{})
	if err != nil {
		t.Fatalf("ListTags() error = %v", err)
	}
	if len(result.Items) != 2 || result.Items[0].Name != "latest" || result.Items[1].Name != "v1" {
		t.Fatalf("Items = %#v, want latest and v1 without the build cache tag", result.Items)
	}
}
