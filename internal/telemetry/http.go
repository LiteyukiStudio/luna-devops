package telemetry

import (
	"context"
	"net/http"
	"net/url"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type originalRequestURLKey struct{}

type restoreRequestURLTransport struct{ next http.RoundTripper }

func (t restoreRequestURLTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	original, _ := request.Context().Value(originalRequestURLKey{}).(*url.URL)
	if original == nil {
		return t.next.RoundTrip(request)
	}
	actual := request.Clone(request.Context())
	actual.URL = cloneURL(original)
	return t.next.RoundTrip(actual)
}

type sanitizedTelemetryTransport struct{ next http.RoundTripper }

func (t sanitizedTelemetryTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request == nil || request.URL == nil {
		return t.next.RoundTrip(request)
	}
	original := cloneURL(request.URL)
	ctx := context.WithValue(request.Context(), originalRequestURLKey{}, original)
	sanitized := request.Clone(ctx)
	sanitized.URL = cloneURL(request.URL)
	sanitized.URL.RawQuery = ""
	sanitized.URL.ForceQuery = false
	sanitized.URL.Fragment = ""
	sanitized.URL.User = nil
	return t.next.RoundTrip(sanitized)
}

func cloneURL(value *url.URL) *url.URL {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func InstrumentHTTPTransport(transport http.RoundTripper) http.RoundTripper {
	if transport == nil {
		transport = http.DefaultTransport
	}
	// otelhttp records url.full. Present a redacted URL to instrumentation,
	// then restore the actual URL only at the network boundary.
	traced := otelhttp.NewTransport(restoreRequestURLTransport{next: transport})
	return sanitizedTelemetryTransport{next: traced}
}

func InstrumentHTTPClient(client *http.Client) *http.Client {
	if client == nil {
		client = &http.Client{}
	}
	client.Transport = InstrumentHTTPTransport(client.Transport)
	return client
}
