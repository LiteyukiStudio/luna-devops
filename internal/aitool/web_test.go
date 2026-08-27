package aitool

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/LiteyukiStudio/devops/internal/security"
)

func TestFetchWebPageExtractsReadableTextAndLinks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = response.Write([]byte(`<!doctype html>
			<html><head><title>部署说明</title><style>.hidden{display:none}</style></head>
			<body><main><h1>示例服务</h1><p>运行 pnpm start，监听 3000 端口。</p>
			<a href="/Dockerfile">Dockerfile</a><script>ignore()</script></main></body></html>`))
	}))
	defer server.Close()

	service := NewService(nil, WithWebPolicyProvider(func(context.Context, string) (security.EgressPolicy, error) {
		return security.AdminEgressPolicy(), nil
	}))
	result, err := service.fetchWebPage(context.Background(), "usr_test", map[string]any{
		"url": server.URL, "maxCharacters": float64(5000),
	})
	if err != nil {
		t.Fatalf("fetchWebPage() error = %v", err)
	}
	value := result.Value.(map[string]any)
	if value["title"] != "部署说明" {
		t.Fatalf("title = %#v", value["title"])
	}
	content := value["content"].(string)
	if !strings.Contains(content, "示例服务") || !strings.Contains(content, "pnpm start") ||
		!strings.Contains(content, "3000") || strings.Contains(content, "ignore()") {
		t.Fatalf("content = %q", content)
	}
	links := value["links"].([]map[string]string)
	if len(links) != 1 || links[0]["url"] != server.URL+"/Dockerfile" {
		t.Fatalf("links = %#v", links)
	}
	if value["contentTrust"] != "untrusted_external" {
		t.Fatalf("contentTrust = %#v", value["contentTrust"])
	}
}

func TestFetchWebPageAppliesIPBlockList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()

	service := NewService(nil, WithWebPolicyProvider(func(context.Context, string) (security.EgressPolicy, error) {
		policy := security.AdminEgressPolicy()
		policy.IPBlockList = []string{"127.0.0.0/8"}
		return policy, nil
	}))
	_, err := service.fetchWebPage(context.Background(), "usr_test", map[string]any{"url": server.URL})
	if !errors.Is(err, ErrWebTargetBlocked) {
		t.Fatalf("fetchWebPage() error = %v, want ErrWebTargetBlocked", err)
	}
}

func TestFetchWebPageRejectsBinaryContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "image/png")
		_, _ = response.Write([]byte{0x89, 0x50, 0x4e, 0x47})
	}))
	defer server.Close()

	service := NewService(nil, WithWebPolicyProvider(func(context.Context, string) (security.EgressPolicy, error) {
		return security.AdminEgressPolicy(), nil
	}))
	_, err := service.fetchWebPage(context.Background(), "usr_test", map[string]any{"url": server.URL})
	if !errors.Is(err, ErrWebContentRejected) {
		t.Fatalf("fetchWebPage() error = %v, want ErrWebContentRejected", err)
	}
}

func TestFetchWebPageRejectsCredentialBearingURLs(t *testing.T) {
	service := NewService(nil, WithWebPolicyProvider(func(context.Context, string) (security.EgressPolicy, error) {
		return security.AdminEgressPolicy(), nil
	}))
	for _, target := range []string{
		"https://user:password@example.com/readme",
		"https://example.com/readme?access_token=secret",
		"https://example.com/readme?X-Amz-Signature=secret",
	} {
		if _, err := service.fetchWebPage(context.Background(), "usr_test", map[string]any{"url": target}); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("fetchWebPage(%q) error = %v, want ErrInvalidInput", target, err)
		}
	}
}

func TestFetchWebPageValidatesEveryRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		http.Redirect(response, request, "https://blocked.example/docs", http.StatusFound)
	}))
	defer server.Close()

	service := NewService(nil, WithWebPolicyProvider(func(context.Context, string) (security.EgressPolicy, error) {
		policy := security.AdminEgressPolicy()
		policy.DomainBlockList = []string{"blocked.example"}
		return policy, nil
	}))
	_, err := service.fetchWebPage(context.Background(), "usr_test", map[string]any{"url": server.URL})
	if !errors.Is(err, ErrWebTargetBlocked) {
		t.Fatalf("fetchWebPage() error = %v, want ErrWebTargetBlocked", err)
	}
}

