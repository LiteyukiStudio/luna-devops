package kubernetes

import (
	"context"
	"net/http"
	"net/url"

	"github.com/LiteyukiStudio/devops/internal/telemetry"
)

type kubernetesOriginalRequestContextKey struct{}

type kubernetesOriginalRequestTarget struct {
	url  *url.URL
	host string
}

type restoreKubernetesRequestTransport struct {
	next http.RoundTripper
}

func (t restoreKubernetesRequestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	original, _ := request.Context().Value(kubernetesOriginalRequestContextKey{}).(kubernetesOriginalRequestTarget)
	if original.url == nil {
		return t.next.RoundTrip(request)
	}
	actual := request.Clone(request.Context())
	actual.URL = cloneKubernetesURL(original.url)
	actual.Host = original.host
	return t.next.RoundTrip(actual)
}

type sanitizeKubernetesTelemetryTransport struct {
	next http.RoundTripper
}

func (t sanitizeKubernetesTelemetryTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request == nil || request.URL == nil {
		return t.next.RoundTrip(request)
	}
	original := kubernetesOriginalRequestTarget{url: cloneKubernetesURL(request.URL), host: request.Host}
	ctx := context.WithValue(request.Context(), kubernetesOriginalRequestContextKey{}, original)
	sanitized := request.Clone(ctx)
	sanitized.URL = cloneKubernetesURL(request.URL)
	sanitized.URL.Host = "kubernetes.invalid"
	sanitized.Host = "kubernetes.invalid"
	sanitized.URL.Path = "/kubernetes"
	sanitized.URL.RawPath = ""
	sanitized.URL.RawQuery = ""
	sanitized.URL.ForceQuery = false
	sanitized.URL.Fragment = ""
	sanitized.URL.User = nil
	return t.next.RoundTrip(sanitized)
}

func instrumentKubernetesHTTPTransport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	traced := telemetry.InstrumentHTTPTransport(restoreKubernetesRequestTransport{next: base})
	return sanitizeKubernetesTelemetryTransport{next: traced}
}

func cloneKubernetesURL(value *url.URL) *url.URL {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
