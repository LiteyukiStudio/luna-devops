package kubeproxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestMetricsApplicationBindingFailsClosedWhenProviderIgnoresSelector(t *testing.T) {
	base, _ := url.Parse("https://upstream.internal")
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("Accept-Encoding") != "identity" || request.Header.Get("Accept") != "application/json" {
			t.Fatalf("unsafe metrics request headers: %#v", request.Header)
		}
		body := `{"apiVersion":"metrics.k8s.io/v1beta1","kind":"PodMetricsList","items":[{"metadata":{"name":"foreign","namespace":"project-a","labels":{"app.kubernetes.io/managed-by":"luna-devops","luna.devops/project-id":"p1","luna.devops/application-id":"a2"}},"timestamp":"2026-08-31T00:00:00Z","window":"30s","containers":[]}]}`
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	access := baseAccess()
	access.ApplicationID = "a1"
	request := httptest.NewRequest(http.MethodGet, "https://gateway.example/apis/metrics.k8s.io/v1beta1/namespaces/project-a/pods", nil)
	writer := httptest.NewRecorder()
	err := (MetricsProxy{}).Serve(writer, request, access, RequestInfo{APIGroup: "metrics.k8s.io", APIVersion: "v1beta1", Resource: "pods", Namespace: access.Namespace, IsCollection: true}, Upstream{BaseURL: base, Transport: transport}, request.URL.EscapedPath())
	if err == nil {
		t.Fatal("foreign metrics entry must fail closed")
	}
	if writer.Body.Len() != 0 {
		t.Fatalf("partial metrics response leaked: %s", writer.Body.String())
	}
}

func TestMetricsUpstreamNon2xxStatusPassesThrough(t *testing.T) {
	base, _ := url.Parse("https://upstream.internal")
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Header: http.Header{"Content-Type": []string{"application/json"}, "Warning": []string{`299 kube "unavailable"`}}, Body: io.NopCloser(strings.NewReader(`{"kind":"Status","status":"Failure","code":503}`))}, nil
	})
	access := baseAccess()
	access.ApplicationID = "a1"
	request := httptest.NewRequest(http.MethodGet, "https://gateway.example/apis/metrics.k8s.io/v1beta1/namespaces/project-a/pods", nil)
	writer := httptest.NewRecorder()
	if err := (MetricsProxy{}).Serve(writer, request, access, RequestInfo{Resource: "pods", IsCollection: true}, Upstream{BaseURL: base, Transport: transport}, request.URL.EscapedPath()); err != nil {
		t.Fatal(err)
	}
	if writer.Code != http.StatusServiceUnavailable || writer.Header().Get("Warning") == "" {
		t.Fatalf("non-2xx metadata was not preserved: %d %#v", writer.Code, writer.Header())
	}
}

func TestMetricsApplicationHeadUsesInternalGetAndStillFailsClosed(t *testing.T) {
	base, _ := url.Parse("https://upstream.internal")
	upstreamMethod := ""
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		upstreamMethod = request.Method
		body := `{"apiVersion":"metrics.k8s.io/v1beta1","kind":"PodMetricsList","items":[{"metadata":{"name":"foreign","namespace":"project-a","labels":{"app.kubernetes.io/managed-by":"luna-devops","luna.devops/project-id":"p1","luna.devops/application-id":"a2","luna.devops/management-source":"kubectl"}},"timestamp":"2026-08-31T00:00:00Z","window":"30s","containers":[]}]}`
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	access := baseAccess()
	access.ApplicationID = "a1"
	request := httptest.NewRequest(http.MethodHead, "https://gateway.example/apis/metrics.k8s.io/v1beta1/namespaces/project-a/pods", nil)
	writer := httptest.NewRecorder()
	err := (MetricsProxy{}).Serve(writer, request, access, RequestInfo{APIGroup: "metrics.k8s.io", APIVersion: "v1beta1", Resource: "pods", Namespace: access.Namespace, IsCollection: true}, Upstream{BaseURL: base, Transport: transport}, request.URL.EscapedPath())
	if err == nil {
		t.Fatal("HEAD must fail closed when the internally fetched metrics list contains a foreign application")
	}
	if upstreamMethod != http.MethodGet {
		t.Fatalf("application PodMetrics HEAD must be validated through an internal GET, got %q", upstreamMethod)
	}
	if writer.Body.Len() != 0 {
		t.Fatalf("HEAD leaked a metrics response body: %s", writer.Body.String())
	}
}