func TestFetchWebPageUsesAuthenticatedProxyWithoutExposingCredentials(t *testing.T) {
	var calls atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		expectedAuthorization := "Basic " + base64.StdEncoding.EncodeToString([]byte("proxy-user:proxy-password"))
		if request.Header.Get("Proxy-Authorization") != expectedAuthorization {
			response.WriteHeader(http.StatusProxyAuthRequired)
			return
		}
		response.Header().Set("Content-Type", "text/plain")
		_, _ = response.Write([]byte("proxied deployment guide"))
	}))
	defer proxy.Close()
	proxyURL, _ := url.Parse(proxy.URL)
	proxyURL.User = url.UserPassword("proxy-user", "proxy-password")

	service := NewService(
		nil,
		WithWebPolicyProvider(func(context.Context, string) (security.EgressPolicy, error) {
			policy := security.AdminEgressPolicy()
			policy.DomainAllowList = []string{"example.com"}
			return policy, nil
		}),
		WithWebProxyProvider(func(context.Context, string) ([]string, error) {
			return []string{proxyURL.String()}, nil
		}),
	)
	result, err := service.fetchWebPage(context.Background(), "usr_test", map[string]any{"url": "http://example.com/readme"})
	if err != nil {
		t.Fatalf("fetchWebPage() error = %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("proxy calls = %d, want 1", calls.Load())
	}
	value := result.Value.(map[string]any)
	if strings.Contains(strings.Join([]string{value["url"].(string), value["finalUrl"].(string), value["content"].(string)}, "\n"), "proxy-password") {
		t.Fatal("proxy credentials leaked into tool result")
	}
}

func TestWebProxyPoolRotatesBetweenEntries(t *testing.T) {
	var firstCalls atomic.Int32
	var secondCalls atomic.Int32
	newProxy := func(counter *atomic.Int32, body string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			counter.Add(1)
			response.Header().Set("Content-Type", "text/plain")
			_, _ = response.Write([]byte(body))
		}))
	}
	first := newProxy(&firstCalls, "first")
	defer first.Close()
	second := newProxy(&secondCalls, "second")
	defer second.Close()

	service := NewService(
		nil,
		WithWebPolicyProvider(func(context.Context, string) (security.EgressPolicy, error) {
			policy := security.AdminEgressPolicy()
			policy.DomainAllowList = []string{"example.com"}
			return policy, nil
		}),
		WithWebProxyProvider(func(context.Context, string) ([]string, error) {
			return []string{first.URL, second.URL}, nil
		}),
	)
	for range 4 {
		if _, err := service.fetchWebPage(context.Background(), "usr_test", map[string]any{"url": "http://example.com/readme"}); err != nil {
			t.Fatalf("fetchWebPage() error = %v", err)
		}
	}
	if firstCalls.Load() != 2 || secondCalls.Load() != 2 {
		t.Fatalf("proxy calls = (%d, %d), want (2, 2)", firstCalls.Load(), secondCalls.Load())
	}
}

func TestEnabledWebProxyFailureDoesNotFallBackToDirectAccess(t *testing.T) {
	var directCalls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		directCalls.Add(1)
		response.Header().Set("Content-Type", "text/plain")
		_, _ = response.Write([]byte("must not be reached"))
	}))
	defer target.Close()

	service := NewService(
		nil,
		WithWebPolicyProvider(func(context.Context, string) (security.EgressPolicy, error) {
			return security.AdminEgressPolicy(), nil
		}),
		WithWebProxyProvider(func(context.Context, string) ([]string, error) {
			return nil, errors.New("encrypted proxy pool unavailable")
		}),
	)
	if _, err := service.fetchWebPage(context.Background(), "usr_test", map[string]any{"url": target.URL}); !errors.Is(err, ErrWebRequestFailed) {
		t.Fatalf("fetchWebPage() error = %v, want ErrWebRequestFailed", err)
	}
	if directCalls.Load() != 0 {
		t.Fatalf("direct target calls = %d, want 0", directCalls.Load())
	}
}

func TestParseWebProxyPoolValidatesAndDeduplicates(t *testing.T) {
	proxies, err := ParseWebProxyPool([]string{
		"http://user:password@proxy.example.com:888",
		"http://user:password@proxy.example.com:888",
		"https://proxy-two.example.com",
	})
	if err != nil {
		t.Fatalf("ParseWebProxyPool() error = %v", err)
	}
	if len(proxies) != 2 || proxies[0].User.Username() != "user" {
		t.Fatalf("proxies = %#v", proxies)
	}
	for _, raw := range []string{
		"socks5://proxy.example.com:1080",
		"http://proxy.example.com:888/path",
		"http://proxy.example.com:888?token=secret",
	} {
		if _, err := ParseWebProxyPool([]string{raw}); err == nil {
			t.Fatalf("invalid proxy accepted: %s", raw)
		}
	}
}

func TestParseDuckDuckGoResultsDecodesTargets(t *testing.T) {
	content := []byte(`<html><body>
		<a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fgithub.com%2FLiteyukiStudio%2Fluna-devops">Luna DevOps</a>
		<a class="result__a" href="https://example.com/docs">Deployment guide</a>
	</body></html>`)
	items := parseDuckDuckGoResults(content, "https://html.duckduckgo.com/html/?q=luna", 5)
	if len(items) != 2 {
		t.Fatalf("items = %#v", items)
	}
	if items[0].URL != "https://github.com/LiteyukiStudio/luna-devops" || items[1].Title != "Deployment guide" {
		t.Fatalf("items = %#v", items)
	}
}
