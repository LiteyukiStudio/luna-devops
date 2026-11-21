package registryprovider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
		"https://index.docker.io":                "https://registry-1.docker.io/v2/",
		"https://index.docker.io/v1/":            "https://registry-1.docker.io/v2/",
		"https://hub.docker.com":                 "https://registry-1.docker.io/v2/",
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

func TestBearerChallengeExchangesTokenForPingAndTags(t *testing.T) {
	expectedBasic := "Basic " + base64.StdEncoding.EncodeToString([]byte("robot$dev:harbor-secret"))
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/service/token":
			if r.Header.Get("Authorization") != expectedBasic {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "issued-jwt-token"})
		case "/v2/":
			if r.Header.Get("Authorization") != "Bearer issued-jwt-token" {
				w.Header().Set("WWW-Authenticate", `Bearer realm="`+serverURL+`/service/token",service="harbor-registry"`)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusOK)
		case "/v2/team/app/tags/list":
			if r.Header.Get("Authorization") != "Bearer issued-jwt-token" {
				w.Header().Set("WWW-Authenticate", `Bearer realm="`+serverURL+`/service/token",service="harbor-registry",scope="repository:team/app:pull"`)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "team/app", "tags": []string{"v2.0.0"}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	cred := Credential{Username: "robot$dev", Secret: "harbor-secret"}
	pingRes := Ping(context.Background(), server.URL, security.AdminEgressPolicy(), cred)
	if !pingRes.Success {
		t.Fatalf("Ping() with Bearer challenge failed: %+v", pingRes)
	}

	tagsRes, err := ListTags(context.Background(), "generic-oci", server.URL, "team/app", 20, security.AdminEgressPolicy(), cred)
	if err != nil {
		t.Fatalf("ListTags() with Bearer challenge error = %v", err)
	}
	if len(tagsRes.Items) != 1 || tagsRes.Items[0].Name != "v2.0.0" {
		t.Fatalf("ListTags() items = %#v, want [v2.0.0]", tagsRes.Items)
	}
}

func TestBearerChallengeRefusesCrossHostCredentialLeak(t *testing.T) {
	evilLeaked := false
	evilRealm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		evilLeaked = true
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "evil-token"})
	}))
	defer evilRealm.Close()

	// Bind evilRealm on an alternate hostname (localhost vs 127.0.0.1) so hosts differ.
	evilURL := "http://localhost" + evilRealm.URL[len("http://127.0.0.1"):]
	registryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="`+evilURL+`/token",service="evil"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer registryServer.Close()

	res := Ping(context.Background(), registryServer.URL, security.AdminEgressPolicy(), Credential{Username: "user", Secret: "top-secret"})
	if res.Success || res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 failure when realm host mismatches, got %+v", res)
	}
	if evilLeaked {
		t.Fatal("credentials were forwarded to an untrusted cross-host Bearer realm")
	}
}

func TestRegistrySecurityAndProtocolHardening(t *testing.T) {
	var serverURL string
	var observedScopes []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/service/token":
			observedScopes = r.URL.Query()["scope"]
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "multi-scope-token"})
		case "/v2/team/app/tags/list":
			if r.Header.Get("Authorization") != "Bearer multi-scope-token" {
				w.Header().Set("WWW-Authenticate", `Basic realm="Fallback", Bearer realm="`+serverURL+`/service/token", service="reg", scope="repository:team/app:pull repository:base:pull", Basic realm="Trailing", service="wrong"`)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "team/app", "tags": []string{"v1"}})
		default:
			t.Fatalf("unexpected path reached upstream: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	// Endpoint configured with trailing /v2 must not double /v2/v2/...
	tagsRes, err := ListTags(context.Background(), "generic-oci", server.URL+"/v2", "team/app", 10, security.AdminEgressPolicy(), Credential{Username: "u", Secret: "p"})
	if err != nil || len(tagsRes.Items) != 1 || tagsRes.Items[0].Name != "v1" {
		t.Fatalf("ListTags(/v2 endpoint) = %#v, err = %v", tagsRes, err)
	}
	if len(observedScopes) != 2 || observedScopes[0] != "repository:team/app:pull" || observedScopes[1] != "repository:base:pull" {
		t.Fatalf("token scopes = %#v, want 2 separate scope query params", observedScopes)
	}

	// Path traversal in repository must be rejected before issuing HTTP requests.
	for _, badRepo := range []string{"../../api/v2.0/users", "team/../admin", ".", "..", `team\app`} {
		if _, err := ListTags(context.Background(), "harbor", server.URL, badRepo, 10, security.AdminEgressPolicy(), Credential{}); err == nil {
			t.Fatalf("expected path traversal repository %q to be rejected", badRepo)
		}
	}

	// Embedded userinfo, query, and fragment in registry endpoint must be rejected.
	for _, badEndpoint := range []string{
		"https://user:pass@registry.example.com",
		"https://registry.example.com?token=1",
		"https://registry.example.com#frag",
	} {
		if _, err := ParseEndpoint(badEndpoint); err == nil {
			t.Fatalf("expected bad endpoint %q to be rejected", badEndpoint)
		}
	}

	// Multi-hop redirect bounce-back (Origin A -> External B -> Origin A) must still strip Authorization
	// and must not bypass HTTPS->HTTP downgrade checks across intermediate hops.
	client := newRegistryHTTPClient(security.PublicEgressPolicy(), 5*time.Second, false)
	initialReq, _ := http.NewRequest(http.MethodGet, "https://reg-a.example.com/v2/", nil)
	hop1Req, _ := http.NewRequest(http.MethodGet, "https://cdn-b.example.com/blob", nil)
	bounceReq, _ := http.NewRequest(http.MethodGet, "https://reg-a.example.com/evil", nil)
	bounceReq.Header.Set("Authorization", "Bearer leaked-token")
	if err := client.CheckRedirect(bounceReq, []*http.Request{initialReq, hop1Req}); err != nil {
		t.Fatalf("unexpected error on valid HTTPS bounce redirect: %v", err)
	}
	if got := bounceReq.Header.Get("Authorization"); got != "" {
		t.Fatalf("Authorization survived multi-hop cross-origin bounce-back: %q", got)
	}

	httpInitial, _ := http.NewRequest(http.MethodGet, "http://reg-a.example.com/v2/", nil)
	httpsHop, _ := http.NewRequest(http.MethodGet, "https://reg-a.example.com/v2/", nil)
	httpDowngrade, _ := http.NewRequest(http.MethodGet, "http://reg-a.example.com/plain", nil)
	if err := client.CheckRedirect(httpDowngrade, []*http.Request{httpInitial, httpsHop}); err == nil {
		t.Fatal("expected http -> https -> http downgrade redirect to be rejected")
	}
}
