package kubeproxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestProxyStripsSensitiveUpstreamHeaders(t *testing.T) {
	base, _ := url.Parse("https://upstream.internal")
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		for _, name := range []string{"Authorization", "Cookie", "Impersonate-User", "Forwarded", "X-Forwarded-For", "traceparent"} {
			if request.Header.Get(name) != "" {
				t.Fatalf("sensitive header %s reached upstream", name)
			}
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"kind":"PodList"}`))}, nil
	})
	request := httptest.NewRequest(http.MethodGet, "https://gateway.example/api/v1/namespaces/project-a/pods", nil)
	for _, name := range []string{"Authorization", "Cookie", "Impersonate-User", "Forwarded", "X-Forwarded-For", "traceparent"} {
		request.Header.Set(name, "secret")
	}
	writer := httptest.NewRecorder()
	err := (HTTPProxy{}).Serve(writer, request, Upstream{BaseURL: base, Transport: transport}, request.URL.EscapedPath(), RequestInfo{IsResourceRequest: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if writer.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("resource response must disable caching")
	}
}

func TestProxyStripsResponseCookieAndRewritesLocation(t *testing.T) {
	base, _ := url.Parse("https://upstream.internal")
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusTemporaryRedirect,
			Header:     http.Header{"Location": []string{"https://upstream.internal/api/v1/pods/app"}, "Set-Cookie": []string{"platform=attacker"}},
			Body:       io.NopCloser(strings.NewReader("redirect")),
		}, nil
	})
	request := httptest.NewRequest(http.MethodGet, "https://gateway.example/api/v1/namespaces/project-a/pods/app", nil)
	writer := httptest.NewRecorder()
	err := (HTTPProxy{}).Serve(writer, request, Upstream{BaseURL: base, Transport: transport}, request.URL.EscapedPath(), RequestInfo{IsResourceRequest: true}, DiscoveryTransformer{KubePrefix: "/kube/v1/bindings/b1", RequestPath: request.URL.EscapedPath()})
	if err != nil {
		t.Fatal(err)
	}
	if writer.Header().Get("Set-Cookie") != "" || writer.Header().Get("Location") != "/kube/v1/bindings/b1/api/v1/pods/app" {
		t.Fatalf("unsafe response headers escaped: %#v", writer.Header())
	}
}

func TestBuildUpstreamRequestRejectsUserInfoAndClearsTransferState(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "https://gateway.example/api/v1/namespaces/project-a/pods", strings.NewReader("{}"))
	request.TransferEncoding = []string{"chunked"}
	request.Trailer = http.Header{"Authorization": []string{"secret"}}
	unsafe, _ := url.Parse("https://user:password@upstream.internal")
	if _, err := BuildUpstreamRequest(request, Upstream{BaseURL: unsafe, Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, nil })}, request.URL.EscapedPath()); err == nil {
		t.Fatal("upstream URL userinfo must be rejected")
	}
	base, _ := url.Parse("https://upstream.internal")
	upstreamRequest, err := BuildUpstreamRequest(request, Upstream{BaseURL: base, Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, nil })}, request.URL.EscapedPath())
	if err != nil {
		t.Fatal(err)
	}
	if len(upstreamRequest.TransferEncoding) != 0 || upstreamRequest.Trailer != nil {
		t.Fatalf("hop-by-hop request state survived: %#v %#v", upstreamRequest.TransferEncoding, upstreamRequest.Trailer)
	}
}
