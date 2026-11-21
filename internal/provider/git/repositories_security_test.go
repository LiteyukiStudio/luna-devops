package gitprovider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/model"
	"github.com/LiteyukiStudio/devops/internal/security"
)

func TestGitRepositoryPathTraversalAndRedirectGuards(t *testing.T) {
	hitUpstream := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitUpstream = true
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	client := NewClientWithPolicy(model.GitProvider{Type: "gitea", BaseURL: upstream.URL}, "secret-token", security.AdminEgressPolicy())

	if _, err := client.ReadFile(context.Background(), "org", "repo", "../../user", "main"); err == nil {
		t.Fatal("expected ReadFile with .. traversal to be rejected")
	}
	if _, err := client.ListContents(context.Background(), "..", "repo", "", "main"); err == nil {
		t.Fatal("expected ListContents with .. owner to be rejected")
	}
	if _, err := client.ListBranches(context.Background(), "org/evil", "repo"); err == nil {
		t.Fatal("expected ListBranches with slash in owner to be rejected")
	}
	if hitUpstream {
		t.Fatal("traversal input reached upstream server")
	}

	// Cross-origin redirect under AllowPrivateNetwork must strip Authorization and block private redirect targets.
	targetHit := false
	privateTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetHit = true
		w.WriteHeader(http.StatusOK)
	}))
	defer privateTarget.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://localhost"+privateTarget.URL[len("http://127.0.0.1"):]+"/steal", http.StatusFound)
	}))
	defer redirector.Close()

	redirectClient := NewClientWithPolicy(model.GitProvider{Type: "gitea", BaseURL: redirector.URL}, "secret-token", security.AdminEgressPolicy())
	if _, err := redirectClient.CurrentUser(context.Background()); err == nil {
		t.Fatal("expected cross-origin redirect to private host to be blocked")
	}
	if targetHit {
		t.Fatal("cross-origin private redirect target was reached")
	}

	// Multi-hop bounce-back (Origin A -> External B -> Origin A) must strip re-copied Authorization,
	// and intermediate http -> https -> http must be rejected.
	httpClient := NewHTTPClientForPolicy(security.PublicEgressPolicy(), gitHTTPTimeout)
	initialReq, _ := http.NewRequest(http.MethodGet, "https://git-a.example.com/api/v1/user", nil)
	hop1Req, _ := http.NewRequest(http.MethodGet, "https://other.example.com/step", nil)
	bounceReq, _ := http.NewRequest(http.MethodGet, "https://git-a.example.com/evil", nil)
	bounceReq.Header.Set("Authorization", "Bearer git-secret")
	if err := httpClient.CheckRedirect(bounceReq, []*http.Request{initialReq, hop1Req}); err != nil {
		t.Fatalf("unexpected error on HTTPS bounce redirect: %v", err)
	}
	if got := bounceReq.Header.Get("Authorization"); got != "" {
		t.Fatalf("Authorization survived multi-hop bounce-back: %q", got)
	}

	httpInitial, _ := http.NewRequest(http.MethodGet, "http://git-a.example.com/api/v1/user", nil)
	httpsHop, _ := http.NewRequest(http.MethodGet, "https://git-a.example.com/api/v1/user", nil)
	httpDowngrade, _ := http.NewRequest(http.MethodGet, "http://git-a.example.com/plain", nil)
	if err := httpClient.CheckRedirect(httpDowngrade, []*http.Request{httpInitial, httpsHop}); err == nil {
		t.Fatal("expected intermediate http -> https -> http downgrade redirect to be rejected")
	}
}
